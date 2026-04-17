package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/state"
	"github.com/nemke/nagare-go/internal/tmux"
)

// scanAll returns all agent sessions from tmux.
func scanAll() []models.Session {
	dir := state.DefaultStatesDir()
	return tmux.ScanSessions(state.LoadStatesByPaneID(dir), state.LoadAllStates(dir))
}

// ListAgentsHandler scans tmux for agent sessions and returns formatted list.
func ListAgentsHandler(mySession string) string {
	sessions := scanAll()

	var lines []string
	for _, s := range sessions {
		if s.Name == mySession {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s (%s) [%s] %s",
			s.Name, models.AgentLabel(s.AgentType),
			models.StatusLabel(s.Status), s.Path))
	}
	if len(lines) == 0 {
		return "No other agents found."
	}
	return strings.Join(lines, "\n")
}

// SendMessageInput is the input for send_message tool.
type SendMessageInput struct {
	Target  string `json:"target" jsonschema:"target session name"`
	Message string `json:"message" jsonschema:"message to send"`
}

// SendMessageHandler validates target is IDLE, writes message file, nudges via tmux.
func SendMessageHandler(mySession string, input SendMessageInput) string {
	session, err := findSession(input.Target)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	if session.Status != models.StatusIdle {
		return fmt.Sprintf("Error: %s is %s, not idle. Wait for it to finish.",
			input.Target, models.StatusLabel(session.Status))
	}

	msg := Message{
		ID:           NewMessageID(),
		FromSession:  mySession,
		ToSession:    input.Target,
		Content:      input.Message,
		ExpectsReply: false,
		Status:       StatusPending,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	// Nudge target — informational only, no reply expected
	nudge := fmt.Sprintf("FYI: '%s' sent you a message. Call check_messages() when convenient. No reply needed.", mySession)
	paneTarget := tmux.PaneTarget(session.Name, session.WindowIndex, session.PaneIndex)
	tmux.RunTmux("send-keys", "-t", paneTarget, nudge, "Enter")

	// Write as delivered (single write)
	msg.Status = StatusDelivered
	if err := WriteMessage(msg); err != nil {
		return fmt.Sprintf("Error writing message: %v", err)
	}

	return fmt.Sprintf("Message sent to %s.", input.Target)
}

// SendMessageAndWaitInput is the input for send_message_and_wait tool.
type SendMessageAndWaitInput struct {
	Target  string `json:"target" jsonschema:"target session name"`
	Message string `json:"message" jsonschema:"message to send"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"timeout in seconds (default 120)"`
}

// SendMessageAndWaitHandler sends a message and polls for a response.
func SendMessageAndWaitHandler(ctx context.Context, mySession string, input SendMessageAndWaitInput) string {
	timeout := input.Timeout
	if timeout == 0 {
		timeout = 120
	}

	session, err := findSession(input.Target)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	if session.Status != models.StatusIdle {
		return fmt.Sprintf("Error: %s is %s, not idle.",
			input.Target, models.StatusLabel(session.Status))
	}

	msg := Message{
		ID:           NewMessageID(),
		FromSession:  mySession,
		ToSession:    input.Target,
		Content:      input.Message,
		ExpectsReply: true,
		Status:       StatusPending,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	// Nudge target — reply required
	nudge := fmt.Sprintf("URGENT: '%s' sent you a message that requires a reply. Call check_messages() to read it, then reply() to respond.", mySession)
	paneTarget := tmux.PaneTarget(session.Name, session.WindowIndex, session.PaneIndex)
	tmux.RunTmux("send-keys", "-t", paneTarget, nudge, "Enter")

	// Write as delivered (single write)
	msg.Status = StatusDelivered
	if err := WriteMessage(msg); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	// Poll for response, respecting context cancellation
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "Cancelled: client disconnected."
		case <-time.After(2 * time.Second):
		}
		updated, err := ReadMessage(input.Target, msg.ID)
		if err != nil {
			continue
		}
		if updated.Status == StatusCompleted && updated.Response != nil {
			return *updated.Response
		}
	}

	return fmt.Sprintf("Timeout: %s did not respond within %d seconds.", input.Target, timeout)
}

