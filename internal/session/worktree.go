package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nemke/nagare-go/internal/git"
	"github.com/nemke/nagare-go/internal/log"
	"github.com/nemke/nagare-go/internal/state"
	"github.com/nemke/nagare-go/internal/tmux"
)

// worktreeLaunch describes how to start an agent in a new worktree.
type worktreeLaunch struct {
	Cmd       string // command sent to the pane
	Cwd       string // directory the window opens in
	Path      string // where the worktree will live
	PreCreate bool   // nagare must create the worktree before opening the window
}

// planWorktreeLaunch decides how a given agent gets into a new worktree.
//
// Claude Code has "-w <name>", so it is handed the flag and creates the
// worktree itself under .claude/worktrees. No other agent has an equivalent, so
// nagare creates the worktree under .worktrees and opens the window inside it.
//
// --tmux is never passed: nagare has already made the window, and letting
// Claude create a second tmux session would fight it.
func planWorktreeLaunch(agent, mainRoot, name string) worktreeLaunch {
	if agent == "claude" {
		return worktreeLaunch{
			Cmd:  "claude -w " + name,
			Cwd:  mainRoot,
			Path: git.ClaudeWorktreePath(mainRoot, name),
		}
	}
	path := filepath.Join(mainRoot, ".worktrees", name)
	return worktreeLaunch{
		// A brand new worktree has no prior session, so never continue.
		Cmd:       agentCommand(agent, path, false),
		Cwd:       path,
		Path:      path,
		PreCreate: true,
	}
}

// sessionForRepo returns the tmux session belonging to the repository whose main
// checkout is mainRoot, or "" when none exists.
//
// Sessions are matched by repository rather than by literal path, because a
// session's own directory is often a worktree — the first worktree session for a
// repo starts life with the worktree as its path — and every worktree of one
// repo must still land in a single session for the picker to group them.
func sessionForRepo(mainRoot string) string {
	raw := tmux.RunTmux("list-sessions", "-F", "#{session_name}:#{session_path}")
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) != 2 {
			continue
		}
		if parts[1] == mainRoot || git.MainRoot(parts[1]) == mainRoot {
			return parts[0]
		}
	}
	return ""
}

// CreateWorktree starts an agent in a new, named git worktree of repoPath.
//
// The pane joins the repo's existing tmux session as a new window rather than
// getting a session of its own, which is what lets the picker group a repo's
// worktrees under one header. If the repo has no session yet, one is created.
func CreateWorktree(repoPath, worktreeName, agent string) (string, error) {
	if err := git.ValidWorktreeName(worktreeName); err != nil {
		return "", err
	}

	repoPath = ExpandPath(ResolvePath(repoPath))
	if _, err := os.Stat(repoPath); err != nil {
		return "", fmt.Errorf("path does not exist: %s", repoPath)
	}

	// Resolve to the main checkout: the selected pane may itself be a worktree,
	// and a worktree of a worktree is not what anyone means.
	mainRoot := git.MainRoot(repoPath)
	if mainRoot == "" {
		return "", fmt.Errorf("%s is not a git repository — worktrees need one", repoPath)
	}

	plan := planWorktreeLaunch(agent, mainRoot, worktreeName)
	if plan.PreCreate {
		if _, err := git.AddWorktree(mainRoot, worktreeName); err != nil {
			return "", err
		}
	} else if _, err := os.Stat(plan.Path); err == nil {
		return "", fmt.Errorf("worktree %q already exists at %s", worktreeName, plan.Path)
	}

	// Join the repo's session so the picker groups this pane with its siblings.
	sessName := sessionForRepo(mainRoot)
	if sessName == "" {
		sessName = UniqueName(filepath.Base(mainRoot))
		tmux.RunTmux("new-session", "-d", "-s", sessName, "-c", plan.Cwd)
		tmux.RunTmux("rename-window", "-t", sessName+":0", worktreeName)
	} else {
		tmux.RunTmux("new-window", "-t", sessName, "-n", worktreeName, "-c", plan.Cwd)
	}
	// Target the window by name rather than the session: sending to the session
	// hits whichever window is current, which is a race if the user switches
	// windows between the two calls.
	tmux.RunTmux("send-keys", "-t", sessName+":"+worktreeName, plan.Cmd, "Enter")

	// Register under the grouped display name, which is what the picker and the
	// star registry key off.
	reg := state.NewRegistry(state.DefaultRegistryPath())
	reg.Register(sessName+"/"+worktreeName, plan.Path, agent)

	log.Info("created worktree session %s/%s (%s) at %s", sessName, worktreeName, agent, plan.Path)
	return sessName, nil
}
