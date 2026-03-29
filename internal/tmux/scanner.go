package tmux

import (
	"fmt"
	"os"
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
// Format: "#{session_name}:#{window_index}:#{pane_index}:#{pane_current_command}:#{pane_pid}"
// Returns agent panes grouped by session name.
func ParseAllPanes(raw string) map[string][]PaneInfo {
	result := make(map[string][]PaneInfo)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 5)
		if len(parts) != 5 {
			continue
		}
		sessionName := parts[0]
		windowIdx, _ := strconv.Atoi(parts[1])
		paneIdx, _ := strconv.Atoi(parts[2])
		cmd := strings.TrimSpace(parts[3])
		pid := parts[4]

		agentType, ok := agentProcesses[cmd]
		if !ok && cmd == "node" {
			agentType, ok = resolveNodeAgent(pid)
			if !ok {
				continue
			}
		} else if !ok {
			continue
		}

		result[sessionName] = append(result[sessionName], PaneInfo{
			WindowIndex: windowIdx,
			PaneIndex:   paneIdx,
			AgentType:   agentType,
		})
	}
	return result
}

// resolveNodeAgent checks /proc to identify Gemini running under node.
// Linux-only: silently returns false on macOS/other platforms.
func resolveNodeAgent(pid string) (models.AgentType, bool) {
	childrenPath := fmt.Sprintf("/proc/%s/task/%s/children", pid, pid)
	data, err := os.ReadFile(childrenPath)
	if err != nil {
		return "", false
	}
	for _, childPid := range strings.Fields(string(data)) {
		cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%s/cmdline", childPid))
		if err != nil {
			continue
		}
		args := strings.Split(string(cmdline), "\x00")
		for _, arg := range args {
			basename := arg
			if idx := strings.LastIndex(arg, "/"); idx >= 0 {
				basename = arg[idx+1:]
			}
			if basename == "gemini" {
				return models.AgentGemini, true
			}
		}
	}
	return "", false
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

	rawPanes := RunTmux("list-panes", "-a", "-F", "#{session_name}:#{window_index}:#{pane_index}:#{pane_current_command}:#{pane_pid}")
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
			} else {
				// Without hook state, default to Idle. The picker captures
				// pane content separately for the selected session's preview.
				status = models.StatusIdle
			}

			result = append(result, models.Session{
				Name:        sess.Name,
				SessionID:   sess.SessionID,
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
