package picker

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/nemke/nagare-go/internal/theme"
)

// helpBar renders the bottom hint bar showing available keybindings.
func helpBar(width int) string {
	c := theme.Current().Colors
	key := lipgloss.NewStyle().Foreground(c.Accent).Bold(true)
	sep := lipgloss.NewStyle().Foreground(c.Muted).Render(" │ ")

	pairs := []struct{ k, v string }{
		{"F1", "Help"},
		{"F2", "Rename"},
		{"F3", "Wtree"},
		{"Enter", "Jump"},
		{"↑/↓", "Navigate"},
		{"Tab", "View"},
		{"^n", "New"},
		{"^r", "Proto"},
		{"^y", "Allow"},
		{"^a", "Always"},
		{"^f", "Star"},
		{"^o", "Sort"},
		{"^l", "Prompt"},
		{"^g", "Editor"},
		{"^t", "Theme"},
		{"^e", "Config"},
		{"^s", "Saved"},
		{"^w", "Unload"},
		{"^x", "Kill"},
		{"Esc", "Quit"},
	}

	var parts []string
	for _, p := range pairs {
		parts = append(parts, key.Render(p.k)+" "+mutedStyle().Render(p.v))
	}

	bar := strings.Join(parts, sep)

	return lipgloss.NewStyle().
		Foreground(c.Muted).
		Background(c.Background).
		Width(width).
		Padding(0, 1).
		Render(bar)
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
		"",
		mutedStyle().Render("  Press F1 or Esc to close"),
	}, "\n")

	return dialogStyle().
		Width(width*2/3).
		Height(height*2/3).
		Padding(2, 4).
		Render(content)
}
