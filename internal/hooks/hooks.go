package hooks

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nemke/nagare-go/internal/config"
	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/notifications"
	"github.com/nemke/nagare-go/internal/state"
)

// HookEvent is the JSON structure received from Claude Code hooks via stdin.
type HookEvent struct {
	HookEventName        string `json:"hook_event_name"`
	SessionID            string `json:"session_id"`
	Cwd                  string `json:"cwd"`
	LastAssistantMessage string `json:"last_assistant_message"`
	NotificationType     string `json:"notification_type"`
}

var needsInputTypes = map[string]bool{
	"permission_prompt":  true,
	"elicitation_dialog": true,
}

// EventToState maps a hook event name to a state string.
func EventToState(event, notificationType string) string {
	switch event {
	case "UserPromptSubmit", "PreToolUse", "BeforeAgent", "BeforeTool", "AfterTool":
		return "working"
	case "Stop", "AfterAgent":
		return "idle"
	case "Notification":
		if needsInputTypes[notificationType] {
			return "waiting_input"
		}
		return "idle"
	case "SessionEnd":
		return "dead"
	case "SessionStart":
		return "idle"
	default:
		return "unknown"
	}
}

// ShouldNotify determines if a notification should fire.
// Returns (eventType, workingSeconds). eventType is "" if no notification.
// minWorkingSeconds is the threshold from config (typically 30).
func ShouldNotify(newState, prevState string, workingSeconds, minWorkingSeconds int) (string, int) {
	if newState == "waiting_input" {
		return "needs_input", 0
	}

	if newState == "idle" && prevState == "working" && workingSeconds >= minWorkingSeconds {
		return "task_complete", workingSeconds
	}

	return "", 0
}

// Handle reads a hook event from stdin and processes it.
func Handle() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return
	}

	var event HookEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}

	newState := EventToState(event.HookEventName, event.NotificationType)
	now := time.Now().UTC().Format(time.RFC3339)
	statesDir := state.DefaultStatesDir()

	// Load previous state
	prevStates := state.LoadAllStates(statesDir)
	prevState, hasPrev := prevStates[event.Cwd]

	// Write new state
	newSessionState := models.SessionState{
		State:            newState,
		SessionID:        event.SessionID,
		Cwd:              event.Cwd,
		Event:            event.HookEventName,
		NotificationType: event.NotificationType,
		LastMessage:      event.LastAssistantMessage,
		Timestamp:        now,
	}
	state.WriteState(statesDir, newSessionState)

	// Determine working duration
	var workingSeconds int
	if hasPrev && prevState.State == "working" {
		prevTime, err := time.Parse(time.RFC3339, prevState.Timestamp)
		if err == nil {
			workingSeconds = int(time.Since(prevTime).Seconds())
		}
	}

	// Load config
	cfg, _ := config.Load()
	if !cfg.Notifications.Enabled {
		return
	}

	// Check if notification needed
	prevStateStr := ""
	if hasPrev {
		prevStateStr = prevState.State
	}

	minSecs := cfg.Notifications.TaskComplete.MinWorkingSeconds
	eventType, _ := ShouldNotify(newState, prevStateStr, workingSeconds, minSecs)
	if eventType == "" {
		return
	}

	var eventCfg config.NotificationEventConfig
	switch eventType {
	case "needs_input":
		eventCfg = cfg.Notifications.NeedsInput
	case "task_complete":
		eventCfg = cfg.Notifications.TaskComplete
	}

	// Resolve session name from tmux
	sessionName := resolveSessionName(event.Cwd)

	notifications.Deliver(
		sessionName,
		eventType,
		event.NotificationType,
		eventCfg.Toast,
		eventCfg.Bell,
		eventCfg.OsNotify,
		cfg.NotificationDuration,
	)

	// Store notification
	store := notifications.NewStore(notifications.DefaultStorePath())
	message := notifications.BuildToastMessage(sessionName, eventType, event.NotificationType)
	store.Add(sessionName, message)
}

// resolveSessionName finds the tmux session name for a working directory.
func resolveSessionName(cwd string) string {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}:#{session_path}").Output()
	if err != nil {
		if idx := strings.LastIndex(cwd, "/"); idx >= 0 {
			return cwd[idx+1:]
		}
		return cwd
	}
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && parts[1] == cwd {
			return parts[0]
		}
	}
	if idx := strings.LastIndex(cwd, "/"); idx >= 0 {
		return cwd[idx+1:]
	}
	return cwd
}
