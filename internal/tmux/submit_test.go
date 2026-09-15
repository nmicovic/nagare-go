package tmux

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// submitHarness installs a tmux stub that records every call and lets the test
// decide after how many Enters the agent reports accepting the prompt.
func submitHarness(t *testing.T, confirmAfter int, reportsState bool) (log func() []string, home string) {
	t.Helper()
	binDir := t.TempDir()
	home = t.TempDir()
	states := filepath.Join(home, ".local", "share", "nagare", "states")
	if err := os.MkdirAll(states, 0o755); err != nil {
		t.Fatal(err)
	}
	if reportsState {
		writeAgentState(t, states, "Stop", "idle")
	}
	calls := filepath.Join(binDir, "calls.log")
	counter := filepath.Join(binDir, "enters")
	script := `#!/bin/sh
echo "$*" >> "` + calls + `"
case "$1" in
  display-message) printf '%%42\n'; exit 0 ;;
  send-keys)
    case "$*" in
      *" Enter"*)
        count=$(cat "` + counter + `" 2>/dev/null || echo 0)
        count=$((count + 1))
        echo "$count" > "` + counter + `"
        if [ -n "$CONFIRM_AFTER" ] && [ "$count" -ge "$CONFIRM_AFTER" ]; then
          printf '{"state":"working","session_id":"s","cwd":"/repo","pane_id":"%%42","event":"UserPromptSubmit","timestamp":"2026-09-15T10:00:01Z"}' > "` + states + `/s.json"
        fi
        ;;
    esac
    exit 0 ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", home)
	if confirmAfter > 0 {
		t.Setenv("CONFIRM_AFTER", strconv.Itoa(confirmAfter))
	} else {
		t.Setenv("CONFIRM_AFTER", "")
	}

	// Real waits would make every case a multi-second test; the sequencing they
	// guard is what is under test, not their length.
	waits, confirm, poll, ready := submitWaits, submitConfirm, submitPoll, submitReady
	t.Cleanup(func() {
		submitWaits, submitConfirm, submitPoll, submitReady = waits, confirm, poll, ready
	})
	submitWaits = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	submitConfirm = 40 * time.Millisecond
	submitPoll = 5 * time.Millisecond
	submitReady = 60 * time.Millisecond

	return func() []string {
		data, err := os.ReadFile(calls)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	}, home
}

func writeAgentState(t *testing.T, dir, event, status string) {
	t.Helper()
	writeState(t, dir, event, status, time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC))
}

func writeState(t *testing.T, dir, event, status string, at time.Time) {
	t.Helper()
	body := `{"state":"` + status + `","session_id":"s","cwd":"/repo","pane_id":"%42","event":"` + event +
		`","timestamp":"` + at.UTC().Format(time.RFC3339) + `"}`
	if err := os.WriteFile(filepath.Join(dir, "s.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func enters(calls []string) int {
	count := 0
	for _, call := range calls {
		if strings.HasPrefix(call, "send-keys") && strings.HasSuffix(call, " Enter") {
			count++
		}
	}
	return count
}

func TestSubmitPromptStopsAtTheEnterTheAgentAccepts(t *testing.T) {
	log, _ := submitHarness(t, 1, true)
	if err := SubmitPrompt(Submit{Target: "%42", Text: "check_messages()", Reports: true}); err != nil {
		t.Fatalf("SubmitPrompt() = %v", err)
	}
	calls := log()
	if got := enters(calls); got != 1 {
		t.Fatalf("enters = %d, want 1:\n%s", got, strings.Join(calls, "\n"))
	}
	if !strings.Contains(calls[0], "-l check_messages()") {
		t.Fatalf("text was not typed literally first:\n%s", strings.Join(calls, "\n"))
	}
}

func TestSubmitPromptResendsEnterUntilTheAgentTakesThePrompt(t *testing.T) {
	// The reported bug: the agent's input debounce swallowed the first Enter and
	// the notice sat in its prompt while nagare called the ticket delivered.
	log, _ := submitHarness(t, 2, true)
	if err := SubmitPrompt(Submit{Target: "board:0.1", Text: "URGENT: message 1234 requires attention.", Reports: true}); err != nil {
		t.Fatalf("SubmitPrompt() = %v", err)
	}
	calls := log()
	if got := enters(calls); got != 2 {
		t.Fatalf("enters = %d, want 2 (a swallowed Enter must be resent):\n%s", got, strings.Join(calls, "\n"))
	}
	var resolved bool
	for _, call := range calls {
		if strings.HasPrefix(call, "display-message") {
			resolved = true
		}
	}
	if !resolved {
		t.Fatalf("a pane target was not resolved to its pane ID:\n%s", strings.Join(calls, "\n"))
	}
}

func TestSubmitPromptReportsAPromptTheAgentNeverAccepted(t *testing.T) {
	log, _ := submitHarness(t, 0, true)
	err := SubmitPrompt(Submit{Target: "%42", Text: "URGENT: message 1234 requires attention.", Reports: true})
	if err == nil {
		t.Fatal("SubmitPrompt() = nil for a prompt that was never accepted")
	}
	for _, want := range []string{"%42", "waiting in its prompt"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if got := enters(log()); got != len(submitWaits) {
		t.Fatalf("enters = %d, want %d", got, len(submitWaits))
	}
}

func TestSubmitPromptDoesNotGuessForAnAgentThatReportsNoState(t *testing.T) {
	// Crush installs no status reporting, so there is nothing to confirm
	// against and a resent Enter would be a guess at another agent's input.
	log, _ := submitHarness(t, 0, false)
	if err := SubmitPrompt(Submit{Target: "%42", Text: "hello", Reports: false}); err != nil {
		t.Fatalf("SubmitPrompt() = %v", err)
	}
	if got := enters(log()); got != 1 {
		t.Fatalf("enters = %d, want 1", got)
	}
}

func TestSubmitPromptRejectsAnEmptyTarget(t *testing.T) {
	if err := SubmitPrompt(Submit{Target: "  ", Text: "text", Reports: true}); err == nil {
		t.Fatal("SubmitPrompt() = nil for an empty target")
	}
}

func TestSubmitPromptTypesNothingAtAnAgentThatIsNotListening(t *testing.T) {
	// Claude Code opens an untrusted directory — which every managed worktree
	// is — with a trust dialog, and waits there indefinitely without running a
	// single hook. Text typed at that dialog is discarded and the Enter after
	// it answers the highlighted "No, exit", killing the agent just launched
	// for the ticket. So an agent that has not reported itself is not typed at.
	log, _ := submitHarness(t, 1, false)
	err := SubmitPrompt(Submit{Target: "%42", Text: "you have been assigned a ticket", Reports: true})
	if err == nil {
		t.Fatal("SubmitPrompt() = nil for an agent that never started listening")
	}
	if !strings.Contains(err.Error(), "trust the folder") {
		t.Errorf("error does not name the likely cause: %v", err)
	}
	if calls := log(); len(calls) > 0 {
		for _, call := range calls {
			if strings.HasPrefix(call, "send-keys") {
				t.Fatalf("typed at an agent that was not listening:\n%s", strings.Join(calls, "\n"))
			}
		}
	}
}

func TestSubmitPromptWaitsForAStartingAgentThenSubmits(t *testing.T) {
	log, home := submitHarness(t, 1, false)
	states := filepath.Join(home, ".local", "share", "nagare", "states")
	go func() {
		// The user answers the trust prompt; the agent starts and reports.
		time.Sleep(15 * time.Millisecond)
		writeState(t, states, "SessionStart", "idle", time.Now())
	}()
	submitReady = time.Second
	if err := SubmitPrompt(Submit{Target: "%42", Text: "ticket", Reports: true}); err != nil {
		t.Fatalf("SubmitPrompt() = %v", err)
	}
	if got := enters(log()); got != 1 {
		t.Fatalf("enters = %d, want 1 once the agent was listening", got)
	}
}

func TestSubmitPromptIgnoresAPreviousOccupantsState(t *testing.T) {
	// A pane keeps the state files of every agent that has run in it, so a
	// launch must not read the last occupant's as the new agent's readiness.
	log, home := submitHarness(t, 1, false)
	states := filepath.Join(home, ".local", "share", "nagare", "states")
	writeState(t, states, "Stop", "idle", time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC))
	err := SubmitPrompt(Submit{
		Target:    "%42",
		Text:      "ticket",
		Reports:   true,
		NotBefore: time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("SubmitPrompt() = nil while only the previous occupant had reported")
	}
	for _, call := range log() {
		if strings.HasPrefix(call, "send-keys") {
			t.Fatal("typed into a pane whose new agent had not reported")
		}
	}
}
