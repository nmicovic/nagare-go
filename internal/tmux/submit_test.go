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
	waits, confirm, poll := submitWaits, submitConfirm, submitPoll
	t.Cleanup(func() { submitWaits, submitConfirm, submitPoll = waits, confirm, poll })
	submitWaits = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	submitConfirm = 40 * time.Millisecond
	submitPoll = 5 * time.Millisecond

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
	body := `{"state":"` + status + `","session_id":"s","cwd":"/repo","pane_id":"%42","event":"` + event + `","timestamp":"2026-09-15T10:00:00Z"}`
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
	if err := SubmitPrompt("%42", "check_messages()"); err != nil {
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
	if err := SubmitPrompt("board:0.1", "URGENT: message 1234 requires attention."); err != nil {
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
	err := SubmitPrompt("%42", "URGENT: message 1234 requires attention.")
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
	if err := SubmitPrompt("%42", "hello"); err != nil {
		t.Fatalf("SubmitPrompt() = %v", err)
	}
	if got := enters(log()); got != 1 {
		t.Fatalf("enters = %d, want 1", got)
	}
}

func TestSubmitPromptRejectsAnEmptyTarget(t *testing.T) {
	if err := SubmitPrompt("  ", "text"); err == nil {
		t.Fatal("SubmitPrompt() = nil for an empty target")
	}
}
