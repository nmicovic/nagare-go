package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestToolNamesCoversEveryMCPTool(t *testing.T) {
	want := []string{"check_messages", "get_ticket", "list_agents", "list_tickets", "reply", "send_message", "send_message_and_wait", "submit_ticket"}
	got := ToolNames()
	if len(got) != len(want) {
		t.Fatalf("ToolNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ToolNames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRunToolUnknownName(t *testing.T) {
	_, err := RunTool(context.Background(), "nope", nil)
	if err == nil {
		t.Fatal("expected an error for an unknown tool")
	}
	// The error should tell the caller what is available.
	if !strings.Contains(err.Error(), "list_agents") {
		t.Errorf("error should list available tools, got %q", err)
	}
}

func TestRunToolInvalidJSON(t *testing.T) {
	_, err := RunTool(context.Background(), "send_message", []byte("{not json"))
	if err == nil {
		t.Fatal("expected an error for malformed arguments")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("error should mention invalid JSON, got %q", err)
	}
}

// Tools with no required arguments must work with no arguments at all, since
// pi's extension passes "{}" and a shell caller may pass nothing.
func TestDecodeArgsEmptyIsNotAnError(t *testing.T) {
	var input SendMessageInput
	for _, args := range []string{"", "   ", "{}"} {
		if err := decodeArgs([]byte(args), &input); err != nil {
			t.Errorf("decodeArgs(%q) = %v, want nil", args, err)
		}
	}
}

func TestDecodeArgsPopulatesInput(t *testing.T) {
	var input SendMessageAndWaitInput
	if err := decodeArgs([]byte(`{"target":"api","message":"hi","timeout":5}`), &input); err != nil {
		t.Fatal(err)
	}
	if input.Target != "api" || input.Message != "hi" || input.Timeout != 5 {
		t.Errorf("decoded = %+v", input)
	}
}
