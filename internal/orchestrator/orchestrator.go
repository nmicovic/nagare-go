// Package orchestrator binds durable tickets to isolated agent worktrees.
package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nemke/nagare-go/internal/attempts"
	"github.com/nemke/nagare-go/internal/git"
	"github.com/nemke/nagare-go/internal/mcp"
	"github.com/nemke/nagare-go/internal/session"
	"github.com/nemke/nagare-go/internal/tickets"
	"github.com/nemke/nagare-go/internal/tmux"
)

// LaunchFunc creates the worktree and starts the agent process.
type LaunchFunc func(repoPath, worktreePath, windowName, branch, baseCommit, agent string) (session.ManagedWorktree, error)

// DeliverFunc delivers the ticket contract to the newly started agent.
type DeliverFunc func(target, message string) error

// Service owns ticket attempt provisioning and conservative cleanup.
type Service struct {
	attempts      *attempts.Store
	launch        LaunchFunc
	deliver       DeliverFunc
	direct        DeliverFunc
	pullRequest   PullRequestFunc
	workspaceRoot string
}

var startMu sync.Mutex

// DefaultWorkspacesDir returns the root containing only Nagare-managed worktrees.
func DefaultWorkspacesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "nagare", "workspaces")
}

// NewService creates an orchestrator with explicit dependencies.
func NewService(store *attempts.Store, launch LaunchFunc, deliver DeliverFunc) *Service {
	return &Service{
		attempts:      store,
		launch:        launch,
		deliver:       deliver,
		direct:        session.SendPromptToPane,
		pullRequest:   githubPullRequest,
		workspaceRoot: DefaultWorkspacesDir(),
	}
}

// NewDefaultService creates the production ticket orchestrator.
func NewDefaultService() *Service {
	return NewService(attempts.NewStore(attempts.DefaultDir()), session.LaunchManagedWorktree, deliverWhenReady)
}

// Start creates one immutable attempt, launches it, and assigns the ticket.
func (s *Service) Start(store *tickets.Store, ticketID, agent string) (attempts.Attempt, error) {
	startMu.Lock()
	defer startMu.Unlock()

	ticket, err := store.Get(ticketID)
	if err != nil {
		return attempts.Attempt{}, err
	}
	if ticket.Status == tickets.StatusRunning || ticket.Status == tickets.StatusReview {
		return attempts.Attempt{}, fmt.Errorf("ticket %s is already %s", ticket.ID, ticket.Status)
	}
	if strings.TrimSpace(ticket.ProjectPath) == "" {
		return attempts.Attempt{}, fmt.Errorf("ticket has no repository; edit it and set a repository path")
	}
	mainRoot := git.MainRoot(ticket.ProjectPath)
	if mainRoot == "" {
		return attempts.Attempt{}, fmt.Errorf("%s is not a git repository", ticket.ProjectPath)
	}
	target := strings.TrimSpace(ticket.TargetBranch)
	if target == "" {
		target = git.DefaultBranch(mainRoot)
	}
	if target == "" {
		return attempts.Attempt{}, fmt.Errorf("repository has no target branch")
	}
	baseCommit, err := git.ResolveBaseCommit(mainRoot, target)
	if err != nil {
		return attempts.Attempt{}, err
	}

	attempt, err := s.attempts.Create(attempts.CreateInput{
		TicketID:     ticket.ID,
		Agent:        agent,
		ProjectPath:  mainRoot,
		TargetBranch: target,
	})
	if err != nil {
		return attempts.Attempt{}, err
	}
	shortTicket := shortID(ticket.ID)
	shortAttempt := shortID(attempt.ID)
	branch := "nagare/" + shortTicket + "-" + shortAttempt
	windowName := shortTicket + "-" + shortAttempt
	worktreePath := filepath.Join(s.workspaceRoot, attempt.ID, filepath.Base(mainRoot))
	attempt, err = s.attempts.Update(attempt.ID, func(current *attempts.Attempt) error {
		current.BaseCommit = baseCommit
		current.Branch = branch
		current.WorktreePath = worktreePath
		return nil
	})
	if err != nil {
		return attempts.Attempt{}, err
	}
	if _, err := store.Update(ticket.ID, func(current *tickets.Ticket) error {
		current.ActiveAttemptID = attempt.ID
		current.ProjectPath = mainRoot
		current.TargetBranch = target
		return nil
	}); err != nil {
		s.markFailed(attempt.ID, err)
		return attempts.Attempt{}, err
	}

	managed, err := s.launch(mainRoot, worktreePath, windowName, branch, baseCommit, agent)
	if err != nil {
		s.markFailed(attempt.ID, err)
		return attempts.Attempt{}, err
	}
	now := time.Now().UTC()
	attempt, err = s.attempts.Update(attempt.ID, func(current *attempts.Attempt) error {
		current.State = attempts.StateRunning
		current.SessionName = managed.DisplayName
		current.PaneID = managed.PaneID
		current.StartedAt = &now
		current.LastError = ""
		return nil
	})
	if err != nil {
		return attempts.Attempt{}, err
	}
	if _, err := store.Update(ticket.ID, func(current *tickets.Ticket) error {
		current.Status = tickets.StatusRunning
		current.PlannedFor = time.Now().Format(time.DateOnly)
		current.ActiveAttemptID = attempt.ID
		current.ProjectPath = mainRoot
		current.TargetBranch = target
		current.AssigneeSession = managed.DisplayName
		current.AssigneePaneID = managed.PaneID
		current.AssigneeAgent = agent
		current.ClearSubmission()
		return nil
	}); err != nil {
		s.markFailed(attempt.ID, err)
		return attempts.Attempt{}, err
	}
	prompt := AssignmentPrompt(ticket)
	if err := s.deliver(managed.DisplayName, prompt); err != nil {
		if directErr := s.direct(managed.PaneID, prompt); directErr != nil {
			combined := fmt.Errorf("mailbox delivery failed: %v; direct delivery failed: %w", err, directErr)
			_, _ = s.attempts.Update(attempt.ID, func(current *attempts.Attempt) error {
				current.LastError = combined.Error()
				return nil
			})
			return attempt, fmt.Errorf("agent started, but ticket delivery failed: %w; worktree kept at %s", combined, worktreePath)
		}
	}
	return attempt, nil
}