// CheckMessagesHandler returns pending incoming messages + completed outgoing responses.
func CheckMessagesHandler(mySession string) string {
	var parts []string

	// Incoming messages (my inbox)
	inbox, _ := ListInbox(mySession)

	// Unread: pending or delivered (not yet seen)
	var unread []Message
	for _, m := range inbox {
		if m.Status == StatusPending || m.Status == StatusDelivered {
			unread = append(unread, m)
		}
	}
	if len(unread) > 0 {
		parts = append(parts, fmt.Sprintf("=== New Messages (%d) ===", len(unread)))
		for i, m := range unread {
			actionNote := "[INFORMATIONAL — no reply needed, do not respond]"
			if m.ExpectsReply {
				actionNote = fmt.Sprintf("[REPLY NEEDED — use reply('%s', 'your response')]", m.ID)
			}
			parts = append(parts, fmt.Sprintf("From: %s (sent %s)\n%s\nMessage ID: %s\n%s",
				m.FromSession, m.CreatedAt, actionNote, m.ID, m.Content))

			// Mark as read
			unread[i].Status = StatusRead
			WriteMessage(unread[i])
		}
	}

	// Awaiting reply: read but still needs a response
	var awaitingReply []Message
	for _, m := range inbox {
		if m.Status == StatusRead && m.ExpectsReply {
			awaitingReply = append(awaitingReply, m)
		}
	}
	if len(awaitingReply) > 0 {
		parts = append(parts, fmt.Sprintf("=== Awaiting Your Reply (%d) ===", len(awaitingReply)))
		for _, m := range awaitingReply {
			parts = append(parts, fmt.Sprintf("From: %s (sent %s) [REPLY NEEDED - use reply('%s', 'your response')]\nMessage ID: %s\n%s",
				m.FromSession, m.CreatedAt, m.ID, m.ID, m.Content))
		}
	}

	// Completed outgoing responses (scan other inboxes for messages FROM me)
	baseDir := MessagesDir()
	dirs, _ := os.ReadDir(baseDir)
	var responses []Message
	for _, d := range dirs {
		if !d.IsDir() || d.Name() == sanitizeName(mySession) {
			continue
		}
		msgs, _ := ListInbox(d.Name())
		for _, m := range msgs {
			if m.FromSession == mySession && m.Status == StatusCompleted && m.Response != nil {
				responses = append(responses, m)
			}
		}
	}
	if len(responses) > 0 {
		parts = append(parts, "=== Responses to Your Messages ===")
		for _, m := range responses {
			parts = append(parts, fmt.Sprintf("Response from %s:\n%s", m.ToSession, *m.Response))
		}
	}

	if len(parts) == 0 {
		return "No pending messages or responses."
	}
	return strings.Join(parts, "\n\n")
}

// ReplyInput is the input for reply tool.
type ReplyInput struct {
	MessageID string `json:"message_id" jsonschema:"ID of the message to reply to"`
	Content   string `json:"content" jsonschema:"reply content"`
}

// ReplyHandler updates a message with a response.
func ReplyHandler(mySession string, input ReplyInput) string {
	inbox, _ := ListInbox(mySession)
	for _, m := range inbox {
		if m.ID == input.MessageID {
			now := time.Now().UTC().Format(time.RFC3339)
			m.Status = StatusCompleted
			m.Response = &input.Content
			m.RespondedAt = &now
			if err := WriteMessage(m); err != nil {
				return fmt.Sprintf("Error saving reply: %v", err)
			}
			return fmt.Sprintf("Reply sent to %s.", m.FromSession)
		}
	}
	return fmt.Sprintf("Error: message %s not found in your inbox.", input.MessageID)
}

// Helper functions

func findSession(name string) (models.Session, error) {
	for _, s := range scanAll() {
		if s.Name == name {
			return s, nil
		}
	}
	return models.Session{}, fmt.Errorf("session '%s' not found", name)
}

func truncate(s string, maxLen int) string {
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen]) + "..."
}

// resolveMySession determines the current session name from cwd.
// Checks live tmux sessions first (authoritative), then falls back to registry.
func resolveMySession() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	// Live tmux sessions are the source of truth
	for _, s := range scanAll() {
		if s.Path == cwd {
			return s.Name
		}
	}
	// Fall back to registry (may be stale)
	reg := state.NewRegistry(state.DefaultRegistryPath())
	if s := reg.FindByPath(cwd); s != nil {
		return s.Name
	}
	return filepath.Base(cwd)
}
