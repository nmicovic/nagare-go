package theme

import "github.com/charmbracelet/lipgloss"

func init() {
	Register("one-dark", &Theme{
		Name: "one-dark",
		Colors: Colors{
			Background: lipgloss.AdaptiveColor{Dark: "#282c34", Light: "#fafafa"},
			Foreground: lipgloss.AdaptiveColor{Dark: "#abb2bf", Light: "#383a42"},
			Primary:    lipgloss.AdaptiveColor{Dark: "#61afef", Light: "#4078f2"},
			Secondary:  lipgloss.AdaptiveColor{Dark: "#56b6c2", Light: "#0184bc"},
			Accent:     lipgloss.AdaptiveColor{Dark: "#c678dd", Light: "#a626a4"},
			Muted:      lipgloss.AdaptiveColor{Dark: "#5c6370", Light: "#a0a1a7"},
			Border:     lipgloss.AdaptiveColor{Dark: "#5c6370", Light: "#a0a1a7"},
			Error:      lipgloss.AdaptiveColor{Dark: "#e06c75", Light: "#e45649"},
			Warning:    lipgloss.AdaptiveColor{Dark: "#e5c07b", Light: "#c18401"},
			Success:    lipgloss.AdaptiveColor{Dark: "#98c379", Light: "#50a14f"},
		},
	})
}
