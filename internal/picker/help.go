package picker

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/theme"
)

// hint is one key and what it does right now.
type hint struct{ key, label string }

// hintsFor returns the keys actually available in the picker's current state.
//
// The bar used to list all twenty bindings unconditionally, which wrapped onto a
// second line and cost a row of the session list. Worse, it advertised keys that
// would do nothing: ^y on an idle session, ^x on a saved one. A footer that
// changes with context is both shorter and truthful, and the full set stays one
// F1 away — the progressive-disclosure convention zellij uses for its modes.
func hintsFor(m Model) []hint {
	// An open overlay or an input mode owns the keyboard, so it owns the footer.
	switch {
	case m.showHelp:
		return []hint{{"F1 / Esc", "Close"}}
	case m.showThemePick:
		return []hint{{"↑/↓", "Preview"}, {"Enter", "Keep"}, {"Esc", "Cancel"}}
	case m.confirmMode:
		return []hint{{"y", "Remove worktree"}, {"n / Esc", "Keep it"}}
	case m.promptMode:
		return []hint{{"Enter", "Send"}, {"Esc", "Cancel"}}
	case m.renameMode:
		return []hint{{"Enter", "Rename"}, {"Esc", "Cancel"}}
	case m.worktreeMode:
		return []hint{{"Enter", "Create worktree"}, {"Esc", "Cancel"}}
	}

	hints := []hint{}
	s, ok := m.selectedSession()
	saved := ok && s.Status == models.StatusSaved

	if ok {
		if saved {
			hints = append(hints, hint{"Enter", "Load"})
		} else {
			hints = append(hints, hint{"Enter", "Jump"})
		}
	}
	hints = append(hints, hint{"↑/↓", "Navigate"})

	// Order below is drop order: helpBar trims from the end to fit one line, so
	// the most useful action for the current selection has to come first.
	//
	// The queue leads. If anything is waiting on the user, getting to it is the
	// most useful thing the picker can offer, and the count is worth showing —
	// "3 waiting" is a different situation from "1 waiting".
	if n := waitingCount(m.filtered); n > 0 {
		label := "Next waiting"
		if n > 1 {
			label = fmt.Sprintf("Next of %d waiting", n)
		}
		hints = append(hints, hint{"F4", label})
	}
	// Then approval, because a waiting agent is blocked until it arrives.
	if ok && approvable(s.Status) {
		hints = append(hints, hint{"^y", "Allow"}, hint{"^a", "Always"})
	}
	if ok && !saved {
		hints = append(hints, hint{"^l", "Prompt"})
		// Ctrl+x offers worktree removal on a worktree pane, and says so rather
		// than leaving the extra prompt as a surprise.
		if s.Details.Worktree != "" {
			hints = append(hints, hint{"^x", "Kill + remove"})
		} else {
			hints = append(hints, hint{"^x", "Kill"})
		}
	}
	hints = append(hints, hint{"Tab", "View"})
	if ok && !saved {
		hints = append(hints, hint{"^w", "Unload"})
	}
	if ok {
		hints = append(hints, hint{"F2", "Rename"}, hint{"F3", "Worktree"})
	}
	hints = append(hints, hint{"^n", "New"}, hint{"^f", "Star"}, hint{"^o", "Sort"})

	return hints
}

