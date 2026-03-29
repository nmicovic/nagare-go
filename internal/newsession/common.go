package newsession

import (
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/nemke/nagare-go/internal/theme"
)

var agents = []string{"claude", "opencode", "gemini"}

func cycleAgent(available []string, current string, delta int) string {
	idx := slices.Index(available, current)
	if idx < 0 {
		return available[0]
	}
	idx = (idx + delta + len(available)) % len(available)
	return available[idx]
}

// quickAgents is the subset for quick prototype (no gemini).
var quickAgents = []string{"claude", "opencode"}

func renderAgentPicker(available []string, selected string, focused bool) string {
	c := theme.Current().Colors

	var opts []string
	for _, a := range available {
		if a == selected {
			opts = append(opts, "(●) "+a)
		} else {
			opts = append(opts, "( ) "+a)
		}
	}
	str := "  Agent: " + strings.Join(opts, "  ")
	if focused {
		str = lipgloss.NewStyle().
			Background(c.Primary).
			Foreground(c.Background).
			Render(str)
	}
	return str
}

func renderError(err error) string {
	if err == nil {
		return ""
	}
	c := theme.Current().Colors
	return lipgloss.NewStyle().
		Foreground(c.Error).
		Render("  Error: "+err.Error()) + "\n"
}

func renderHint(text string) string {
	return lipgloss.NewStyle().
		Foreground(theme.Current().Colors.Muted).
		Render("  " + text)
}
