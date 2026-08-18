package popup

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// sizedPopup builds a popup at a terminal size, with a preview long enough to
// fill whatever room the layout gives it.
func sizedPopup(t *testing.T, w, h int, eventType string) Model {
	t.Helper()
	m := New("cosmic-platform-backend", eventType,
		"Claude needs permission to run a shell command", 10, 0)
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = next.(Model)
	m.preview = strings.Repeat("a preview line of captured output\n", 40)
	return m
}

var popupSizes = [][2]int{{160, 20}, {120, 16}, {100, 16}, {80, 14}, {70, 12}, {60, 12}, {50, 10}, {40, 8}}

// TestPopupFillsItsFrameExactly asserts on the *unclamped* frame. View() applies
// MaxWidth/MaxHeight, so measuring after it would hide the very overflow this
// guards: the popup renders inline, and a frame one row too tall scrolls the host
// pane. Worse, the clamp trims from the bottom, so the row it removed was the
// hint bar — the keys the popup exists to offer.
func TestPopupFillsItsFrameExactly(t *testing.T) {
	for _, eventType := range []string{"needs_input", "task_complete"} {
		for _, sz := range popupSizes {
			w, h := sz[0], sz[1]
			t.Run(fmt.Sprintf("%s/%dx%d", eventType, w, h), func(t *testing.T) {
				raw := sizedPopup(t, w, h, eventType).view()

				if got := lipgloss.Height(raw); got != h {
					t.Errorf("frame is %d rows, want %d", got, h)
				}
				for i, row := range strings.Split(raw, "\n") {
					if got := ansi.StringWidth(row); got != w {
						t.Errorf("row %d is %d cells, want %d", i, got, w)
					}
				}
			})
		}
	}
}

// TestPopupHintBarSurvives — the bar is the reason the popup is on screen. It
// must be present and on one row at every size.
func TestPopupHintBarSurvives(t *testing.T) {
	for _, sz := range popupSizes {
		w, h := sz[0], sz[1]
		m := sizedPopup(t, w, h, "needs_input")

		bar := m.renderHintBar(w - 2)
		if got := lipgloss.Height(bar); got != 1 {
			t.Errorf("%dx%d: hint bar is %d rows, want 1: %q", w, h, got, ansi.Strip(bar))
		}
		if got := lipgloss.Width(bar); got > w-2 {
			t.Errorf("%dx%d: hint bar is %d cells, wider than the %d it was given", w, h, got, w-2)
		}
		// And it reached the frame rather than being clipped off the bottom.
		if !strings.Contains(ansi.Strip(m.view()), "Enter Jump") {
			t.Errorf("%dx%d: hint bar missing from the rendered frame", w, h)
		}
	}
}

// TestPopupDropsCountdownBeforeKeys — when the bar cannot fit, the countdown is
// the part worth losing.
func TestPopupDropsCountdownBeforeKeys(t *testing.T) {
	wide := ansi.Strip(sizedPopup(t, 160, 20, "needs_input").renderHintBar(158))
	narrow := ansi.Strip(sizedPopup(t, 50, 10, "needs_input").renderHintBar(48))

	if !strings.Contains(wide, "Auto-closing") {
		t.Error("a wide popup should show the countdown")
	}
	if strings.Contains(narrow, "Auto-closing") {
		t.Error("a narrow popup kept the countdown instead of the keys")
	}
	if !strings.Contains(narrow, "Enter Jump") {
		t.Errorf("a narrow popup dropped the keys: %q", narrow)
	}
}
