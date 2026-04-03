package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

	title := lipgloss.NewStyle().
		Foreground(c.Primary).
		Bold(true).
		Render("Keyboard Shortcuts")

	section := func(name string) string {
		return "\n" + lipgloss.NewStyle().Foreground(c.Accent).Bold(true).Render(name) + "\n"
	}

	key := func(k string) string {
		return lipgloss.NewStyle().Foreground(c.Primary).Width(14).Render(k)
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
		line("Ctrl+x", "Kill entire tmux session"),
		line("F2", "Rename session"),
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
