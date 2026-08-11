package session

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanWorktreeLaunchClaude(t *testing.T) {
	root := "/home/u/Projects/app"
	got := planWorktreeLaunch("claude", root, "the-site")

	// Claude creates the worktree itself, so the window opens at the repo root
	// and the command carries the flag.
	if got.Cmd != "claude -w the-site" {
		t.Errorf("Cmd = %q, want %q", got.Cmd, "claude -w the-site")
	}
	if got.Cwd != root {
		t.Errorf("Cwd = %q, want the repo root %q", got.Cwd, root)
	}
	want := filepath.Join(root, ".claude", "worktrees", "the-site")
	if got.Path != want {
		t.Errorf("Path = %q, want %q", got.Path, want)
	}
	if got.PreCreate {
		t.Error("PreCreate = true, but Claude creates its own worktree")
	}
}

func TestPlanWorktreeLaunchOtherAgents(t *testing.T) {
	root := "/home/u/Projects/app"
	for _, agent := range []string{"opencode", "gemini", "crush", "pi"} {
		got := planWorktreeLaunch(agent, root, "the-site")

		want := filepath.Join(root, ".worktrees", "the-site")
		if got.Path != want {
			t.Errorf("%s: Path = %q, want %q", agent, got.Path, want)
		}
		// No -w flag exists for these, so nagare makes the worktree and the
		// window opens inside it.
		if !got.PreCreate {
			t.Errorf("%s: PreCreate = false, want true", agent)
		}
		if got.Cwd != want {
			t.Errorf("%s: Cwd = %q, want the worktree %q", agent, got.Cwd, want)
		}
		if strings.Contains(got.Cmd, "-w") {
			t.Errorf("%s: Cmd = %q, must not pass -w", agent, got.Cmd)
		}
		if !strings.HasPrefix(got.Cmd, agent) {
			t.Errorf("%s: Cmd = %q, want it to launch %s", agent, got.Cmd, agent)
		}
		// A fresh worktree has no prior session to continue.
		if strings.Contains(got.Cmd, "-c") {
			t.Errorf("%s: Cmd = %q, must not continue a session in a new worktree", agent, got.Cmd)
		}
	}
}

// --tmux would make Claude create a second tmux session, fighting the one
// nagare just made the window in.
func TestPlanWorktreeLaunchNeverPassesTmux(t *testing.T) {
	for _, agent := range []string{"claude", "opencode", "pi"} {
		if got := planWorktreeLaunch(agent, "/r", "w"); strings.Contains(got.Cmd, "--tmux") {
			t.Errorf("%s: Cmd = %q, must not pass --tmux", agent, got.Cmd)
		}
	}
}

// Every agent must launch itself. "crush" previously fell through to the
// default branch and silently started claude.
func TestAgentCommandLaunchesTheRequestedAgent(t *testing.T) {
	for _, agent := range []string{"claude", "opencode", "gemini", "crush", "pi"} {
		if got := agentCommand(agent, "/tmp", false); !strings.HasPrefix(got, agent) {
			t.Errorf("agentCommand(%q) = %q, want it to launch %s", agent, got, agent)
		}
	}
}
