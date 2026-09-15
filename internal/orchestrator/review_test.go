package orchestrator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nemke/nagare-go/internal/attempts"
	"github.com/nemke/nagare-go/internal/git"
	"github.com/nemke/nagare-go/internal/tickets"
)

func submittedReviewFixture(t *testing.T) (*Service, *tickets.Store, *attempts.Store, tickets.Ticket, attempts.Attempt) {
	t.Helper()
	repo := initMainRepo(t)
	base, err := git.ResolveBaseCommit(repo, "main")
	if err != nil {
		t.Fatal(err)
	}
	workspaceRoot := t.TempDir()
	worktree := filepath.Join(workspaceRoot, "attempt", "repo")
	branch := "nagare/review-pr"
	if err := git.AddManagedWorktree(repo, worktree, branch, base); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "file.txt"), []byte("reviewed change"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, worktree, "reviewed change")

	attemptStore := attempts.NewStore(t.TempDir())
	attempt, err := attemptStore.Create(attempts.CreateInput{
		TicketID: "placeholder", Agent: "codex", ProjectPath: repo, TargetBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err = attemptStore.Update(attempt.ID, func(current *attempts.Attempt) error {
		current.State = attempts.StateSubmitted
		current.BaseCommit = base
		current.Branch = branch
		current.WorktreePath = worktree
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ticketStore := tickets.NewStore(t.TempDir())
	ticket, err := ticketStore.Create(tickets.CreateInput{
		Title: "Open a safe PR", ProjectPath: repo, TargetBranch: "main",
		Status: tickets.StatusReview, Priority: tickets.PriorityHigh,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ticketStore.Update(ticket.ID, func(current *tickets.Ticket) error {
		current.ActiveAttemptID = attempt.ID
		current.SubmittedSummary = "Implemented the reviewed change and ran tests."
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	attempt, err = attemptStore.Update(attempt.ID, func(current *attempts.Attempt) error {
		current.TicketID = ticket.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(attemptStore, nil, nil)
	service.workspaceRoot = workspaceRoot
	return service, ticketStore, attemptStore, ticket, attempt
}

func gitCommit(t *testing.T, dir, message string) {
	t.Helper()
	for _, args := range [][]string{{"add", "."}, {"commit", "-qm", message}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestReviewLoadsSubmittedAttemptDiff(t *testing.T) {
	service, ticketStore, _, ticket, attempt := submittedReviewFixture(t)
	review, err := service.Review(ticketStore, ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if review.Attempt.ID != attempt.ID || review.Git.Commits != 1 || review.Git.DirtyFiles != 0 {
		t.Fatalf("review = %#v", review)
	}
	if !strings.Contains(review.Git.Stat, "file.txt") || !strings.Contains(review.Git.Diff, "+reviewed change") {
		t.Fatalf("review output missing change: %#v", review.Git)
	}
}

func TestCreatePullRequestPersistsAndRecoversProvenance(t *testing.T) {
	service, ticketStore, attemptStore, ticket, attempt := submittedReviewFixture(t)
	calls := 0
	service.pullRequest = func(worktreePath, branch, target, title, body string) (PullRequest, error) {
		calls++
		if target != "main" || title != ticket.Title || branch != attempt.Branch {
			t.Fatalf("PR identity = %q, %q, %q", target, title, branch)
		}
		for _, want := range []string{"Implemented the reviewed change", ticket.ID, attempt.BaseCommit} {
			if !strings.Contains(body, want) {
				t.Errorf("PR body missing %q: %s", want, body)
			}
		}
		return PullRequest{URL: "https://github.com/acme/repo/pull/42", Number: 42, State: "OPEN"}, nil
	}

	pr, err := service.CreatePullRequest(ticketStore, ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 42 || calls != 1 {
		t.Fatalf("PR = %#v, calls = %d", pr, calls)
	}
	storedAttempt, err := attemptStore.Get(attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedAttempt.PullRequestURL != pr.URL || storedAttempt.PullRequestAt == nil {
		t.Fatalf("attempt PR = %#v", storedAttempt)
	}
	storedTicket, err := ticketStore.Get(ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedTicket.PullRequestURL != pr.URL || storedTicket.PullRequestNumber != 42 {
		t.Fatalf("ticket PR = %#v", storedTicket)
	}

	// A retry after creation repairs/returns persisted provenance without another remote action.
	recovered, err := service.CreatePullRequest(ticketStore, ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.URL != pr.URL || calls != 1 {
		t.Fatalf("recovered PR = %#v, calls = %d", recovered, calls)
	}
}

func TestCreatePullRequestRefusesDirtySubmittedWorktree(t *testing.T) {
	service, ticketStore, _, ticket, attempt := submittedReviewFixture(t)
	if err := os.WriteFile(filepath.Join(attempt.WorktreePath, "dirty.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	called := false
	service.pullRequest = func(string, string, string, string, string) (PullRequest, error) {
		called = true
		return PullRequest{}, nil
	}
	if _, err := service.CreatePullRequest(ticketStore, ticket.ID); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("dirty PR error = %v", err)
	}
	if called {
		t.Fatal("PR provider called for dirty worktree")
	}
}
func TestCreatePullRequestRefusesAttemptWithoutCommits(t *testing.T) {
	service, ticketStore, _, ticket, attempt := submittedReviewFixture(t)
	runGitTestCommand(t, attempt.WorktreePath, "reset", "--hard", attempt.BaseCommit)
	called := false
	service.pullRequest = func(string, string, string, string, string) (PullRequest, error) {
		called = true
		return PullRequest{}, nil
	}
	if _, err := service.CreatePullRequest(ticketStore, ticket.ID); err == nil || !strings.Contains(err.Error(), "no commits") {
		t.Fatalf("empty PR error = %v", err)
	}
	if called {
		t.Fatal("PR provider called for an attempt without commits")
	}
}

func TestGitHubPullRequestUsesExplicitRefsAndRecoversCreatedPR(t *testing.T) {
	_, _, _, _, attempt := submittedReviewFixture(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, remote, "init", "-q", "--bare")
	runGitTestCommand(t, attempt.ProjectPath, "remote", "add", "origin", remote)

	binDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "pr.json")
	argsPath := filepath.Join(t.TempDir(), "args")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$GH_ARGS"
if [ "$1" = "pr" ] && [ "$2" = "view" ]; then
  if [ -f "$GH_STATE" ]; then
    cat "$GH_STATE"
    exit 0
  fi
  exit 1
fi
if [ "$1" = "pr" ] && [ "$2" = "create" ]; then
  printf '{"url":"https://github.com/acme/repo/pull/73","number":73,"state":"OPEN"}' > "$GH_STATE"
  if [ "$GH_CREATE_FAIL" = "1" ]; then
    exit 1
  fi
  printf 'https://github.com/acme/repo/pull/73\n'
  exit 0
fi
exit 2
`
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_ARGS", argsPath)
	t.Setenv("GH_STATE", statePath)

	pr, err := githubPullRequest(attempt.WorktreePath, attempt.Branch, "main", "Safe title", "Safe body")
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 73 || pr.URL == "" {
		t.Fatalf("PR = %#v", pr)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	calls := string(args)
	for _, want := range []string{
		"pr create --base main --head " + attempt.Branch,
		"--title Safe title --body Safe body",
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("gh calls missing %q:\n%s", want, calls)
		}
	}
	if strings.Count(calls, "pr create") != 1 {
		t.Fatalf("create call count:\n%s", calls)
	}

	// Recover when gh created the PR remotely but returned an error locally.
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_CREATE_FAIL", "1")
	recovered, err := githubPullRequest(attempt.WorktreePath, attempt.Branch, "main", "Safe title", "Safe body")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Number != 73 {
		t.Fatalf("recovered PR = %#v", recovered)
	}
}

func runGitTestCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
