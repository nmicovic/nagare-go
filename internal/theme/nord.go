package theme

import "github.com/charmbracelet/lipgloss"

func init() {
	Register("nord", &Theme{
		Name: "nord",
		Colors: Colors{
			Background: lipgloss.AdaptiveColor{Dark: "#2e3440", Light: "#eceff4"},
			Foreground: lipgloss.AdaptiveColor{Dark: "#d8dee9", Light: "#2e3440"},
			Primary:    lipgloss.AdaptiveColor{Dark: "#88c0d0", Light: "#5e81ac"},
			Secondary:  lipgloss.AdaptiveColor{Dark: "#b48ead", Light: "#8a4f8a"},
			Accent:     lipgloss.AdaptiveColor{Dark: "#81a1c1", Light: "#4c7a9e"},
			Muted:      lipgloss.AdaptiveColor{Dark: "#4c566a", Light: "#9ea7b0"},
			Border:     lipgloss.AdaptiveColor{Dark: "#3b4252", Light: "#d8dee9"},
			Error:      lipgloss.AdaptiveColor{Dark: "#bf616a", Light: "#bf616a"},
			Warning:    lipgloss.AdaptiveColor{Dark: "#ebcb8b", Light: "#d08770"},
			Success:    lipgloss.AdaptiveColor{Dark: "#a3be8c", Light: "#a3be8c"},
		},
	})
}
