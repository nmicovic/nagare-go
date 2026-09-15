package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	for _, session := range sessions {
		if session.Name == mySession {
			continue
		}
		status := models.StatusLabel(session.Status)
		waiting, err := waitingMessageCount(session)
		if err != nil {
			status += "; message state unavailable"
		} else if waiting > 0 {
			status += fmt.Sprintf("; %d message(s) waiting", waiting)
		}
		lines = append(lines, fmt.Sprintf("- %s (%s) [%s] %s",
			session.Name, models.AgentLabel(session.AgentType), status, session.Path))
	}
	if len(lines) == 0 {
		return "No other agents found."
	}
	return strings.Join(lines, "\n")
}

func waitingMessageCount(session models.Session) (int, error) {
	inbox, err := ListInboxFor(session.Name, session.PaneID)
	if err != nil {
		return 0, err
	}
	name, paneID, agentID := instanceOf(session)
	count := 0
	for _, message := range forInstance(inbox, name, paneID, agentID) {
		if message.Status == StatusPending || message.Status == StatusDelivered {
			count++
		}
	}
	return count, nil
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

	myPaneID, myAgentID := agentInstance()
	msg := Message{
		ID:           NewMessageID(),
		FromSession:  mySession,
		FromPaneID:   myPaneID,
		FromAgentID:  myAgentID,
		ToSession:    input.Target,
		ToPaneID:     session.PaneID,
		ToAgentID:    agentIDForPane(session.PaneID),
		Content:      input.Message,
		ExpectsReply: false,
		Status:       StatusPending,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	// Persist before notifying so an immediate inbox check cannot race the file.
	nudge := fmt.Sprintf("URGENT: message %s from '%s' requires attention. Call check_messages() now to read the persisted message.", msg.ID, mySession)
	if err := deliverMessage(session, msg, nudge, sendNudge); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return fmt.Sprintf("Message %s was saved and its notification was submitted to %s.", msg.ID, input.Target)
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
	if timeout < 0 {
		return "Error: timeout must be zero (the default) or a positive number of seconds."
	}

	session, err := findSession(input.Target)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	if session.Status != models.StatusIdle {
		return fmt.Sprintf("Error: %s is %s, not idle.",
			input.Target, models.StatusLabel(session.Status))
	}

	myPaneID, myAgentID := agentInstance()
	msg := Message{
		ID:           NewMessageID(),
		FromSession:  mySession,
		FromPaneID:   myPaneID,
		FromAgentID:  myAgentID,
		ToSession:    input.Target,
		ToPaneID:     session.PaneID,
		ToAgentID:    agentIDForPane(session.PaneID),
		Content:      input.Message,
		ExpectsReply: true,
		Status:       StatusPending,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	// Persist before notifying so an immediate inbox check cannot race the file.
	nudge := fmt.Sprintf("URGENT: message %s from '%s' requires a reply. Call check_messages() to read it, then reply('%s', ...) to respond.", msg.ID, mySession, msg.ID)
	if err := deliverMessage(session, msg, nudge, sendNudge); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return waitForMessage(ctx, msg, time.Duration(timeout)*time.Second, 2*time.Second, 30*time.Second)
}

func waitForMessage(ctx context.Context, message Message, replyTimeout, pollInterval, deliveryTimeout time.Duration) string {
	startedAt := time.Now()
	replyDeadline := startedAt.Add(replyTimeout)
	deliveryDeadline := startedAt.Add(deliveryTimeout)
	if deliveryDeadline.After(replyDeadline) {
		deliveryDeadline = replyDeadline
	}
	lastStatus := StatusDelivered

	for {
		nextDeadline := replyDeadline
		if lastStatus != StatusRead && lastStatus != StatusCompleted && deliveryDeadline.Before(nextDeadline) {
			nextDeadline = deliveryDeadline
		}
		wait := pollInterval
		if remaining := time.Until(nextDeadline); remaining < wait {
			wait = remaining
		}
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return fmt.Sprintf("Cancelled while waiting for message %s: client disconnected.", message.ID)
			case <-timer.C:
			}
		}

		updated, err := ReadStoredMessage(message)
		if err != nil {
			return fmt.Sprintf("Delivery state error for message %s: %v", message.ID, err)
		}
		lastStatus = updated.Status
		if updated.Status == StatusCompleted && updated.Response != nil {
			return *updated.Response
		}

		now := time.Now()
		if lastStatus != StatusRead && lastStatus != StatusCompleted && !now.Before(deliveryDeadline) {
			return fmt.Sprintf("Delivery unacknowledged: message %s was saved and the target pane was notified, but %s did not read it within %s.",
				message.ID, message.ToSession, deliveryDeadline.Sub(startedAt))
		}
		if !now.Before(replyDeadline) {
			if lastStatus == StatusRead || lastStatus == StatusCompleted {
				return fmt.Sprintf("Timeout: %s read message %s but did not reply within %s.",
					message.ToSession, message.ID, replyTimeout)
			}
			return fmt.Sprintf("Delivery unacknowledged: message %s was saved and the target pane was notified, but %s did not read it within %s.",
				message.ID, message.ToSession, replyTimeout)
		}
	}
}

