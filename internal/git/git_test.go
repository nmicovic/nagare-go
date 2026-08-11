package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseRevParse(t *testing.T) {
	tests := []struct {
		name string
		out  string
		dir  string
		want Repo
	}{
		{
			name: "main checkout has a relative common dir",
			out:  ".git\n/home/u/Projects/app\ndev\n",
			dir:  "/home/u/Projects/app",
			want: Repo{Branch: "dev", RepoName: "app"},
		},
		{
			name: "linked worktree",
			out:  "/home/u/Projects/app/.git\n/home/u/Projects/app/.claude/worktrees/the-site\nworktree-the-site\n",
			dir:  "/home/u/Projects/app/.claude/worktrees/the-site",
			want: Repo{Branch: "worktree-the-site", RepoName: "app", Worktree: "the-site", IsWorktree: true},
		},
		{
			name: "detached HEAD reports no branch",
			out:  ".git\n/home/u/Projects/app\nHEAD\n",
			dir:  "/home/u/Projects/app",
			want: Repo{RepoName: "app"},
		},
		{
			name: "empty output yields the zero value",
			out:  "",
			dir:  "/tmp",
			want: Repo{},
		},
		{
			name: "truncated output yields the zero value",
			out:  ".git\n",
			dir:  "/tmp",
			want: Repo{},
		},
	}
	for _, tt := range tests {
		got := parseRevParse(tt.out, tt.dir)
		if got != tt.want {
			t.Errorf("%s: parseRevParse() = %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestDescribeRealRepoAndWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	repoDir := filepath.Join(root, "app")

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		// Keep the sandbox free of the developer's real git identity/hooks.
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	run(repoDir, "init", "-q", "-b", "dev")
	if err := os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	run(repoDir, "add", ".")
	run(repoDir, "commit", "-qm", "init")

	wtDir := filepath.Join(repoDir, ".claude", "worktrees", "the-site")
	run(repoDir, "worktree", "add", "-q", "-b", "worktree-the-site", wtDir)

	main := Describe(repoDir)
	if main.IsWorktree {
		t.Errorf("main checkout reported as a worktree: %+v", main)
	}
	if main.RepoName != "app" || main.Branch != "dev" || main.Worktree != "" {
		t.Errorf("main checkout = %+v", main)
	}

	wt := Describe(wtDir)
	if !wt.IsWorktree {
		t.Errorf("linked worktree not detected: %+v", wt)
	}
	if wt.RepoName != "app" || wt.Worktree != "the-site" || wt.Branch != "worktree-the-site" {
		t.Errorf("worktree = %+v", wt)
	}

	// A detached worktree HEAD must report no branch, not "HEAD".
	run(wtDir, "checkout", "-q", "--detach")
	if got := Describe(wtDir).Branch; got != "" {
		t.Errorf("detached HEAD branch = %q, want empty", got)
	}

	// A directory outside any repository must not panic or invent facts.
	if got := Describe(root); got != (Repo{}) {
		t.Errorf("non-repo = %+v, want zero value", got)
	}
}
