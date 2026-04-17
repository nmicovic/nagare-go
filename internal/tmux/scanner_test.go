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

func TestParseAllPanesCapturesPaneID(t *testing.T) {
	raw := "work:0:0:claude:123:? claude:%7\n"
	got := ParseAllPanes(raw)
	panes := got["work"]
	if len(panes) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(panes))
	}
	if panes[0].PaneID != "%7" {
		t.Errorf("PaneID = %q, want %q", panes[0].PaneID, "%7")
	}
}

func TestComputeDisplayNames(t *testing.T) {
	sess := "cosmo-ai"
	panes := []PaneInfo{
		{WindowIndex: 1, PaneIndex: 0, AgentType: models.AgentClaude, WindowName: "terminal", PaneID: "%2"},
		{WindowIndex: 0, PaneIndex: 0, AgentType: models.AgentClaude, WindowName: "? claude", PaneID: "%1"},
	}
	got := ComputeDisplayNames(sess, panes)
	if got["%1"] != "cosmo-ai/claude_01" {
		t.Errorf("pane %%1 = %q, want cosmo-ai/claude_01", got["%1"])
	}
	if got["%2"] != "cosmo-ai/claude_02" {
		t.Errorf("pane %%2 = %q, want cosmo-ai/claude_02", got["%2"])
	}
}

func TestComputeDisplayNamesSinglePane(t *testing.T) {
	got := ComputeDisplayNames("work", []PaneInfo{
		{WindowIndex: 0, PaneIndex: 0, AgentType: models.AgentClaude, WindowName: "zsh", PaneID: "%3"},
	})
	if got["%3"] != "work" {
		t.Errorf("single pane name = %q, want work", got["%3"])
	}
}

func TestComputeDisplayNamesCustomWindowName(t *testing.T) {
	panes := []PaneInfo{
		{WindowIndex: 0, PaneIndex: 0, AgentType: models.AgentClaude, WindowName: "? claude", PaneID: "%1"},
		{WindowIndex: 1, PaneIndex: 0, AgentType: models.AgentClaude, WindowName: "planning", PaneID: "%2"},
	}
	got := ComputeDisplayNames("cosmo-ai", panes)
	if got["%1"] != "cosmo-ai/claude_01" {
		t.Errorf("pane %%1 = %q", got["%1"])
	}
	if got["%2"] != "cosmo-ai/planning" {
		t.Errorf("pane %%2 = %q", got["%2"])
	}
}

func TestComputeDisplayNamesMixedAgents(t *testing.T) {
	panes := []PaneInfo{
		{WindowIndex: 0, PaneIndex: 0, AgentType: models.AgentClaude, WindowName: "zsh", PaneID: "%1"},
		{WindowIndex: 1, PaneIndex: 0, AgentType: models.AgentGemini, WindowName: "zsh", PaneID: "%2"},
	}
	got := ComputeDisplayNames("proj", panes)
	if got["%1"] != "proj/claude_01" {
		t.Errorf("pane %%1 = %q", got["%1"])
	}
	if got["%2"] != "proj/gemini_01" {
		t.Errorf("pane %%2 = %q", got["%2"])
	}
}
