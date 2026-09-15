package mcp

import (
	"os"
	"strings"

	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/state"
)

// Neither a tmux pane nor a display name is an agent's identity.
//
// A pane outlives the agent that ran in it: exit one agent, start another in
// the same pane, and a filter keyed on the pane hands the new occupant the
// previous one's outbox — and every reply addressed to it. That was observed
// across two unrelated repositories, delivered silently, with the receiving
// agent given an action item for a checkout it did not have.
//
// A display name is no better: it is reused across repositories and across
// months, and it legitimately *changes* for one agent when panes are added and
// it gains a "/claude_02" suffix — which is why the pane check was introduced
// in the first place.
//
// What actually identifies an agent instance is the session ID it reports
// through its hooks. It is written per pane on every event, so it names the
// agent occupying the pane *now*, and it changes when a new agent takes over.
// Messages record it at send time; matching prefers it and degrades to the
// weaker identities only for records written before it existed.

// agentInstance returns the pane the current agent occupies and the session ID
// it reports there. Either may be empty: the agent may not be under tmux, and
// an agent that reports no status at all (Crush) writes no state file.
func agentInstance() (paneID, agentID string) {
	paneID = os.Getenv("TMUX_PANE")
	if paneID == "" {
		return "", ""
	}
	return paneID, agentIDForPane(paneID)
}

// agentIDForPane returns the session ID of the agent occupying paneID now.
func agentIDForPane(paneID string) string {
	if paneID == "" {
		return ""
	}
	current, ok := state.LoadStatesByPaneID(state.DefaultStatesDir())[paneID]
	if !ok {
		return ""
	}
	return current.SessionID
}

// instanceOf returns the identity triple of a discovered session.
func instanceOf(session models.Session) (name, paneID, agentID string) {
	return session.Name, session.PaneID, agentIDForPane(session.PaneID)
}

// sentBy reports whether m was sent by the agent instance identified by the
// arguments, preferring the strongest identity both sides carry.
func (m Message) sentBy(session, paneID, agentID string) bool {
	if m.FromAgentID != "" && agentID != "" {
		return m.FromAgentID == agentID
	}
	if m.FromPaneID != "" && paneID != "" {
		return m.FromPaneID == paneID && sameAgent(m.FromSession, session)
	}
	return m.FromSession == session
}

// addressedTo reports whether m was addressed to this agent instance. The pane
// inbox is keyed by pane, so a message left unread by a pane's previous
// occupant would otherwise be read by whoever starts there next.
func (m Message) addressedTo(session, paneID, agentID string) bool {
	if m.ToAgentID != "" && agentID != "" {
		return m.ToAgentID == agentID
	}
	if m.ToPaneID != "" && paneID != "" {
		return m.ToPaneID == paneID && sameAgent(m.ToSession, session)
	}
	return m.ToSession == session
}

// sameAgent reports whether two display names can name one agent across a
// rename. A display name is "{tmux session}/{worktree or pane suffix}", and
// only the suffix moves: a pane gains one when a second agent joins, and the
// user can rename the window behind it at will. The tmux session names the
// repository, so a name whose root changed is a different agent — which is the
// case a reused pane produces.
func sameAgent(recorded, current string) bool {
	return nameRoot(recorded) == nameRoot(current)
}

func nameRoot(name string) string {
	root, _, _ := strings.Cut(name, "/")
	return root
}

// forInstance keeps only the messages addressed to one agent instance.
func forInstance(messages []Message, session, paneID, agentID string) []Message {
	kept := make([]Message, 0, len(messages))
	for _, message := range messages {
		if message.addressedTo(session, paneID, agentID) {
			kept = append(kept, message)
		}
	}
	return kept
}
