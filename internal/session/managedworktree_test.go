package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nemke/nagare-go/internal/git"
)

func managedRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-qm", "initial"}} {
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

func fakeTmux(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  list-sessions) exit 0 ;;
  new-session) printf '%%42\n'; exit 0 ;;
  send-keys) [ -n "$FAIL_SEND" ] && { echo send-failed >&2; exit 1; }; exit 0 ;;
  *) exit 0 ;;
esac
`
	path := filepath.Join(binDir, "tmux")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(binDir, agent), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())
}

func TestLaunchManagedWorktreeOwnsCreationForEveryAgent(t *testing.T) {
	fakeTmux(t)
	repo := managedRepo(t)
	base, err := git.ResolveBaseCommit(repo, "main")
	if err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(t.TempDir(), "workspace", "repo")
	managed, err := LaunchManagedWorktree(repo, worktree, "ticket-attempt", "nagare/ticket-attempt", base, AgentSpec{Agent: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if managed.PaneID != "%42" || managed.DisplayName != "repo/ticket-attempt" {
		t.Fatalf("managed session = %#v", managed)
	}
	if got := git.Describe(worktree).Branch; got != "nagare/ticket-attempt" {
		t.Fatalf("worktree branch = %q", got)
	}
}

func TestLaunchFailureAfterGitCreationKeepsWorktreeForRecovery(t *testing.T) {
	fakeTmux(t)
	t.Setenv("FAIL_SEND", "1")
	repo := managedRepo(t)
	base, err := git.ResolveBaseCommit(repo, "main")
	if err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(t.TempDir(), "workspace", "repo")
	if _, err := LaunchManagedWorktree(repo, worktree, "ticket-attempt", "nagare/ticket-attempt", base, AgentSpec{Agent: "codex"}); err == nil {
		t.Fatal("LaunchManagedWorktree() = nil error")
	}
	if _, err := os.Stat(worktree); err != nil {
		t.Fatalf("worktree was deleted after tmux failure: %v", err)
	}
	if got := git.Describe(worktree).Branch; got != "nagare/ticket-attempt" {
		t.Fatalf("recoverable branch = %q", got)
	}
}
