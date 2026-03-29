package tmux

import (
	"testing"

	"github.com/nemke/nagare-go/internal/models"
)

func TestParseSessions(t *testing.T) {
	raw := "my-project:$0:/home/user/project\nother:$1:/tmp/other\n"
	sessions := ParseSessions(raw)
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].Name != "my-project" {
		t.Errorf("name = %q, want %q", sessions[0].Name, "my-project")
	}
	if sessions[0].SessionID != "$0" {
		t.Errorf("id = %q, want %q", sessions[0].SessionID, "$0")
	}
	if sessions[0].Path != "/home/user/project" {
		t.Errorf("path = %q, want %q", sessions[0].Path, "/home/user/project")
	}
}

func TestParseSessionsEmpty(t *testing.T) {
	sessions := ParseSessions("")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestParseAllPanes(t *testing.T) {
	raw := "my-project:0:0:claude:12345\nmy-project:0:1:zsh:12346\nother:0:0:opencode:12347\n"
	panes := ParseAllPanes(raw)

	myPanes, ok := panes["my-project"]
	if !ok {
		t.Fatal("expected my-project panes")
	}
	if len(myPanes) != 1 {
		t.Fatalf("expected 1 agent pane, got %d", len(myPanes))
	}
	if myPanes[0].AgentType != models.AgentClaude {
		t.Errorf("agent = %q, want %q", myPanes[0].AgentType, models.AgentClaude)
	}

	otherPanes := panes["other"]
	if len(otherPanes) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(otherPanes))
	}
	if otherPanes[0].AgentType != models.AgentOpenCode {
		t.Errorf("agent = %q, want %q", otherPanes[0].AgentType, models.AgentOpenCode)
	}
}

func TestParseAllPanes_IgnoresNonAgent(t *testing.T) {
	raw := "sess:0:0:zsh:12345\nsess:0:1:vim:12346\n"
	panes := ParseAllPanes(raw)
	if len(panes) != 0 {
		t.Errorf("expected 0 agent sessions, got %d", len(panes))
	}
}
