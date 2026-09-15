package tmux

import (
	"fmt"
	"strings"
	"time"

	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/state"
)

// Typing text into an agent TUI is not the same as submitting it. Agent TUIs
// debounce their input: a burst of literal text is treated as a paste, and an
// Enter arriving inside that window is appended to the buffer as a newline
// instead of submitting it. The failure is silent and it is the worst kind —
// the ticket sits unsent in the agent's prompt while nagare reports the work
// handed off, so the board shows an agent running that has not read anything.
//
// So the submit is confirmed by the agent rather than assumed. Every agent that
// reports status writes a hook state file when it accepts a prompt
// (UserPromptSubmit and its per-agent spellings), so Enter is resent until that
// file changes. An agent that reports no state at all — Crush — has nothing to
// confirm against and keeps the old best-effort single Enter.

// submitWaits are the pauses before each Enter: the first clears the paste
// debounce, the rest back off for an agent still drawing its startup.
var submitWaits = []time.Duration{
	250 * time.Millisecond,
	500 * time.Millisecond,
	1000 * time.Millisecond,
}

// submitConfirm is how long the agent is given to report that it took the
// prompt; submitPoll is how often its state file is re-read.
var (
	submitConfirm = 1500 * time.Millisecond
	submitPoll    = 150 * time.Millisecond
)

// SubmitPrompt types text into the agent TUI at target and returns once that
// agent has reported accepting it. target may be a pane ID ("%42") or a pane
// target ("session:0.1").
func SubmitPrompt(target, text string) error {
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("pane target is empty")
	}
	if _, err := RunStrict("send-keys", "-t", target, "-l", text); err != nil {
		return err
	}
	pane := resolvePaneID(target)
	before, reports := agentState(pane)
	for _, wait := range submitWaits {
		time.Sleep(wait)
		if _, err := RunStrict("send-keys", "-t", target, "Enter"); err != nil {
			return err
		}
		if !reports {
			// This agent reports no state, so there is nothing to confirm
			// against and resending Enter would only guess.
			return nil
		}
		if acceptedPrompt(pane, before) {
			return nil
		}
	}
	return fmt.Errorf("pane %s took the text but the agent did not report accepting it after %d submits; the text is waiting in its prompt", target, len(submitWaits))
}

// acceptedPrompt waits for the agent in pane to report any new hook event. A
// submitted prompt writes one; an Enter swallowed by the input buffer does not.
func acceptedPrompt(pane string, before models.SessionState) bool {
	deadline := time.Now().Add(submitConfirm)
	for {
		if current, ok := agentState(pane); ok && current != before {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(submitPoll)
	}
}

func agentState(pane string) (models.SessionState, bool) {
	if pane == "" {
		return models.SessionState{}, false
	}
	current, ok := state.LoadStatesByPaneID(state.DefaultStatesDir())[pane]
	return current, ok
}

// resolvePaneID turns any tmux target into the %-prefixed pane ID that hook
// state files are keyed by.
func resolvePaneID(target string) string {
	if strings.HasPrefix(target, "%") {
		return target
	}
	return strings.TrimSpace(RunTmux("display-message", "-p", "-t", target, "#{pane_id}"))
}
