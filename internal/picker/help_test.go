package picker

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nemke/nagare-go/internal/models"
)

// lipglossHeight is a thin alias so the intent reads clearly in assertions.
func lipglossHeight(s string) int { return lipgloss.Height(s) }

func footerText(m Model, width int) string {
	return ansi.Strip(helpBar(m, width))
}

func footerKeys(m Model) []string {
	keys := make([]string, 0, 8)
	for _, h := range hintsFor(m) {
		keys = append(keys, h.key)
	}
	return keys
}

func hasKey(m Model, key string) bool {
	for _, k := range footerKeys(m) {
		if k == key {
			return true
		}
	}
	return false
}

func footerModel(t *testing.T, sessions []models.Session) Model {
	t.Helper()
	return driveModel(t, NewForTest(),
		tea.WindowSizeMsg{Width: 120, Height: 30},
		SessionsUpdatedMsg(sessions),
	)
}

// TestFooterIsAlwaysOneLine is the reason the bar was rewritten: the old one
// wrapped to two lines and stole a row from the session list. Narrow terminals
// are the case that used to break.
func TestFooterIsAlwaysOneLine(t *testing.T) {
	m := footerModel(t, mouseSessions())

	for _, width := range []int{20, 40, 60, 80, 120, 200} {
		bar := helpBar(m, width)
		if got := lipglossHeight(bar); got != 1 {
			t.Errorf("width %d: footer is %d lines, want 1\n%s", width, got, ansi.Strip(bar))
		}
		if got := ansi.StringWidth(strings.Split(bar, "\n")[0]); got != width {
			t.Errorf("width %d: footer renders %d cells wide", width, got)
		}
	}
}

// TestFooterAlwaysOffersAWayOutAndIn — however little room there is, the user
// must be able to see how to reach the rest of the keymap and how to quit.
func TestFooterAlwaysOffersAWayOutAndIn(t *testing.T) {
	m := footerModel(t, mouseSessions())

	for _, width := range []int{20, 30, 50, 120} {
		text := footerText(m, width)
		if !strings.Contains(text, "F1") {
			t.Errorf("width %d: footer does not mention F1\n%s", width, text)
		}
		if !strings.Contains(text, "Esc") {
			t.Errorf("width %d: footer does not mention Esc\n%s", width, text)
		}
	}
}

// TestFooterHidesKeysThatWouldDoNothing — advertising ^y on an idle session is
// what made the old bar untrustworthy.
func TestFooterHidesKeysThatWouldDoNothing(t *testing.T) {
	idle := footerModel(t, []models.Session{
		{Name: "idle", SessionName: "idle", Path: "/tmp/idle",
			Status: models.StatusIdle, AgentType: models.AgentClaude},
	})
	if hasKey(idle, "^y") {
		t.Error("idle session offers ^y Allow, which would do nothing")
	}

	waiting := footerModel(t, []models.Session{
		{Name: "waiting", SessionName: "waiting", Path: "/tmp/waiting",
			Status: models.StatusWaitingInput, AgentType: models.AgentClaude},
	})
	if !hasKey(waiting, "^y") {
		t.Error("a session waiting for input should offer ^y Allow")
	}
	if !hasKey(waiting, "^a") {
		t.Error("a session waiting for input should offer ^a Always")
	}
}

// TestFooterAdaptsToSavedSessions — a saved session is loaded, not jumped to,
// and cannot be unloaded or killed.
func TestFooterAdaptsToSavedSessions(t *testing.T) {
	// Saved sessions are filtered out until ^s reveals them, so the toggle has
	// to be on before one can be the selection at all.
	m := NewForTest()
	m.showSaved = true
	m = driveModel(t, m,
		tea.WindowSizeMsg{Width: 120, Height: 30},
		SessionsUpdatedMsg([]models.Session{
			{Name: "saved", Path: "/tmp/saved", Status: models.StatusSaved, AgentType: models.AgentClaude},
		}),
	)
	if _, ok := m.selectedSession(); !ok {
		t.Fatal("no saved session selected; the filter did not include it")
	}

	var enterLabel string
	for _, h := range hintsFor(m) {
		if h.key == "Enter" {
			enterLabel = h.label
		}
	}
	if enterLabel != "Load" {
		t.Errorf("Enter on a saved session is labelled %q, want \"Load\"", enterLabel)
	}
	if hasKey(m, "^w") {
		t.Error("a saved session offers ^w Unload, but there is no pane to unload")
	}
	if hasKey(m, "^x") {
		t.Error("a saved session offers ^x Kill, but there is nothing to kill")
	}
}

