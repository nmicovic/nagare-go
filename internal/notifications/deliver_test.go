package notifications

import "testing"

func TestBuildToastMessage(t *testing.T) {
	msg := BuildToastMessage("my-session", "needs_input", "permission_prompt")
	if msg == "" {
		t.Error("message should not be empty")
	}
}

func TestBuildToastMessage_TaskComplete(t *testing.T) {
	msg := BuildToastMessage("my-session", "task_complete", "")
	if msg == "" {
		t.Error("message should not be empty")
	}
}