// helpBar renders the bottom hint bar. It is always exactly one line: hints are
// dropped from the end until they fit, and the trailing "F1 More · Esc Quit" is
// reserved space so there is always a visible way to reach the rest and to get
// out.
func helpBar(m Model, width int) string {
	c := theme.Current().Colors
	keyStyle := lipgloss.NewStyle().Foreground(c.Accent).Bold(true)
	sep := lipgloss.NewStyle().Foreground(c.Muted).Render(" │ ")
	sepWidth := lipgloss.Width(sep)

	render := func(h hint) string {
		return keyStyle.Render(h.key) + " " + mutedStyle().Render(h.label)
	}

	tail := []hint{{"F1", "More"}, {"Esc", "Quit"}}
	if m.showHelp || m.showThemePick || m.confirmMode || m.promptMode ||
		m.renameMode || m.worktreeMode {
		// A mode's own footer already names its exit; "More"/"Quit" would be
		// wrong there, since F1 and Esc mean something else.
		tail = nil
	}

	var tailParts []string
	tailWidth := 0
	for _, h := range tail {
		part := render(h)
		tailParts = append(tailParts, part)
		tailWidth += lipgloss.Width(part) + sepWidth
	}

	// Available content width, inside this style's horizontal padding.
	budget := width - 2 - tailWidth

	var parts []string
	used := 0
	for _, h := range hintsFor(m) {
		part := render(h)
		cost := lipgloss.Width(part)
		if len(parts) > 0 {
			cost += sepWidth
		}
		if used+cost > budget {
			break
		}
		parts = append(parts, part)
		used += cost
	}
	parts = append(parts, tailParts...)

	return lipgloss.NewStyle().
		Foreground(c.Muted).
		Background(canvasBg()).
		Width(width).
		MaxHeight(1).
		Padding(0, 1).
		Render(onPlane(strings.Join(parts, sep), canvasBg()))
}

// helpOverlay renders the full help screen shown on F1.
func helpOverlay(width, height int) string {
	c := theme.Current().Colors

	// Inner width after the outer dialog's border (2 cols) and padding
	// (2*4 horizontal). Section fill lines match that width.
	innerWidth := width*2/3 - 10

	title := sectionHeader("Keyboard Shortcuts", innerWidth)

	section := func(name string) string {
		return "\n" + sectionHeader(name, innerWidth) + "\n"
	}

	key := func(k string) string {
		return lipgloss.NewStyle().Foreground(c.Accent).Width(14).Render(k)
	}

	desc := func(d string) string {
		return lipgloss.NewStyle().Foreground(c.Foreground).Render(d)
	}

	line := func(k, d string) string {
		return fmt.Sprintf("  %s %s", key(k), desc(d))
	}

	content := strings.Join([]string{
		title,
		section("Navigation"),
		line("↑ / ↓", "Move cursor up/down"),
		line("← / →", "Move cursor left/right (grid view)"),
		line("Enter", "Jump to selected session"),
		line("Esc", "Quit nagare"),
		section("Views"),
		line("Tab", "Toggle list / grid view"),
		line("Ctrl+t", "Cycle color theme"),
		line("F1", "Toggle this help screen"),
		section("Actions"),
		line("F4", "Jump to the next session waiting on you"),
		line("Ctrl+y", "Approve permission (waiting sessions)"),
		line("Ctrl+a", "Approve always (waiting sessions)"),
		line("Ctrl+f", "Toggle star/favorite"),
		line("Ctrl+o", "Cycle sort mode (status/name/agent)"),
		line("Enter", "Jump to session / Load saved session"),
		line("Ctrl+s", "Toggle saved (unloaded) sessions"),
		line("Ctrl+w", "Unload agent (kill pane)"),
		line("Ctrl+x", "Kill this pane's window, or the session if it is the only one"),
		line("F2", "Rename session"),
		line("F3", "New git worktree for this repo"),
		line("Ctrl+n", "Create new session"),
		line("Ctrl+r", "Quick prototype"),
		line("Ctrl+l", "Send inline prompt to session"),
		line("Ctrl+g", "Send prompt via $EDITOR"),
		line("Ctrl+e", "Edit config file"),
		section("Search"),
		line("Type", "Fuzzy search by session name or path"),
		line("", "Best match is auto-selected"),
		section("Mouse"),
		line("Click", "Select a session; click it again to jump"),
		line("Wheel", "Move the selection"),
		line("Click away", "Close this screen or the theme picker"),
		line("", "Disable with picker.mouse = false in the config"),
		"",
		mutedStyle().Render("  Press F1 or Esc to close"),
	}, "\n")

	return dialogStyle().
		Width(width*2/3).
		Height(height*2/3).
		Padding(2, 4).
		Render(onPlane(content, c.Overlay))
}
