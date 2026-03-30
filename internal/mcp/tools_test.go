package mcp

import (
	"testing"
)

// TestSendMessage tests sending a message to react session
func TestSendMessage(t *testing.T) {
	// Test send_message to react session
	input := SendMessageInput{
		Target:  "react",
		Message: "hi",
	}

	result := SendMessageHandler("nagare-go", input)
	t.Logf("Result: %s", result)

	// The result should indicate success or error
	if result == "" {
		t.Error("Expected a result from SendMessageHandler")
	}
}

// TestListAgents tests listing all agents
func TestListAgents(t *testing.T) {
	result := ListAgentsHandler("nagare-go")
	t.Logf("Agents: %s", result)

	if result == "" {
		t.Error("Expected agent list")
	}
}
