package session

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nemke/nagare-go/internal/git"
	"github.com/nemke/nagare-go/internal/log"
	"github.com/nemke/nagare-go/internal/state"
	"github.com/nemke/nagare-go/internal/tmux"
)

// ManagedWorktree identifies the resources created for an orchestrated attempt.
type ManagedWorktree struct {
	ProjectPath  string
	WorktreePath string
	Branch       string
	SessionName  string
	DisplayName  string
	PaneID       string
}

// ValidateAgent reports whether agent is supported and its executable is available.
func ValidateAgent(agent string) error {
	switch agent {
	case "claude", "codex", "opencode", "gemini", "crush", "pi", "omp":
	default:
		return fmt.Errorf("unsupported agent %q", agent)
	}
	if _, err := exec.LookPath(agent); err != nil {
		return fmt.Errorf("%s executable not found in PATH", agent)
	}
	return nil
}

// SendPromptToPane submits literal text to an already started agent TUI.
func SendPromptToPane(paneID, prompt string) error {
	if strings.TrimSpace(paneID) == "" {
		return fmt.Errorf("pane ID is empty")
	}
	if _, err := tmux.RunStrict("send-keys", "-t", paneID, "-l", prompt); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	_, err := tmux.RunStrict("send-keys", "-t", paneID, "Enter")
	return err
}

// LaunchManagedWorktree creates an explicitly based worktree and starts an agent
// in a dedicated tmux window. Once Git creation succeeds, later failures leave
// the branch and worktree intact for recovery.
func LaunchManagedWorktree(repoPath, worktreePath, windowName, branch, baseCommit, agent string) (ManagedWorktree, error) {
	if err := ValidateAgent(agent); err != nil {
		return ManagedWorktree{}, err
	}
	mainRoot := git.MainRoot(ExpandPath(repoPath))
	if mainRoot == "" {
		return ManagedWorktree{}, fmt.Errorf("%s is not a git repository", repoPath)
	}
	if strings.TrimSpace(windowName) == "" || strings.ContainsAny(windowName, ":./\\") {
		return ManagedWorktree{}, fmt.Errorf("invalid tmux window name %q", windowName)
	}
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o700); err != nil {
		return ManagedWorktree{}, fmt.Errorf("create managed workspace directory: %w", err)
	}
	if err := git.AddManagedWorktree(mainRoot, worktreePath, branch, baseCommit); err != nil {
		return ManagedWorktree{}, err
	}

	sessName := sessionForRepo(mainRoot)
	var paneID string
	var err error
	if sessName == "" {
		sessName = UniqueName(filepath.Base(mainRoot))
		paneID, err = tmux.RunStrict("new-session", "-d", "-P", "-F", "#{pane_id}", "-s", sessName, "-n", windowName, "-c", worktreePath)
	} else {
		paneID, err = tmux.RunStrict("new-window", "-d", "-P", "-F", "#{pane_id}", "-t", sessName, "-n", windowName, "-c", worktreePath)
	}
	if err != nil {
		return ManagedWorktree{}, err
	}
	paneID = strings.TrimSpace(paneID)
	if paneID == "" {
		return ManagedWorktree{}, fmt.Errorf("tmux did not report the new pane ID")
	}
	if _, err := tmux.RunStrict("send-keys", "-t", paneID, agentCommand(agent, worktreePath, false), "Enter"); err != nil {
		return ManagedWorktree{}, err
	}

	displayName := sessName + "/" + windowName
	state.NewRegistry(state.DefaultRegistryPath()).Register(displayName, worktreePath, agent)
	log.Info("created managed attempt %s (%s) on %s at %s", displayName, agent, branch, worktreePath)
	return ManagedWorktree{
		ProjectPath:  mainRoot,
		WorktreePath: worktreePath,
		Branch:       branch,
		SessionName:  sessName,
		DisplayName:  displayName,
		PaneID:       paneID,
	}, nil
}
