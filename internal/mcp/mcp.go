package mcp

import "time"

// AgentInfo describes a discovered agent for MCP listing.
type AgentInfo struct {
	SessionName string
	AgentType   string
	Status      string
	Path        string
}

// Message is an inter-agent message.
type Message struct {
	ID        string
	From      string
	To        string
	Content   string
	Timestamp string
}

// Server defines the MCP server interface. Implementation deferred to v2.
type Server interface {
	ListAgents() ([]AgentInfo, error)
	SendMessage(target string, message string) error
	SendMessageAndWait(target string, message string, timeout time.Duration) (string, error)
	CheckMessages() ([]Message, error)
	Reply(messageID string, response string) error
}