// TestFooterNamesWorktreeRemoval — Ctrl+x behaves differently on a worktree
// pane, so it should say so instead of springing an extra prompt.
func TestFooterNamesWorktreeRemoval(t *testing.T) {
	plain := footerModel(t, []models.Session{
		{Name: "plain", SessionName: "plain", Path: "/tmp/plain",
			Status: models.StatusIdle, AgentType: models.AgentClaude},
	})
	worktree := footerModel(t, []models.Session{
		{Name: "repo/wt", SessionName: "repo", Path: "/tmp/repo/wt",
			Status: models.StatusIdle, AgentType: models.AgentClaude,
			Details: models.SessionDetails{RepoName: "repo", Worktree: "wt"}},
	})

	labelFor := func(m Model, key string) string {
		for _, h := range hintsFor(m) {
			if h.key == key {
				return h.label
			}
		}
		return ""
	}

	if got := labelFor(plain, "^x"); got != "Kill" {
		t.Errorf("^x on a plain pane is labelled %q, want \"Kill\"", got)
	}
	if got := labelFor(worktree, "^x"); !strings.Contains(got, "remove") {
		t.Errorf("^x on a worktree pane is labelled %q, want it to mention removal", got)
	}
}

// TestFooterFollowsTheActiveMode — whichever overlay or input mode owns the
// keyboard owns the footer, and its keys replace the normal set entirely rather
// than being appended to it.
func TestFooterFollowsTheActiveMode(t *testing.T) {
	base := footerModel(t, mouseSessions())

	cases := []struct {
		name  string
		setup func(Model) Model
		want  []string
		gone  string
	}{
		{"help", func(m Model) Model { m.showHelp = true; return m }, []string{"Close"}, "Navigate"},
		{"theme picker", func(m Model) Model { m.showThemePick = true; return m }, []string{"Preview", "Cancel"}, "Navigate"},
		{"confirm", func(m Model) Model { m.confirmMode = true; return m }, []string{"Remove worktree", "Keep it"}, "Navigate"},
		{"prompt", func(m Model) Model { m.promptMode = true; return m }, []string{"Send", "Cancel"}, "Navigate"},
		{"rename", func(m Model) Model { m.renameMode = true; return m }, []string{"Rename", "Cancel"}, "Navigate"},
		{"new worktree", func(m Model) Model { m.worktreeMode = true; return m }, []string{"Create worktree"}, "Navigate"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := footerText(tc.setup(base), 120)
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("footer lacks %q\ngot: %s", want, text)
				}
			}
			if strings.Contains(text, tc.gone) {
				t.Errorf("footer still shows the normal-mode hint %q\ngot: %s", tc.gone, text)
			}
		})
	}
}

// TestFooterModesDoNotClaimF1OrEsc — inside a mode those keys mean something
// specific, so the generic "F1 More / Esc Quit" tail must not be appended and
// contradict the mode's own labels.
func TestFooterModesDoNotClaimF1OrEsc(t *testing.T) {
	m := footerModel(t, mouseSessions())
	m.confirmMode = true

	if text := footerText(m, 120); strings.Contains(text, "More") || strings.Contains(text, "Quit") {
		t.Errorf("confirm-mode footer advertises More/Quit\ngot: %s", text)
	}
}

// TestFooterHandlesAnEmptyList — with nothing selected there is no session to
// act on, and the footer must not offer session actions or panic.
func TestFooterHandlesAnEmptyList(t *testing.T) {
	m := footerModel(t, nil)

	for _, key := range []string{"Enter", "^y", "^w", "^x", "F2", "F3"} {
		if hasKey(m, key) {
			t.Errorf("empty list offers %s with nothing selected", key)
		}
	}
	if got := lipglossHeight(helpBar(m, 120)); got != 1 {
		t.Errorf("footer with an empty list is %d lines, want 1", got)
	}
}
