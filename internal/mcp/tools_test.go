package mcp

import (
	"strings"
	"testing"

	"github.com/nemke/nagare-go/internal/models"
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

func TestResolveSessionExact(t *testing.T) {
	sessions := []models.Session{
		{Name: "cosmo-ai"},
		{Name: "cosmo-ai/claude_01"},
	}
	got, err := resolveSession("cosmo-ai", sessions)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "cosmo-ai" {
		t.Errorf("got %q", got.Name)
	}
}

func TestResolveSessionPrefix(t *testing.T) {
	sessions := []models.Session{{Name: "cosmo-ai/claude_01"}}
	got, err := resolveSession("cosmo-ai", sessions)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "cosmo-ai/claude_01" {
		t.Errorf("got %q", got.Name)
	}
}

func TestResolveSessionAmbiguous(t *testing.T) {
	sessions := []models.Session{
		{Name: "cosmo-ai/claude_01"},
		{Name: "cosmo-ai/claude_02"},
	}
	_, err := resolveSession("cosmo-ai", sessions)
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "cosmo-ai/claude_01") || !strings.Contains(err.Error(), "cosmo-ai/claude_02") {
		t.Errorf("error should list candidates: %v", err)
	}
}

func TestResolveSessionNotFound(t *testing.T) {
	_, err := resolveSession("nope", []models.Session{{Name: "other"}})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got %v", err)
	}
}
