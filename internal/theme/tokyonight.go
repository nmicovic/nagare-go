package theme

import "github.com/charmbracelet/lipgloss"

func init() {
	Register("tokyonight", &Theme{
		Name: "tokyonight",
		Colors: Colors{
			Background: lipgloss.AdaptiveColor{Dark: "#1a1b26", Light: "#d5d6db"},
			Foreground: lipgloss.AdaptiveColor{Dark: "#c0caf5", Light: "#343b58"},
			Primary:    lipgloss.AdaptiveColor{Dark: "#7aa2f7", Light: "#34548a"},
			Secondary:  lipgloss.AdaptiveColor{Dark: "#bb9af7", Light: "#5a4a78"},
			Accent:     lipgloss.AdaptiveColor{Dark: "#7dcfff", Light: "#0f4b6e"},
			Muted:      lipgloss.AdaptiveColor{Dark: "#565f89", Light: "#9699a3"},
			Border:     lipgloss.AdaptiveColor{Dark: "#3b4261", Light: "#a9b1d6"},
			Error:      lipgloss.AdaptiveColor{Dark: "#db4b4b", Light: "#8c4351"},
			Warning:    lipgloss.AdaptiveColor{Dark: "#e0af68", Light: "#8f5e15"},
			Success:    lipgloss.AdaptiveColor{Dark: "#00D26A", Light: "#33635c"},
		},
	})
}
