package tmux

import (
	"fmt"
	"strings"
	"time"

	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/state"
)

// Typing text into an agent pane is not the same as submitting it, and a pane
// that is running an agent is not the same as an agent ready to be typed at.
// Both cost a ticket its handoff, silently, and the second can cost the agent
// its life:
//
//   - A freshly launched agent is not listening yet. Claude Code opens a new
//     directory — which every Nagare-managed worktree is — by asking whether
//     the folder is trusted, and waits there indefinitely. Text typed at that
//     dialog is discarded and the Enter after it answers the dialog, where the
//     default choice is "No, exit". So the old blind send could kill the agent
//     it had just started for a ticket, and the board would go on showing the
//     ticket as running.
//   - An agent that is listening debounces its input. A burst of literal text
//     reads as a paste, and an Enter arriving inside that window is appended to
//     the buffer as a newline rather than submitting it, leaving the ticket
//     sitting unsent in the prompt.
//
// So nothing is typed until the agent has reported itself through its hooks —
// which it cannot do while a trust dialog is up, making that the exact signal
// needed — and the Enter is then confirmed by the agent rather than assumed,
// resent until the agent reports it took the prompt.

// Submit describes one prompt handed to an agent pane.
type Submit struct {
	// Target is a pane ID ("%42") or a pane target ("session:0.1").
	Target string
	Text   string
	// Reports says whether this agent reports status through nagare's hooks.
	// Every supported agent does except Crush, which has nothing to confirm
	// against and so keeps a blind best-effort submit.
	Reports bool
	// NotBefore discards hook state older than itself. A pane keeps the state
	// files of every agent that has run in it, so a launch must not read the
	// previous occupant's as readiness.
	NotBefore time.Time
}

// submitWaits are the pauses before each Enter: the first clears the paste
// debounce, the rest back off for an agent that is slow to take the prompt.
var submitWaits = []time.Duration{
	250 * time.Millisecond,
	500 * time.Millisecond,
	1000 * time.Millisecond,
	2 * time.Second,
}

var (
	// submitReady is how long an agent is given to finish starting, which for a
	// new worktree means how long the user has to answer its trust prompt.
	submitReady = 60 * time.Second
	// submitConfirm is how long the agent is given to report that it took the
	// prompt; submitPoll is how often its state file is re-read.
	submitConfirm = 2 * time.Second
	submitPoll    = 250 * time.Millisecond
)

// SubmitPrompt types text into an agent TUI and returns once that agent has
// reported accepting it.
func SubmitPrompt(submit Submit) error {
	if strings.TrimSpace(submit.Target) == "" {
		return fmt.Errorf("pane target is empty")
	}
	pane := resolvePaneID(submit.Target)
	before, ready := agentState(pane, submit.NotBefore)
	if submit.Reports && !ready {
		// Nothing is typed at an agent that has not started listening: at a
		// trust dialog the text is discarded and the Enter answers "No, exit".
		before, ready = waitForAgent(pane, submit.NotBefore)
		if !ready {
			return fmt.Errorf("agent in pane %s did not start accepting input within %s; if it is asking whether you trust the folder, answer that prompt in the pane", submit.Target, submitReady)
		}
	}
	if _, err := RunStrict("send-keys", "-t", submit.Target, "-l", submit.Text); err != nil {
		return err
	}
	for _, wait := range submitWaits {
		time.Sleep(wait)
		if _, err := RunStrict("send-keys", "-t", submit.Target, "Enter"); err != nil {
			return err
		}
		if !submit.Reports {
			// This agent reports no state, so there is nothing to confirm
			// against and resending Enter would only guess.
			return nil
		}
		if acceptedPrompt(pane, before, submit.NotBefore) {
			return nil
		}
	}
	return fmt.Errorf("pane %s took the text but the agent did not report accepting it after %d submits; the text is waiting in its prompt", submit.Target, len(submitWaits))
}

// waitForAgent waits for the agent in pane to report itself for the first time.
func waitForAgent(pane string, notBefore time.Time) (models.SessionState, bool) {
	deadline := time.Now().Add(submitReady)
	for {
		if current, ok := agentState(pane, notBefore); ok {
			return current, true
		}
		if time.Now().After(deadline) || !PaneExists(pane) {
			return models.SessionState{}, false
		}
		time.Sleep(submitPoll)
	}
}

// acceptedPrompt waits for the agent in pane to report any new hook event. A
// submitted prompt writes one; an Enter swallowed by the input buffer does not.
func acceptedPrompt(pane string, before models.SessionState, notBefore time.Time) bool {
	deadline := time.Now().Add(submitConfirm)
	for {
		if current, ok := agentState(pane, notBefore); ok && current != before {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(submitPoll)
	}
}

// agentState returns what the agent occupying pane last reported, ignoring
// anything a previous occupant left behind before notBefore.
func agentState(pane string, notBefore time.Time) (models.SessionState, bool) {
	if pane == "" {
		return models.SessionState{}, false
	}
	current, ok := state.LoadStatesByPaneID(state.DefaultStatesDir())[pane]
	if !ok || current.State == "dead" {
		return models.SessionState{}, false
	}
	if !notBefore.IsZero() {
		at, err := time.Parse(time.RFC3339, current.Timestamp)
		if err != nil || at.Before(notBefore.Truncate(time.Second)) {
			return models.SessionState{}, false
		}
	}
	return current, true
}

// resolvePaneID turns any tmux target into the %-prefixed pane ID that hook
// state files are keyed by.
func resolvePaneID(target string) string {
	if strings.HasPrefix(target, "%") {
		return target
	}
	return strings.TrimSpace(RunTmux("display-message", "-p", "-t", target, "#{pane_id}"))
}