// Archive removes a completed ticket's clean managed worktree and retains its branch.
func (s *Service) Archive(store *tickets.Store, ticketID string) error {
	ticket, err := store.Get(ticketID)
	if err != nil {
		return err
	}
	if ticket.Status != tickets.StatusDone {
		return fmt.Errorf("only done tickets can be archived")
	}
	if ticket.ActiveAttemptID == "" {
		return fmt.Errorf("ticket has no active attempt")
	}
	attempt, err := s.attempts.Get(ticket.ActiveAttemptID)
	if err != nil {
		return err
	}
	if attempt.State != attempts.StateSubmitted {
		return fmt.Errorf("attempt is %s, not submitted", attempt.State)
	}
	if tmux.PaneExists(attempt.PaneID) {
		return fmt.Errorf("agent pane is still running; close it before archiving")
	}
	if err := managedPath(s.workspaceRoot, attempt.WorktreePath); err != nil {
		return err
	}
	if _, err := os.Stat(attempt.WorktreePath); err == nil {
		if git.MainRoot(attempt.WorktreePath) != filepath.Clean(attempt.ProjectPath) {
			return fmt.Errorf("managed worktree repository does not match its attempt")
		}
		if err := git.RemoveWorktree(attempt.WorktreePath); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	now := time.Now().UTC()
	if _, err := s.attempts.Update(attempt.ID, func(current *attempts.Attempt) error {
		current.State = attempts.StateArchived
		current.ArchivedAt = &now
		return nil
	}); err != nil {
		return err
	}
	_, err = store.Update(ticket.ID, func(current *tickets.Ticket) error {
		current.ActiveAttemptID = ""
		current.AssigneeSession = ""
		current.AssigneePaneID = ""
		current.AssigneeAgent = ""
		return nil
	})
	return err
}

// Reconcile marks attempts whose managed worktrees disappeared as failed. It
// never deletes directories, branches, or Git metadata.
func (s *Service) Reconcile(store *tickets.Store) error {
	all, err := s.attempts.List()
	if err != nil {
		return err
	}
	for _, attempt := range all {
		if attempt.State != attempts.StateProvisioning && attempt.State != attempts.StateRunning {
			continue
		}
		if attempt.WorktreePath == "" {
			if attempt.State == attempts.StateProvisioning && time.Since(attempt.CreatedAt) < 2*time.Minute {
				continue
			}
			s.failMissingAttempt(store, attempt, "provisioning did not record a worktree")
			continue
		}
		if err := managedPath(s.workspaceRoot, attempt.WorktreePath); err != nil {
			s.failMissingAttempt(store, attempt, err.Error())
			continue
		}
		if _, err := os.Stat(attempt.WorktreePath); os.IsNotExist(err) {
			if attempt.State == attempts.StateProvisioning && time.Since(attempt.CreatedAt) < 2*time.Minute {
				continue
			}
			s.failMissingAttempt(store, attempt, "managed worktree is missing")
		}
	}
	return nil
}

func (s *Service) failMissingAttempt(store *tickets.Store, attempt attempts.Attempt, reason string) {
	s.markFailed(attempt.ID, fmt.Errorf("%s", reason))
	ticket, err := store.Get(attempt.TicketID)
	if err != nil || ticket.ActiveAttemptID != attempt.ID {
		return
	}
	_, _ = store.Update(ticket.ID, func(current *tickets.Ticket) error {
		if current.ActiveAttemptID != attempt.ID {
			return nil
		}
		if current.Status == tickets.StatusRunning {
			current.Status = tickets.StatusReady
		}
		current.AssigneeSession = ""
		current.AssigneePaneID = ""
		current.AssigneeAgent = ""
		return nil
	})
}

// AssignmentPrompt is the execution contract sent to a ticket agent.
func AssignmentPrompt(ticket tickets.Ticket) string {
	var description string
	if ticket.Description != "" {
		description = "\n\nDescription and acceptance criteria:\n" + ticket.Description
	}
	return fmt.Sprintf("You have been assigned Nagare ticket %s: %s%s\n\nWork only in the current worktree. Do not modify the source checkout, target branch, or remote branches directly. When the requested outcome is implemented and verified, call submit_ticket with ticket_id %q. The summary must explain what changed and how it was verified; Nagare records your agent, session, repository, and submission time automatically. Do not mark the ticket done; it requires human review.", ticket.ID, ticket.Title, description, ticket.ID)
}

func (s *Service) markFailed(id string, cause error) {
	_, _ = s.attempts.Update(id, func(current *attempts.Attempt) error {
		current.State = attempts.StateFailed
		current.LastError = cause.Error()
		return nil
	})
}

func deliverWhenReady(target, message string) error {
	deadline := time.Now().Add(45 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		last = mcp.SendMessageHandler("nagare-board", mcp.SendMessageInput{Target: target, Message: message})
		if !strings.HasPrefix(last, "Error") {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("%s", last)
}

func managedPath(workspaceRoot, path string) error {
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return err
	}
	candidate, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing unmanaged worktree path %s", path)
	}
	return nil
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
