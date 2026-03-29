package models

import "testing"

func TestSessionStatusString(t *testing.T) {
	tests := []struct {
		status SessionStatus
		want   string
	}{
		{StatusWaitingInput, "waiting_input"},
		{StatusRunning, "running"},
		{StatusIdle, "idle"},
		{StatusDead, "dead"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("SessionStatus = %q, want %q", tt.status, tt.want)
		}
	}
}

func TestAgentTypeString(t *testing.T) {
	tests := []struct {
		agent AgentType
		want  string
	}{
		{AgentClaude, "claude"},
		{AgentOpenCode, "opencode"},
		{AgentGemini, "gemini"},
		{AgentUnknown, "unknown"},
	}
	for _, tt := range tests {
		if string(tt.agent) != tt.want {
			t.Errorf("AgentType = %q, want %q", tt.agent, tt.want)
		}
	}
}

func TestStatusLabel(t *testing.T) {
	if got := StatusLabel(StatusIdle); got != "Idle" {
		t.Errorf("StatusLabel(Idle) = %q, want %q", got, "Idle")
	}
	if got := StatusLabel(StatusWaitingInput); got != "Waiting for input" {
		t.Errorf("StatusLabel(WaitingInput) = %q, want %q", got, "Waiting for input")
	}
}

func TestAgentLabel(t *testing.T) {
	if got := AgentLabel(AgentClaude); got != "Claude" {
		t.Errorf("AgentLabel(Claude) = %q, want %q", got, "Claude")
	}
}

func TestStatusColor(t *testing.T) {
	if got := StatusColor(StatusIdle); got != "#00D26A" {
		t.Errorf("StatusColor(Idle) = %q, want %q", got, "#00D26A")
	}
}
