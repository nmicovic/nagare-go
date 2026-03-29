package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nemke/nagare-go/internal/config"
	"github.com/nemke/nagare-go/internal/log"
	"github.com/nemke/nagare-go/internal/state"
	"github.com/nemke/nagare-go/internal/tmux"
)

// ResolvePath resolves a path. If it contains no / and no ~, prepend QuickProjectPath.
func ResolvePath(path string) string {
	if !strings.Contains(path, "/") && !strings.Contains(path, "~") {
		cfg, _ := config.Load()
		return filepath.Join(cfg.Picker.QuickProjectPath, path)
	}
	return path
}

// ExpandPath expands ~ to home directory and returns absolute path.
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

// UniqueName ensures a session name is unique by appending -N if needed.
func UniqueName(name string) string {
	existing := make(map[string]bool)
	raw := tmux.RunTmux("list-sessions", "-F", "#{session_name}")
	for _, line := range strings.Split(raw, "\n") {
		existing[strings.TrimSpace(line)] = true
	}
	if !existing[name] {
		return name
	}
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if !existing[candidate] {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d", name, os.Getpid())
}

// Create creates a new tmux session with the specified agent.
func Create(path, name, agent string, continueSession bool) (string, error) {
	path = ExpandPath(ResolvePath(path))

	if err := os.MkdirAll(path, 0755); err != nil {
		return "", fmt.Errorf("cannot create directory: %w", err)
	}

	if name == "" {
		name = filepath.Base(path)
	}
	name = UniqueName(name)

	// Create tmux session
	tmux.RunTmux("new-session", "-d", "-s", name, "-c", path)

	// Launch agent
	cmd := agentCommand(agent, continueSession)
	tmux.RunTmux("send-keys", "-t", name, cmd, "Enter")

	// Register
	reg := state.NewRegistry(state.DefaultRegistryPath())
	reg.Register(name, path, agent)

	log.Info("created session %s (%s) at %s", name, agent, path)
	return name, nil
}

// agentCommand returns the command to launch an agent.
func agentCommand(agent string, continueSession bool) string {
	switch agent {
	case "opencode":
		if continueSession {
			return "opencode -c"
		}
		return "opencode"
	case "gemini":
		return "gemini"
	default: // claude
		if continueSession {
			return "claude -c"
		}
		return "claude"
	}
}

// ListDirectories returns directory suggestions for path autocomplete.
func ListDirectories(partial string, maxResults int) []string {
	partial = ExpandPath(partial)

	dir := partial
	prefix := ""
	if !strings.HasSuffix(partial, "/") {
		dir = filepath.Dir(partial)
		prefix = strings.ToLower(filepath.Base(partial))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var results []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(e.Name()), prefix) {
			continue
		}
		results = append(results, filepath.Join(dir, e.Name()))
		if len(results) >= maxResults {
			break
		}
	}
	return results
}

// SwitchToSession switches to or attaches to a tmux session.
func SwitchToSession(name string) {
	// Inside tmux: switch client
	if os.Getenv("TMUX") != "" {
		tmux.RunTmux("switch-client", "-t", name)
	} else {
		// Outside tmux: attach
		tmux.RunTmux("attach-session", "-t", name)
	}
}
