// Package git resolves a directory into the git repository facts nagare
// displays: the current branch, the repository name, and — when the directory
// is a linked worktree — the worktree name.
package git

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo describes the git checkout a directory belongs to. The zero value is
// what callers get for a directory that is not a repository, and it renders as
// "no git information" rather than as an error.
type Repo struct {
	Branch     string // empty when detached or not a repository
	RepoName   string // basename of the main checkout, identical across its worktrees
	Worktree   string // basename of the worktree directory, empty for the main checkout
	IsWorktree bool
}

// parseRevParse turns the three lines of `git rev-parse --git-common-dir
// --show-toplevel --abbrev-ref HEAD` into a Repo. dir is the directory the
// command ran in, needed because git prints a bare ".git" for a main checkout.
func parseRevParse(out, dir string) Repo {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 {
		return Repo{}
	}

	commonDir, toplevel, branch := lines[0], lines[1], lines[2]
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(dir, commonDir)
	}

	// A main checkout's common dir sits directly inside its own toplevel; a
	// linked worktree's points back at the main checkout instead.
	mainRoot := filepath.Dir(filepath.Clean(commonDir))
	repo := Repo{
		RepoName:   filepath.Base(mainRoot),
		IsWorktree: mainRoot != filepath.Clean(toplevel),
	}
	if repo.IsWorktree {
		repo.Worktree = filepath.Base(toplevel)
	}
	// Detached HEAD prints the literal "HEAD"; report no branch, matching what
	// `git branch --show-current` returns.
	if branch != "HEAD" {
		repo.Branch = branch
	}
	return repo
}

// Describe resolves dir into a Repo using a single git invocation. Any failure
// (not a repository, git missing) returns the zero Repo.
func Describe(dir string) Repo {
	cmd := exec.Command("git", "-C", dir, "rev-parse",
		"--git-common-dir", "--show-toplevel", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return Repo{}
	}
	return parseRevParse(string(out), dir)
}