// CheckMessagesHandler returns pending incoming messages + completed outgoing responses.
func CheckMessagesHandler(mySession string) string {
	var parts []string

	// Incoming messages (my inbox)
	myPaneID, myAgentID := agentInstance()
	inbox, err := ListInboxFor(mySession, myPaneID)
	if err != nil {
		return fmt.Sprintf("Error reading inbox: %v", err)
	}
	inbox = forInstance(inbox, mySession, myPaneID, myAgentID)

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

			// Mark as read. The sender treats this durable state change as the
			// delivery acknowledgment.
			unread[i].Status = StatusRead
			if err := WriteMessage(unread[i]); err != nil {
				parts = append(parts, fmt.Sprintf("WARNING: could not acknowledge message %s as read: %v", m.ID, err))
			}
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

	allMessages, err := ListAllMessages()
	if err != nil {
		parts = append(parts, fmt.Sprintf("WARNING: outgoing message state unavailable: %v", err))
	} else {
		var outgoing, responses []Message
		for _, message := range allMessages {
			if !message.sentBy(mySession, myPaneID, myAgentID) {
				continue
			}
			switch {
			case message.Status == StatusPending || message.Status == StatusDelivered:
				outgoing = append(outgoing, message)
			case message.Status == StatusRead && message.ExpectsReply:
				outgoing = append(outgoing, message)
			case message.Status == StatusCompleted && message.Response != nil:
				responses = append(responses, message)
			}
		}
		if len(outgoing) > 0 {
			sort.Slice(outgoing, func(i, j int) bool {
				return outgoing[i].CreatedAt > outgoing[j].CreatedAt
			})
			if len(outgoing) > 10 {
				outgoing = outgoing[:10]
			}
			parts = append(parts, "=== Outgoing Message State ===")
			for _, message := range outgoing {
				state := "SAVED — waiting for target notification"
				if message.Status == StatusDelivered {
					state = "NOTIFIED — waiting for target to read"
				} else if message.Status == StatusRead {
					state = "READ — waiting for reply"
				}
				parts = append(parts, fmt.Sprintf("To: %s (sent %s)\nMessage ID: %s\nStatus: %s",
					message.ToSession, message.CreatedAt, message.ID, state))
			}
		}
		if len(responses) > 0 {
			sort.Slice(responses, func(i, j int) bool {
				return responses[i].CreatedAt > responses[j].CreatedAt
			})
			if len(responses) > 10 {
				responses = responses[:10]
			}
			parts = append(parts, "=== Responses to Your Messages ===")
			for _, message := range responses {
				parts = append(parts, fmt.Sprintf("Response from %s to message %s:\n%s",
					message.ToSession, message.ID, *message.Response))
			}
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
	inbox, err := ListInboxFor(mySession, os.Getenv("TMUX_PANE"))
	if err != nil {
		return fmt.Sprintf("Error reading inbox: %v", err)
	}
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

// nudgeFunc submits a persisted-message notice to an agent pane. It takes the
// whole session because delivery depends on the agent: only an agent that
// reports status can confirm that it took the notice.
type nudgeFunc func(target models.Session, text string) error

// MessagePersistedMarker appears in every error a send returns after the
// message file has been written. A caller retrying a send must stop once it
// sees this: the mailbox entry exists, and only the pane notice failed.
const MessagePersistedMarker = "was saved"

// deliverMessage persists before notifying so an immediate inbox check cannot
// race the file. It records notification only if the target has not already
// advanced the message to read or completed.
func deliverMessage(session models.Session, message Message, nudge string, notify nudgeFunc) error {
	message.Status = StatusPending
	if err := WriteMessage(message); err != nil {
		return fmt.Errorf("writing message: %w", err)
	}
	if err := notify(session, nudge); err != nil {
		return fmt.Errorf("message %s was saved, but target notification failed: %w", message.ID, err)
	}
	stored, err := ReadStoredMessage(message)
	if err != nil {
		return fmt.Errorf("message %s was saved and target notification submitted, but its delivery state could not be read: %w", message.ID, err)
	}
	if stored.Status != StatusPending {
		return nil
	}
	stored.Status = StatusDelivered
	if err := WriteMessage(stored); err != nil {
		return fmt.Errorf("message %s was saved and target notification submitted, but notification state could not be recorded: %w", message.ID, err)
	}
	return nil
}

// sendNudge submits a notice to an agent pane and waits for that agent to
// report that it took it, so a notice left sitting in the input buffer is
// reported as a delivery failure instead of passing as delivered.
func sendNudge(target models.Session, text string) error {
	return tmux.SubmitPrompt(tmux.Submit{
		Target:  paneTargetFor(target),
		Text:    text,
		Reports: models.ReportsStatus(target.AgentType),
	})
}

// paneTargetFor builds a tmux pane target for a discovered session. It uses
// SessionName (the real tmux session) rather than Name (the display name,
// which can contain "/" for multi-pane disambiguation and is not a valid
// tmux target).
func paneTargetFor(s models.Session) string {
	return tmux.PaneTarget(s.SessionName, s.WindowIndex, s.PaneIndex)
}

// resolveSession finds a session by name. Exact match wins. If no exact match,
// a prefix match on "{name}/..." is tried. Multiple prefix matches produce an
// ambiguity error listing the candidates.
func resolveSession(name string, sessions []models.Session) (models.Session, error) {
	for _, s := range sessions {
		if s.Name == name {
			return s, nil
		}
	}
	prefix := name + "/"
	var matches []models.Session
	for _, s := range sessions {
		if strings.HasPrefix(s.Name, prefix) {
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 0:
		return models.Session{}, fmt.Errorf("session '%s' not found", name)
	case 1:
		return matches[0], nil
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = m.Name
		}
		return models.Session{}, fmt.Errorf("'%s' is ambiguous — matches %s. Please specify one.",
			name, strings.Join(names, ", "))
	}
}

func findSession(name string) (models.Session, error) {
	return resolveSession(name, scanAll())
}

func truncate(s string, maxLen int) string {
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen]) + "..."
}

// resolveMySession determines the current session name.
// Prefers TMUX_PANE (unambiguous when multiple agents share a cwd),
// falling back to cwd match against live tmux sessions, then the registry.
func resolveMySession() string {
	paneID := os.Getenv("TMUX_PANE")
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}

	sessions := scanAll()

	// 1. Match by pane_id (unambiguous when multiple agents share a cwd)
	if paneID != "" {
		for _, s := range sessions {
			if s.PaneID == paneID {
				return s.Name
			}
		}
	}

	// 2. Fall back to cwd match
	if cwd != "" {
		for _, s := range sessions {
			if s.Path == cwd {
				return s.Name
			}
		}
	}

	// 3. Fall back to registry
	if cwd != "" {
		reg := state.NewRegistry(state.DefaultRegistryPath())
		if s := reg.FindByPath(cwd); s != nil {
			return s.Name
		}
	}
	if cwd == "" {
		return "unknown"
	}
	return filepath.Base(cwd)
}
