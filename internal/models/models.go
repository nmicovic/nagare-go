package models

// SessionStatus represents the current state of an agent session.
type SessionStatus string

const (
	// StatusWaitingInput indicates the agent is waiting for user input.
	StatusWaitingInput SessionStatus = "waiting_input"
	// StatusRunning indicates the agent is actively working.
	StatusRunning SessionStatus = "running"
	// StatusIdle indicates the agent is idle.
	StatusIdle SessionStatus = "idle"
	// StatusDead indicates the agent process has exited.
	StatusDead SessionStatus = "dead"
)

// AgentType represents which AI coding agent is running.
type AgentType string

const (
	// AgentClaude represents the Claude Code agent.
	AgentClaude AgentType = "claude"
	// AgentOpenCode represents the OpenCode agent.
	AgentOpenCode AgentType = "opencode"
	// AgentGemini represents the Gemini agent.
	AgentGemini AgentType = "gemini"
	// AgentUnknown represents an unrecognized agent.
	AgentUnknown AgentType = "unknown"
)

// SessionDetails holds metadata about a session.
type SessionDetails struct {
	GitBranch    string
	Model        string
	ContextUsage string
	LastActivity string // ISO 8601 timestamp of last hook event
	LastEvent    string // last hook event name (e.g. "Stop", "UserPromptSubmit")
}

// Session represents a discovered agent pane in tmux.
type Session struct {
	Name        string
	SessionID   string
	Path        string
	WindowIndex int
	PaneIndex   int
	Status      SessionStatus
	AgentType   AgentType
	Details     SessionDetails
	LastMessage string
}

// SessionState is the JSON-serializable hook state written to disk.
type SessionState struct {
	State            string `json:"state"`
	SessionID        string `json:"session_id"`
	Cwd              string `json:"cwd"`
	Event            string `json:"event"`
	NotificationType string `json:"notification_type,omitempty"`
	LastMessage      string `json:"last_message,omitempty"`
	Timestamp        string `json:"timestamp"`
}

// StatusColor returns the hex color for a status (tokyonight palette).
func StatusColor(s SessionStatus) string {
	switch s {
	case StatusWaitingInput:
		return "#db4b4b"
	case StatusRunning:
		return "#e0af68"
	case StatusIdle:
		return "#00D26A"
	case StatusDead:
		return "#565f89"
	default:
		return "#565f89"
	}
}

// StatusLabel returns the human-readable label for a status.
func StatusLabel(s SessionStatus) string {
	switch s {
	case StatusWaitingInput:
		return "Waiting for input"
	case StatusRunning:
		return "Working"
	case StatusIdle:
		return "Idle"
	case StatusDead:
		return "Exited"
	default:
		return "Unknown"
	}
}

// AgentLabel returns the human-readable label for an agent type.
func AgentLabel(a AgentType) string {
	switch a {
	case AgentClaude:
		return "Claude"
	case AgentOpenCode:
		return "OpenCode"
	case AgentGemini:
		return "Gemini"
	default:
		return "Unknown"
	}
}

// AgentColor returns the foreground hex color for an agent type.
func AgentColor(a AgentType) string {
	switch a {
	case AgentClaude:
		return "#da7756"
	case AgentOpenCode:
		return "#00e5ff"
	case AgentGemini:
		return "#4285f4"
	default:
		return "#565f89"
	}
}

// AgentBgColor returns the background hex color for an agent type.
func AgentBgColor(a AgentType) string {
	switch a {
	case AgentClaude:
		return "#3b2820"
	case AgentOpenCode:
		return "#002b33"
	case AgentGemini:
		return "#1a2744"
	default:
		return ""
	}
}
