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
	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/state"
	"github.com/nemke/nagare-go/internal/tmux"
)

// AgentSpec identifies the agent executable and optional per-session model.
type AgentSpec struct {
	Agent string
	Model string
}

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

// SendPromptToPane submits literal text to an agent TUI once that agent is
// listening, and waits for it to report that it accepted the prompt. Anything
// typed earlier goes to the agent's startup — for a new worktree, to Claude
// Code's trust dialog, where the Enter after it answers "No, exit".
func SendPromptToPane(paneID, prompt, agent string, startedAt time.Time) error {
	if strings.TrimSpace(paneID) == "" {
		return fmt.Errorf("pane ID is empty")
	}
	return tmux.SubmitPrompt(tmux.Submit{
		Target:    paneID,
		Text:      prompt,
		Reports:   models.ReportsStatus(models.AgentType(agent)),
		NotBefore: startedAt,
	})
}

// LaunchManagedWorktree creates an explicitly based worktree and starts an agent
// in a dedicated tmux window. Once Git creation succeeds, later failures leave
// the branch and worktree intact for recovery.
func LaunchManagedWorktree(repoPath, worktreePath, windowName, branch, baseCommit string, spec AgentSpec) (ManagedWorktree, error) {
	spec.Agent = strings.TrimSpace(spec.Agent)
	spec.Model = strings.TrimSpace(spec.Model)
	if err := ValidateAgent(spec.Agent); err != nil {
		return ManagedWorktree{}, err
	}
	if err := ValidateModelSelection(spec.Agent, spec.Model); err != nil {
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
	if _, err := tmux.RunStrict("send-keys", "-t", paneID, agentCommand(spec.Agent, spec.Model, worktreePath, false), "Enter"); err != nil {
		return ManagedWorktree{}, err
	}

	displayName := sessName + "/" + windowName
	state.NewRegistry(state.DefaultRegistryPath()).Register(displayName, worktreePath, spec.Agent)
	log.Info("created managed attempt %s (%s/%s) on %s at %s", displayName, spec.Agent, spec.Model, branch, worktreePath)
	return ManagedWorktree{
		ProjectPath:  mainRoot,
		WorktreePath: worktreePath,
		Branch:       branch,
		SessionName:  sessName,
		DisplayName:  displayName,
		PaneID:       paneID,
	}, nil
}
