package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/nemke/nagare-go/internal/models"
)

// RawSession is a parsed tmux session from list-sessions.
type RawSession struct {
	Name      string
	SessionID string
	Path      string
}

// PaneInfo is a parsed agent pane from list-panes.
type PaneInfo struct {
	WindowIndex int
	PaneIndex   int
	AgentType   models.AgentType
	WindowName  string
}

var agentProcesses = map[string]models.AgentType{
	"claude":   models.AgentClaude,
	"opencode": models.AgentOpenCode,
}

// ParseSessions parses tmux list-sessions output.
// Format: "#{session_name}:#{session_id}:#{session_path}"
func ParseSessions(raw string) []RawSession {
	var sessions []RawSession
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		sessions = append(sessions, RawSession{
			Name:      parts[0],
			SessionID: parts[1],
			Path:      parts[2],
		})
	}
	return sessions
}

// ParseAllPanes parses tmux list-panes -a output.
// Format: "#{session_name}:#{window_index}:#{pane_index}:#{pane_current_command}:#{pane_pid}:#{window_name}"
// Returns agent panes grouped by session name.
func ParseAllPanes(raw string) map[string][]PaneInfo {
	result := make(map[string][]PaneInfo)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 6)
		if len(parts) < 5 {
			continue
		}
		sessionName := parts[0]
		windowIdx, _ := strconv.Atoi(parts[1])
		paneIdx, _ := strconv.Atoi(parts[2])
		cmd := strings.TrimSpace(parts[3])
		pid := parts[4]
		windowName := ""
		if len(parts) >= 6 {
			windowName = strings.TrimSpace(parts[5])
		}

		agentType, ok := agentProcesses[cmd]
		if !ok {
			agentType, ok = resolveAgentFromDescendants(pid)
			if !ok {
				continue
			}
		}

		result[sessionName] = append(result[sessionName], PaneInfo{
			WindowIndex: windowIdx,
			PaneIndex:   paneIdx,
			AgentType:   agentType,
			WindowName:  windowName,
		})
	}
	return result
}

// descendantAgents maps process names found in /proc cmdline to agent types.
var descendantAgents = map[string]models.AgentType{
	"gemini": models.AgentGemini,
	"crush":  models.AgentCrush,
}

// resolveAgentFromDescendants walks the process tree via /proc to find
// known agents running as descendants (e.g. zsh → node → crush).
// Linux-only: silently returns false on macOS/other platforms.
func resolveAgentFromDescendants(pid string) (models.AgentType, bool) {
	// BFS through child processes, max 3 levels deep.
	queue := []string{pid}
	for depth := 0; depth < 3 && len(queue) > 0; depth++ {
		var next []string
		for _, p := range queue {
			childrenPath := fmt.Sprintf("/proc/%s/task/%s/children", p, p)
			data, err := os.ReadFile(childrenPath)
			if err != nil {
				continue
			}
			for _, childPid := range strings.Fields(string(data)) {
				next = append(next, childPid)
				cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%s/cmdline", childPid))
				if err != nil {
					continue
				}
				for _, arg := range strings.Split(string(cmdline), "\x00") {
					base := arg
					if idx := strings.LastIndex(arg, "/"); idx >= 0 {
						base = arg[idx+1:]
					}
					if agentType, ok := descendantAgents[base]; ok {
						return agentType, true
					}
				}
			}
		}
		queue = next
	}
	return "", false
}

// gitBranch returns the current git branch for a directory, or "".
func gitBranch(dir string) string {
	cmd := exec.Command("git", "-C", dir, "branch", "--show-current")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// hookStateMap maps hook state strings to SessionStatus.
var hookStateMap = map[string]models.SessionStatus{
	"working":       models.StatusRunning,
	"waiting_input": models.StatusWaitingInput,
	"idle":          models.StatusIdle,
	"dead":          models.StatusDead,
}

// ScanSessions discovers all agent sessions in tmux.
// hookStates should be pre-loaded via state.LoadAllStates().
func ScanSessions(hookStates map[string]models.SessionState) []models.Session {
	rawSessions := RunTmux("list-sessions", "-F", "#{session_name}:#{session_id}:#{session_path}")
	sessions := ParseSessions(rawSessions)

	rawPanes := RunTmux("list-panes", "-a", "-F", "#{session_name}:#{window_index}:#{pane_index}:#{pane_current_command}:#{pane_pid}:#{window_name}")
	allPanes := ParseAllPanes(rawPanes)

	var result []models.Session
	for _, sess := range sessions {
		panes, ok := allPanes[sess.Name]
		if !ok {
			continue
		}
		for _, pane := range panes {
			hookState, hasHook := hookStates[sess.Path]

			var status models.SessionStatus
			var lastMessage string
			var details models.SessionDetails

			if hasHook {
				s, ok := hookStateMap[hookState.State]
				if ok {
					status = s
				} else {
					status = models.StatusIdle
				}
				lastMessage = hookState.LastMessage
				details.LastActivity = hookState.Timestamp
				details.LastEvent = hookState.Event
			} else {
				status = models.StatusIdle
			}

			// Get git branch from working directory
			details.GitBranch = gitBranch(sess.Path)

			// Use window name as display name when multiple agents share a session
			displayName := sess.Name
			if len(panes) > 1 && pane.WindowName != "" && pane.WindowName != sess.Name {
				displayName = pane.WindowName
			}

			result = append(result, models.Session{
				Name:        displayName,
				SessionID:   sess.SessionID,
				SessionName: sess.Name,
				Path:        sess.Path,
				WindowIndex: pane.WindowIndex,
				PaneIndex:   pane.PaneIndex,
				Status:      status,
				AgentType:   pane.AgentType,
				Details:     details,
				LastMessage: lastMessage,
			})
		}
	}
	return result
}
