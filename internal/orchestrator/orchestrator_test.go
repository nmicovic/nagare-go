package orchestrator

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nemke/nagare-go/internal/attempts"
	"github.com/nemke/nagare-go/internal/git"
	"github.com/nemke/nagare-go/internal/session"
	"github.com/nemke/nagare-go/internal/tickets"
)

func initMainRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	commands := [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "."},
		{"commit", "-qm", "initial"},
	}
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range commands {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return repo
}

func createReadyTicket(t *testing.T, store *tickets.Store, repo string) tickets.Ticket {
	t.Helper()
	ticket, err := store.Create(tickets.CreateInput{
		Title:        "Implement isolation",
		Description:  "Keep the source checkout unchanged.",
		ProjectPath:  repo,
		TargetBranch: "main",
		Status:       tickets.StatusReady,
		Priority:     tickets.PriorityHigh,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ticket
}

func TestStartPersistsAttemptAndAssignsDedicatedSession(t *testing.T) {
	repo := initMainRepo(t)
	ticketStore := tickets.NewStore(t.TempDir())
	ticket := createReadyTicket(t, ticketStore, repo)
	attemptStore := attempts.NewStore(t.TempDir())
	workspaceRoot := t.TempDir()

	var delivered string
	service := NewService(attemptStore,
		func(repoPath, worktreePath, windowName, branch, baseCommit, agent string) (session.ManagedWorktree, error) {
			if repoPath != repo || !strings.HasPrefix(worktreePath, workspaceRoot+string(filepath.Separator)) {
				t.Fatalf("launch paths = %q, %q", repoPath, worktreePath)
			}
			if agent != "codex" || !strings.HasPrefix(branch, "nagare/") || len(baseCommit) != 40 {
				t.Fatalf("launch identity = %q, %q, %q", agent, branch, baseCommit)
			}
			return session.ManagedWorktree{DisplayName: "repo/attempt", PaneID: "%7"}, nil
		},
		func(target, message string) error {
			delivered = target + "\n" + message
			return nil
		})
	service.workspaceRoot = workspaceRoot

	attempt, err := service.Start(ticketStore, ticket.ID, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if attempt.State != attempts.StateRunning || attempt.SessionName != "repo/attempt" || attempt.PaneID != "%7" {
		t.Fatalf("attempt = %#v", attempt)
	}
	updated, err := ticketStore.Get(ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != tickets.StatusRunning || updated.ActiveAttemptID != attempt.ID || updated.AssigneePaneID != "%7" {
		t.Fatalf("ticket = %#v", updated)
	}
	for _, want := range []string{"repo/attempt", ticket.ID, "current worktree", "Do not modify the source checkout", "submit_ticket"} {
		if !strings.Contains(delivered, want) {
			t.Errorf("delivery missing %q: %s", want, delivered)
		}
	}
}

func TestStartFailureKeepsTicketRetryableAndRecordsAttemptError(t *testing.T) {
	repo := initMainRepo(t)
	ticketStore := tickets.NewStore(t.TempDir())
	ticket := createReadyTicket(t, ticketStore, repo)
	attemptStore := attempts.NewStore(t.TempDir())
	service := NewService(attemptStore,
		func(string, string, string, string, string, string) (session.ManagedWorktree, error) {
			return session.ManagedWorktree{}, errors.New("tmux failed")
		},
		func(string, string) error { return nil })
	service.workspaceRoot = t.TempDir()

	if _, err := service.Start(ticketStore, ticket.ID, "claude"); err == nil {
		t.Fatal("Start() = nil error")
	}
	updated, err := ticketStore.Get(ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != tickets.StatusReady || updated.ActiveAttemptID == "" {
		t.Fatalf("failed ticket = %#v", updated)
	}
	attempt, err := attemptStore.Get(updated.ActiveAttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.State != attempts.StateFailed || !strings.Contains(attempt.LastError, "tmux failed") {
		t.Fatalf("failed attempt = %#v", attempt)
	}
}

func TestDeliveryFailureKeepsStartedAttemptRunningAndRecoverable(t *testing.T) {
	repo := initMainRepo(t)
	ticketStore := tickets.NewStore(t.TempDir())
	ticket := createReadyTicket(t, ticketStore, repo)
	attemptStore := attempts.NewStore(t.TempDir())
	service := NewService(attemptStore,
		func(string, string, string, string, string, string) (session.ManagedWorktree, error) {
			return session.ManagedWorktree{DisplayName: "repo/attempt", PaneID: "%8"}, nil
		},
		func(string, string) error { return errors.New("mailbox unavailable") })
	service.direct = func(string, string) error { return errors.New("pane unavailable") }
	service.workspaceRoot = t.TempDir()

	attempt, err := service.Start(ticketStore, ticket.ID, "codex")
	if err == nil {
		t.Fatal("Start() = nil error")
	}
	if attempt.ID == "" || attempt.State != attempts.StateRunning {
		t.Fatalf("started attempt = %#v", attempt)
	}
	storedAttempt, getErr := attemptStore.Get(attempt.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if storedAttempt.State != attempts.StateRunning || !strings.Contains(storedAttempt.LastError, "direct delivery failed") {
		t.Fatalf("stored attempt = %#v", storedAttempt)
	}
	updatedTicket, getErr := ticketStore.Get(ticket.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if updatedTicket.Status != tickets.StatusRunning || updatedTicket.AssigneePaneID != "%8" {
		t.Fatalf("started ticket = %#v", updatedTicket)
	}
}

func TestArchiveRefusesDirtyWorktreeThenRemovesCleanOneAndKeepsBranch(t *testing.T) {
	repo := initMainRepo(t)
	base, err := git.ResolveBaseCommit(repo, "main")
	if err != nil {
		t.Fatal(err)
	}
	workspaceRoot := t.TempDir()
	worktreePath := filepath.Join(workspaceRoot, "attempt", "repo")
	branch := "nagare/archive-test"
	if err := git.AddManagedWorktree(repo, worktreePath, branch, base); err != nil {
		t.Fatal(err)
	}

	attemptStore := attempts.NewStore(t.TempDir())
	attempt, err := attemptStore.Create(attempts.CreateInput{TicketID: "ticket", Agent: "codex", ProjectPath: repo, TargetBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err = attemptStore.Update(attempt.ID, func(current *attempts.Attempt) error {
		current.State = attempts.StateSubmitted
		current.BaseCommit = base
		current.Branch = branch
		current.WorktreePath = worktreePath
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ticketStore := tickets.NewStore(t.TempDir())
	ticket, err := ticketStore.Create(tickets.CreateInput{Title: "Done", ProjectPath: repo, TargetBranch: "main", Status: tickets.StatusDone, Priority: tickets.PriorityMedium})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ticketStore.Update(ticket.ID, func(current *tickets.Ticket) error {
		current.ActiveAttemptID = attempt.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := NewService(attemptStore, nil, nil)
	service.workspaceRoot = workspaceRoot

	dirty := filepath.Join(worktreePath, "dirty.txt")
	if err := os.WriteFile(dirty, []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := service.Archive(ticketStore, ticket.ID); err == nil {
		t.Fatal("Archive() removed dirty worktree")
	}
	if _, err := os.Stat(worktreePath); err != nil {
		t.Fatal("dirty worktree disappeared")
	}
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	if err := service.Archive(ticketStore, ticket.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatal("clean worktree still exists")
	}
	cmd := exec.Command("git", "-C", repo, "branch", "--list", branch)
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Fatalf("attempt branch was lost: %q, %v", out, err)
	}
}

func TestReconcileMakesMissingRunningAttemptRetryableWithoutDeletingAnything(t *testing.T) {
	workspaceRoot := t.TempDir()
	repo := initMainRepo(t)
	attemptStore := attempts.NewStore(t.TempDir())
	attempt, err := attemptStore.Create(attempts.CreateInput{
		TicketID:     "ticket",
		Agent:        "codex",
		ProjectPath:  repo,
		TargetBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(workspaceRoot, attempt.ID, "repo")
	attempt, err = attemptStore.Update(attempt.ID, func(current *attempts.Attempt) error {
		current.State = attempts.StateRunning
		current.WorktreePath = missingPath
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ticketStore := tickets.NewStore(t.TempDir())
	ticket, err := ticketStore.Create(tickets.CreateInput{
		Title: "Recover me", ProjectPath: repo, TargetBranch: "main",
		Status: tickets.StatusRunning, Priority: tickets.PriorityMedium,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ticketStore.Update(ticket.ID, func(current *tickets.Ticket) error {
		current.ActiveAttemptID = attempt.ID
		current.AssigneeSession = "repo/attempt"
		current.AssigneePaneID = "%9"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Point the fixture attempt at the actual ticket after both records exist.
	if _, err := attemptStore.Update(attempt.ID, func(current *attempts.Attempt) error {
		current.TicketID = ticket.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	service := NewService(attemptStore, nil, nil)
	service.workspaceRoot = workspaceRoot
	if err := service.Reconcile(ticketStore); err != nil {
		t.Fatal(err)
	}
	updatedAttempt, err := attemptStore.Get(attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedAttempt.State != attempts.StateFailed || !strings.Contains(updatedAttempt.LastError, "missing") {
		t.Fatalf("reconciled attempt = %#v", updatedAttempt)
	}
	updatedTicket, err := ticketStore.Get(ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedTicket.Status != tickets.StatusReady || updatedTicket.AssigneeSession != "" || updatedTicket.AssigneePaneID != "" {
		t.Fatalf("reconciled ticket = %#v", updatedTicket)
	}
}

func TestManagedPathRejectsAnythingOutsideOwnedRoot(t *testing.T) {
	root := t.TempDir()
	if err := managedPath(root, filepath.Join(root, "attempt", "repo")); err != nil {
		t.Fatalf("owned path rejected: %v", err)
	}
	if err := managedPath(root, filepath.Dir(root)); err == nil {
		t.Fatal("parent path accepted as managed")
	}
	if err := managedPath(root, root); err == nil {
		t.Fatal("workspace root accepted as removable worktree")
	}
}
