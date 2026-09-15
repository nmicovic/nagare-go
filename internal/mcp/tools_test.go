package mcp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/state"
)

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

// TestPaneTargetForUsesSessionName locks in the invariant that pane targets
// are built from SessionName (the real tmux session), not Name (the display
// name which can contain "/" for multi-pane disambiguation).
func TestPaneTargetForUsesSessionName(t *testing.T) {
	s := models.Session{
		Name:        "cosmo-ai/claude_02",
		SessionName: "cosmo-ai",
		WindowIndex: 1,
		PaneIndex:   0,
	}
	got := paneTargetFor(s)
	if got != "cosmo-ai:1.0" {
		t.Errorf("paneTargetFor = %q, want cosmo-ai:1.0", got)
	}
	if strings.Contains(got, "/") {
		t.Errorf("pane target leaked display-name slash: %q", got)
	}
}

func TestDeliverMessageSurvivesImmediateCheckAndDisplayNameChanges(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%backend")
	targetAtSend := "cosmic-platform-backend/claude_01"
	targetAfterRename := "cosmic-platform-backend/api"
	msg := Message{
		ID:           "race-proof",
		FromSession:  "cosmic-platform-frontend/claude_02",
		FromPaneID:   "%frontend",
		ToSession:    targetAtSend,
		ToPaneID:     "%backend",
		Content:      strings.Repeat("payload-", 1024),
		ExpectsReply: true,
		Status:       StatusPending,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	session := models.Session{
		Name: targetAtSend, SessionName: "cosmic-platform-backend",
		WindowIndex: 1, PaneIndex: 0, PaneID: "%backend",
	}
	notified := false
	err := deliverMessage(session, msg, "urgent notice", func(paneTarget, notice string) error {
		notified = true
		if paneTarget != "cosmic-platform-backend:1.0" || notice != "urgent notice" {
			t.Fatalf("nudge = %q, %q", paneTarget, notice)
		}
		// Simulate the target reacting immediately after its display name changed.
		inbox := CheckMessagesHandler(targetAfterRename)
		for _, want := range []string{msg.FromSession, msg.ID, "REPLY NEEDED"} {
			if !strings.Contains(inbox, want) {
				t.Errorf("immediate inbox missing %q:\n%s", want, inbox)
			}
		}
		if !strings.Contains(inbox, msg.Content) {
			t.Errorf("immediate inbox truncated an %d-byte payload", len(msg.Content))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !notified {
		t.Fatal("target was not notified")
	}
	stored, err := ReadStoredMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != StatusRead {
		t.Fatalf("message status regressed after immediate read: %q", stored.Status)
	}
	reply := ReplyHandler(targetAfterRename, ReplyInput{MessageID: msg.ID, Content: "Contract verified."})
	if !strings.Contains(reply, "Reply sent") {
		t.Fatalf("reply failed after target rename: %s", reply)
	}

	t.Setenv("TMUX_PANE", "%frontend")
	response := CheckMessagesHandler("cosmic-platform-frontend/ui")
	if !strings.Contains(response, "Contract verified.") {
		t.Fatalf("sender missed response after its display name changed:\n%s", response)
	}
}

func TestDeliverMessageNeverNudgesWhenPersistenceFails(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".local"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	notified := false
	err := deliverMessage(models.Session{}, Message{
		ID: "write-fails", ToSession: "backend",
	}, "notice", func(string, string) error {
		notified = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "writing message") {
		t.Fatalf("persistence error = %v", err)
	}
	if notified {
		t.Fatal("target was nudged without a durable inbox message")
	}
}

func TestDeliverMessageRetainsInboxRecordWhenNudgeFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	msg := Message{ID: "notify-fails", ToSession: "backend", Status: StatusPending}
	err := deliverMessage(models.Session{}, msg, "notice", func(string, string) error {
		return errors.New("pane disappeared")
	})
	if err == nil || !strings.Contains(err.Error(), "was saved") {
		t.Fatalf("notification error = %v", err)
	}
	stored, readErr := ReadMessage(msg.ToSession, msg.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if stored.Status != StatusPending {
		t.Fatalf("stored status = %q, want pending for an unnotified message", stored.Status)
	}
}

func TestWaitingMessageCountUsesStablePaneInbox(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	session := models.Session{Name: "backend/renamed", PaneID: "%backend"}
	for _, message := range []Message{
		{ID: "pending", ToSession: "backend/original", ToPaneID: session.PaneID, Status: StatusPending},
		{ID: "delivered", ToSession: "backend/original", ToPaneID: session.PaneID, Status: StatusDelivered},
		{ID: "read", ToSession: "backend/original", ToPaneID: session.PaneID, Status: StatusRead},
	} {
		if err := WriteMessage(message); err != nil {
			t.Fatal(err)
		}
	}
	count, err := waitingMessageCount(session)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("waiting count = %d, want 2", count)
	}
}

func TestCheckMessagesShowsOutgoingLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%frontend")
	message := Message{
		ID:           "observable",
		FromSession:  "frontend/original",
		FromPaneID:   "%frontend",
		ToSession:    "backend/original",
		ToPaneID:     "%backend",
		Status:       StatusDelivered,
		CreatedAt:    "2026-09-04T20:26:57Z",
		ExpectsReply: true,
	}
	if err := WriteMessage(message); err != nil {
		t.Fatal(err)
	}
	notified := CheckMessagesHandler("frontend/renamed")
	for _, want := range []string{"Outgoing Message State", message.ID, "NOTIFIED", "waiting for target to read"} {
		if !strings.Contains(notified, want) {
			t.Errorf("notified state missing %q:\n%s", want, notified)
		}
	}

	message.Status = StatusRead
	if err := WriteMessage(message); err != nil {
		t.Fatal(err)
	}
	read := CheckMessagesHandler("frontend/renamed")
	if !strings.Contains(read, "READ — waiting for reply") {
		t.Fatalf("read state not visible:\n%s", read)
	}

	response := "API contract verified."
	message.Status = StatusCompleted
	message.Response = &response
	if err := WriteMessage(message); err != nil {
		t.Fatal(err)
	}
	completed := CheckMessagesHandler("frontend/renamed")
	for _, want := range []string{message.ID, response} {
		if !strings.Contains(completed, want) {
			t.Errorf("completed state missing %q:\n%s", want, completed)
		}
	}
}

func TestWaitForMessageDistinguishesUnreadFromReadWithoutReply(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	message := Message{
		ID: "wait-state", ToSession: "backend", ToPaneID: "%backend", Status: StatusDelivered,
	}
	if err := WriteMessage(message); err != nil {
		t.Fatal(err)
	}
	unread := waitForMessage(context.Background(), message, 20*time.Millisecond, time.Millisecond, 5*time.Millisecond)
	for _, want := range []string{"Delivery unacknowledged", message.ID, "did not read"} {
		if !strings.Contains(unread, want) {
			t.Errorf("unread result missing %q: %s", want, unread)
		}
	}

	message.Status = StatusRead
	if err := WriteMessage(message); err != nil {
		t.Fatal(err)
	}
	read := waitForMessage(context.Background(), message, 5*time.Millisecond, time.Millisecond, 5*time.Millisecond)
	for _, want := range []string{"Timeout", message.ID, "read", "did not reply"} {
		if !strings.Contains(read, want) {
			t.Errorf("read result missing %q: %s", want, read)
		}
	}
}

func TestWaitForMessageObservesAcknowledgmentAndReply(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	message := Message{
		ID: "round-trip", ToSession: "backend", ToPaneID: "%backend", Status: StatusDelivered,
	}
	if err := WriteMessage(message); err != nil {
		t.Fatal(err)
	}
	writeDone := make(chan error, 1)
	go func() {
		time.Sleep(2 * time.Millisecond)
		stored, err := ReadStoredMessage(message)
		if err != nil {
			writeDone <- err
			return
		}
		stored.Status = StatusRead
		if err := WriteMessage(stored); err != nil {
			writeDone <- err
			return
		}
		time.Sleep(2 * time.Millisecond)
		response := "round trip complete"
		stored.Status = StatusCompleted
		stored.Response = &response
		writeDone <- WriteMessage(stored)
	}()

	result := waitForMessage(context.Background(), message, 100*time.Millisecond, time.Millisecond, 20*time.Millisecond)
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	if result != "round trip complete" {
		t.Fatalf("result = %q", result)
	}
}

func TestCheckMessagesReportsUnreadableInbox(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%backend")
	dir := paneInboxDir("%backend")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "msg_broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := CheckMessagesHandler("backend")
	if !strings.Contains(result, "Error reading inbox") || strings.Contains(result, "No pending messages") {
		t.Fatalf("read failure was hidden: %s", result)
	}
}

func TestCheckMessagesRecoversLegacyDisplayNameInbox(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%backend")
	message := Message{
		ID:           "legacy-recovery",
		FromSession:  "frontend/claude_02",
		ToSession:    "backend",
		Content:      "Persisted before pane identities were recorded.",
		Status:       StatusDelivered,
		ExpectsReply: true,
	}
	if err := WriteMessage(message); err != nil {
		t.Fatal(err)
	}
	result := CheckMessagesHandler("backend")
	for _, want := range []string{message.ID, message.FromSession, message.Content, "REPLY NEEDED"} {
		if !strings.Contains(result, want) {
			t.Errorf("legacy inbox recovery missing %q:\n%s", want, result)
		}
	}
}

// writeAgentState records a pane's current occupant the way the hook handler
// does, which is where message identity gets its agent instance. Timestamps
// advance because a pane keeps the state file of every agent that has run in
// it, and the newest live one names the current occupant.
var agentStateClock = time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)

func writeAgentState(t *testing.T, paneID, agentID string) {
	t.Helper()
	dir := state.DefaultStatesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	agentStateClock = agentStateClock.Add(time.Second)
	if err := state.WriteState(dir, models.SessionState{
		State: "idle", SessionID: agentID, PaneID: paneID,
		Event: "Stop", Timestamp: agentStateClock.Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReusedPaneDoesNotInheritThePreviousAgentsMessages(t *testing.T) {
	// Reported from a live session: a pane outlives the agent that ran in it,
	// so keying outgoing state on the pane handed the next occupant another
	// repository's reply, complete with an action item it could not act on.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%19")
	response := "ACTION FOR YOU — the beacon doesn't send the session"
	previous := Message{
		ID:          "3c2cbb181078",
		FromSession: "cosmic-platform-frontend/claude_02",
		FromPaneID:  "%19",
		ToSession:   "cosmic-platform-backend",
		ToPaneID:    "%10",
		Status:      StatusCompleted,
		Response:    &response,
		CreatedAt:   "2026-09-12T12:55:53Z",
	}
	if err := WriteMessage(previous); err != nil {
		t.Fatal(err)
	}

	// A different agent, in a different repository, now occupies pane %19.
	writeAgentState(t, "%19", "lab-frontend-session")
	inbox := CheckMessagesHandler("cosmiclab-frontend/labFrontend")
	if strings.Contains(inbox, previous.ID) || strings.Contains(inbox, response) {
		t.Fatalf("new occupant of pane %%19 inherited the previous agent's reply:\n%s", inbox)
	}

	// The agent that actually sent it still sees it, renamed or not.
	writeAgentState(t, "%19", "platform-frontend-session")
	own := CheckMessagesHandler("cosmic-platform-frontend/ui")
	if !strings.Contains(own, response) {
		t.Fatalf("sender lost its own reply:\n%s", own)
	}
}

func TestAgentKeepsItsMessagesWhenPanesAreRenumbered(t *testing.T) {
	// The latent half of the same defect: substituting the pane check for the
	// name check silently discarded an agent's own outgoing state whenever its
	// pane ID changed, as a tmux server restart makes it.
	t.Setenv("HOME", t.TempDir())
	response := "Contract verified."
	sent := Message{
		ID:          "renumbered",
		FromSession: "backend/claude_01",
		FromPaneID:  "%19",
		FromAgentID: "backend-session",
		ToSession:   "frontend",
		ToPaneID:    "%10",
		Status:      StatusCompleted,
		Response:    &response,
		CreatedAt:   "2026-09-12T12:55:53Z",
	}
	if err := WriteMessage(sent); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_PANE", "%3")
	writeAgentState(t, "%3", "backend-session")
	inbox := CheckMessagesHandler("backend/claude_01")
	if !strings.Contains(inbox, response) {
		t.Fatalf("agent lost its own reply after its pane was renumbered:\n%s", inbox)
	}
}

func TestSentMessagesRecordTheAgentInstance(t *testing.T) {
	message := Message{FromSession: "repo/one", FromPaneID: "%19", FromAgentID: "session-a"}
	cases := []struct {
		name                     string
		session, paneID, agentID string
		want                     bool
	}{
		{"same instance", "repo/one", "%19", "session-a", true},
		{"same instance, renamed window", "repo/two", "%19", "session-a", true},
		{"same instance, renumbered pane", "repo/one", "%3", "session-a", true},
		{"new agent in the same pane", "other/one", "%19", "session-b", false},
		{"legacy record, same repo", "repo/two", "%19", "", true},
		{"legacy record, reused pane", "other/one", "%19", "", false},
	}
	for _, testCase := range cases {
		probe := message
		if testCase.agentID == "" {
			probe.FromAgentID = ""
		}
		if got := probe.sentBy(testCase.session, testCase.paneID, testCase.agentID); got != testCase.want {
			t.Errorf("%s: sentBy = %v, want %v", testCase.name, got, testCase.want)
		}
	}
}

func TestPaneInboxDoesNotLeakToTheNextOccupant(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%19")
	unread := Message{
		ID: "unread", FromSession: "backend", ToSession: "platform-frontend/claude_02",
		ToPaneID: "%19", ToAgentID: "platform-session", Status: StatusDelivered,
		Content: "the beacon doesn't send the session", CreatedAt: "2026-09-12T12:55:53Z",
	}
	if err := WriteMessage(unread); err != nil {
		t.Fatal(err)
	}
	writeAgentState(t, "%19", "lab-session")
	inbox := CheckMessagesHandler("cosmiclab-frontend/labFrontend")
	if strings.Contains(inbox, unread.Content) {
		t.Fatalf("new occupant read a message addressed to the previous one:\n%s", inbox)
	}
	writeAgentState(t, "%19", "platform-session")
	own := CheckMessagesHandler("platform-frontend/claude_02")
	if !strings.Contains(own, unread.Content) {
		t.Fatalf("addressed agent did not receive its message:\n%s", own)
	}
}
