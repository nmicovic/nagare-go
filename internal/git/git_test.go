package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// gitRun runs git in dir, failing the test on error. The environment is stripped
// of the developer's real git identity and hooks so the sandbox is reproducible.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
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

// initRepo creates a git repository at dir on branch "dev" with one commit.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "init", "-q", "-b", "dev")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-qm", "init")
}

func TestDescribeRealRepoAndWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	repoDir := filepath.Join(root, "app")
	initRepo(t, repoDir)

	wtDir := filepath.Join(repoDir, ".claude", "worktrees", "the-site")
	gitRun(t, repoDir, "worktree", "add", "-q", "-b", "worktree-the-site", wtDir)

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
	gitRun(t, wtDir, "checkout", "-q", "--detach")
	if got := Describe(wtDir).Branch; got != "" {
		t.Errorf("detached HEAD branch = %q, want empty", got)
	}

	// A directory outside any repository must not panic or invent facts.
	if got := Describe(root); got != (Repo{}) {
		t.Errorf("non-repo = %+v, want zero value", got)
	}
}

func TestValidWorktreeName(t *testing.T) {
	valid := []string{"the-site", "shipping", "feat_2", "a", "v1.2"}
	for _, name := range valid {
		if err := ValidWorktreeName(name); err != nil {
			t.Errorf("ValidWorktreeName(%q) = %v, want nil", name, err)
		}
	}
	invalid := []string{"", "   ", "a/b", `a\b`, "..", ".", "-x", "a b", "a:b", "a~b"}
	for _, name := range invalid {
		if err := ValidWorktreeName(name); err == nil {
			t.Errorf("ValidWorktreeName(%q) = nil, want an error", name)
		}
	}
}

func TestAddWorktreeAndMainRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	repoDir := filepath.Join(t.TempDir(), "app")
	initRepo(t, repoDir)

	path, err := AddWorktree(repoDir, "the-site")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(repoDir, ".worktrees", "the-site")
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("worktree directory missing: %v", err)
	}

	// The new worktree must be detected as one, on its own branch.
	got := Describe(path)
	if !got.IsWorktree || got.Worktree != "the-site" || got.Branch != "the-site" {
		t.Errorf("Describe(worktree) = %+v", got)
	}

	// MainRoot resolves back to the main checkout from either side.
	if r := MainRoot(path); r != repoDir {
		t.Errorf("MainRoot(worktree) = %q, want %q", r, repoDir)
	}
	if r := MainRoot(repoDir); r != repoDir {
		t.Errorf("MainRoot(main) = %q, want %q", r, repoDir)
	}

	// A duplicate name must fail rather than silently reuse.
	if _, err := AddWorktree(repoDir, "the-site"); err == nil {
		t.Error("AddWorktree with a duplicate name = nil, want an error")
	}
}

func TestMainRootOutsideRepo(t *testing.T) {
	if got := MainRoot(t.TempDir()); got != "" {
		t.Errorf("MainRoot(non-repo) = %q, want empty", got)
	}
}

func TestWorkStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repoDir := filepath.Join(t.TempDir(), "app")
	initRepo(t, repoDir)

	// Clean checkout, no upstream configured.
	got := WorkStatus(repoDir)
	if got.Dirty != 0 {
		t.Errorf("clean repo Dirty = %d, want 0", got.Dirty)
	}
	if got.HasUpstream {
		t.Error("HasUpstream = true, but no remote is configured")
	}

	// Two changed files: one modified, one untracked.
	if err := os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "new.txt"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := WorkStatus(repoDir); got.Dirty != 2 {
		t.Errorf("Dirty = %d, want 2", got.Dirty)
	}

	// A non-repo must not report phantom work.
	if got := WorkStatus(t.TempDir()); got != (Work{}) {
		t.Errorf("non-repo WorkStatus = %+v, want zero value", got)
	}
}

func TestRemoveWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repoDir := filepath.Join(t.TempDir(), "app")
	initRepo(t, repoDir)

	path, err := AddWorktree(repoDir, "throwaway")
	if err != nil {
		t.Fatal(err)
	}

	// Uncommitted work must block removal — that is the whole guard.
	if err := os.WriteFile(filepath.Join(path, "wip.txt"), []byte("wip"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWorktree(path); err == nil {
		t.Error("RemoveWorktree with uncommitted work = nil, want an error")
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("worktree was removed despite uncommitted work")
	}

	// Clean it and removal succeeds; the branch survives.
	if err := os.Remove(filepath.Join(path, "wip.txt")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWorktree(path); err != nil {
		t.Fatalf("RemoveWorktree on a clean worktree: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("worktree directory still exists after removal")
	}
	out, err := exec.Command("git", "-C", repoDir, "branch", "--list", "throwaway").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Error("branch was deleted; commits on it would be lost")
	}
}

// Claude Code locks the worktrees it creates, and git refuses to remove a locked
// worktree without --force — which would also defeat the dirty guard. Removal
// must therefore handle a locked-but-clean worktree, and still refuse a dirty one.
func TestRemoveWorktreeLocked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repoDir := filepath.Join(t.TempDir(), "app")
	initRepo(t, repoDir)

	path, err := AddWorktree(repoDir, "locked-wt")
	if err != nil {
		t.Fatal(err)
	}
	gitRun(t, repoDir, "worktree", "lock", path)

	// Locked and dirty: still refused, and nothing is deleted.
	if err := os.WriteFile(filepath.Join(path, "wip.txt"), []byte("wip"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWorktree(path); err == nil {
		t.Error("RemoveWorktree on a locked, dirty worktree = nil, want an error")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("worktree was removed despite uncommitted work")
	}

	// Locked and clean: removal succeeds.
	if err := os.Remove(filepath.Join(path, "wip.txt")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWorktree(path); err != nil {
		t.Fatalf("RemoveWorktree on a locked, clean worktree: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("locked worktree still exists after removal")
	}
}

func TestManagedWorktreeUsesExplicitBaseWithoutTouchingSourceCheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	repoDir := filepath.Join(root, "app")
	initRepo(t, repoDir)
	gitRun(t, repoDir, "branch", "main")

	if err := os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("dev"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repoDir, "commit", "-qam", "dev change")
	if err := os.WriteFile(filepath.Join(repoDir, "dirty.txt"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	base, err := ResolveBaseCommit(repoDir, "main")
	if err != nil {
		t.Fatal(err)
	}
	worktreePath := filepath.Join(root, "managed", "app")
	if err := AddManagedWorktree(repoDir, worktreePath, "nagare/ticket-attempt", base); err != nil {
		t.Fatal(err)
	}

	if got := Describe(repoDir).Branch; got != "dev" {
		t.Errorf("source branch = %q, want dev", got)
	}
	if data, err := os.ReadFile(filepath.Join(repoDir, "dirty.txt")); err != nil || string(data) != "keep me" {
		t.Fatalf("source dirty file changed: %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(worktreePath, "f.txt")); err != nil || string(data) != "x" {
		t.Fatalf("managed worktree did not use main base: %q, %v", data, err)
	}
	if got := Describe(worktreePath).Branch; got != "nagare/ticket-attempt" {
		t.Errorf("managed branch = %q", got)
	}
}

func TestReviewAndPushManagedBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	repoDir := filepath.Join(root, "app")
	initRepo(t, repoDir)
	base, err := ResolveBaseCommit(repoDir, "dev")
	if err != nil {
		t.Fatal(err)
	}
	worktreePath := filepath.Join(root, "managed", "app")
	branch := "nagare/review-test"
	if err := AddManagedWorktree(repoDir, worktreePath, branch, base); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "f.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, worktreePath, "commit", "-qam", "change")

	review, err := ReviewWorktree(worktreePath, branch, base)
	if err != nil {
		t.Fatal(err)
	}
	if review.DirtyFiles != 0 || review.Commits != 1 || !strings.Contains(review.Stat, "f.txt") || !strings.Contains(review.Diff, "+changed") {
		t.Fatalf("review = %#v", review)
	}

	remote := filepath.Join(root, "remote.git")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, remote, "init", "-q", "--bare")
	gitRun(t, repoDir, "remote", "add", "origin", remote)
	if err := PushBranch(worktreePath, branch); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "--git-dir", remote, "show-ref", "--verify", "refs/heads/"+branch).Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Fatalf("remote branch missing: %q, %v", out, err)
	}
	if err := PushBranch(worktreePath, "nagare/other"); err == nil {
		t.Fatal("PushBranch accepted a branch other than the checked-out recorded branch")
	}

	if err := os.WriteFile(filepath.Join(worktreePath, "dirty.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PushBranch(worktreePath, branch); err == nil {
		t.Fatal("PushBranch accepted dirty worktree")
	}
}

func TestPullRequestBaseNormalizesOnlyOriginAndLocalRefs(t *testing.T) {
	for input, want := range map[string]string{
		"main":                     "main",
		"release/v2":               "release/v2",
		"origin/main":              "main",
		"refs/heads/main":          "main",
		"refs/remotes/origin/main": "main",
	} {
		got, err := PullRequestBase(input)
		if err != nil || got != want {
			t.Errorf("PullRequestBase(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := PullRequestBase("refs/remotes/upstream/main"); err == nil {
		t.Fatal("non-origin remote ref accepted as GitHub base")
	}
}
