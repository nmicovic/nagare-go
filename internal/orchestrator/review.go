package orchestrator

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/nemke/nagare-go/internal/attempts"
	"github.com/nemke/nagare-go/internal/git"
	"github.com/nemke/nagare-go/internal/tickets"
)

// Review is the human-reviewable state of one submitted attempt.
type Review struct {
	Attempt attempts.Attempt
	Git     git.Review
}

// PullRequest is the durable identity returned by a Git hosting provider.
type PullRequest struct {
	URL    string
	Number int
	State  string
}

// PullRequestFunc pushes and creates or finds a pull request.
type PullRequestFunc func(worktreePath, branch, target, title, body string) (PullRequest, error)

// Review loads the tracked diff from a managed attempt's immutable base.
func (s *Service) Review(store *tickets.Store, ticketID string) (Review, error) {
	ticket, err := store.Get(ticketID)
	if err != nil {
		return Review{}, err
	}
	if ticket.ActiveAttemptID == "" {
		return Review{}, fmt.Errorf("ticket has no active attempt")
	}
	attempt, err := s.attempts.Get(ticket.ActiveAttemptID)
	if err != nil {
		return Review{}, err
	}
	if attempt.State != attempts.StateSubmitted {
		return Review{}, fmt.Errorf("attempt is %s, not submitted", attempt.State)
	}
	if err := managedPath(s.workspaceRoot, attempt.WorktreePath); err != nil {
		return Review{}, err
	}
	if git.MainRoot(attempt.WorktreePath) != attempt.ProjectPath {
		return Review{}, fmt.Errorf("managed worktree repository does not match its attempt")
	}
	review, err := git.ReviewWorktree(attempt.WorktreePath, attempt.Branch, attempt.BaseCommit)
	if err != nil {
		return Review{}, err
	}
	return Review{Attempt: attempt, Git: review}, nil
}

// CreatePullRequest safely pushes the recorded branch and creates or recovers a
// GitHub pull request for a submitted ticket.
func (s *Service) CreatePullRequest(store *tickets.Store, ticketID string) (PullRequest, error) {
	ticket, err := store.Get(ticketID)
	if err != nil {
		return PullRequest{}, err
	}
	if ticket.Status != tickets.StatusReview {
		return PullRequest{}, fmt.Errorf("ticket is %s, not review", ticket.Status)
	}
	review, err := s.Review(store, ticket.ID)
	if err != nil {
		return PullRequest{}, err
	}
	attempt := review.Attempt
	if attempt.PullRequestURL != "" {
		pr := PullRequest{URL: attempt.PullRequestURL, Number: attempt.PullRequestNumber, State: attempt.PullRequestState}
		if _, err := store.Update(ticket.ID, func(current *tickets.Ticket) error {
			current.PullRequestURL = pr.URL
			current.PullRequestNumber = pr.Number
			return nil
		}); err != nil {
			return PullRequest{}, err
		}
		return pr, nil
	}
	if review.Git.DirtyFiles != 0 {
		return PullRequest{}, fmt.Errorf("worktree has %d uncommitted change(s); commit them before creating a PR", review.Git.DirtyFiles)
	}
	if review.Git.Commits == 0 {
		return PullRequest{}, fmt.Errorf("attempt has no commits beyond its base")
	}
	target, err := git.PullRequestBase(attempt.TargetBranch)
	if err != nil {
		return PullRequest{}, err
	}
	body := pullRequestBody(ticket, attempt)
	pr, err := s.pullRequest(attempt.WorktreePath, attempt.Branch, target, ticket.Title, body)
	if err != nil {
		return PullRequest{}, err
	}
	if strings.TrimSpace(pr.URL) == "" || pr.Number <= 0 {
		return PullRequest{}, fmt.Errorf("pull request provider returned incomplete identity")
	}
	createdAt := time.Now().UTC()
	if _, err := s.attempts.Update(attempt.ID, func(current *attempts.Attempt) error {
		current.PullRequestURL = pr.URL
		current.PullRequestNumber = pr.Number
		current.PullRequestState = pr.State
		current.PullRequestAt = &createdAt
		return nil
	}); err != nil {
		return PullRequest{}, fmt.Errorf("PR created but attempt provenance failed: %w", err)
	}
	if _, err := store.Update(ticket.ID, func(current *tickets.Ticket) error {
		current.PullRequestURL = pr.URL
		current.PullRequestNumber = pr.Number
		return nil
	}); err != nil {
		return PullRequest{}, fmt.Errorf("PR created but ticket provenance failed: %w", err)
	}
	return pr, nil
}

func pullRequestBody(ticket tickets.Ticket, attempt attempts.Attempt) string {
	summary := strings.TrimSpace(ticket.SubmittedSummary)
	if summary == "" {
		summary = "Changes submitted for human review."
	}
	return fmt.Sprintf("## Summary\n\n%s\n\n---\nNagare ticket: `%s`\nBase commit: `%s`", summary, ticket.ID, attempt.BaseCommit)
}

func githubPullRequest(worktreePath, branch, target, title, body string) (PullRequest, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return PullRequest{}, fmt.Errorf("gh executable not found in PATH")
	}
	if err := git.PushBranch(worktreePath, branch); err != nil {
		return PullRequest{}, err
	}
	if pr, found := lookupGitHubPullRequest(worktreePath, branch); found {
		return pr, nil
	}
	cmd := exec.Command("gh", "pr", "create",
		"--base", target,
		"--head", branch,
		"--title", title,
		"--body", body,
	)
	cmd.Dir = worktreePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		if pr, found := lookupGitHubPullRequest(worktreePath, branch); found {
			return pr, nil
		}
		return PullRequest{}, fmt.Errorf("gh pr create: %w: %s", err, strings.TrimSpace(string(out)))
	}
	url := strings.TrimSpace(string(out))
	if pr, found := lookupGitHubPullRequest(worktreePath, branch); found {
		return pr, nil
	}
	if url == "" {
		return PullRequest{}, fmt.Errorf("gh pr create returned no URL")
	}
	return PullRequest{URL: url, Number: pullRequestNumber(url), State: "OPEN"}, nil
}

func lookupGitHubPullRequest(worktreePath, branch string) (PullRequest, bool) {
	cmd := exec.Command("gh", "pr", "view", branch, "--json", "url,number,state")
	cmd.Dir = worktreePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return PullRequest{}, false
	}
	var pr PullRequest
	if err := json.Unmarshal(out, &pr); err != nil || pr.URL == "" {
		return PullRequest{}, false
	}
	return pr, true
}

func pullRequestNumber(url string) int {
	marker := "/pull/"
	index := strings.LastIndex(url, marker)
	if index < 0 {
		return 0
	}
	number, _ := strconv.Atoi(strings.Trim(strings.TrimSpace(url[index+len(marker):]), "/"))
	return number
}
