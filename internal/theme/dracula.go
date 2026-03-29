package theme

import "github.com/charmbracelet/lipgloss"

func init() {
	Register("dracula", &Theme{
		Name: "dracula",
		Colors: Colors{
			Background: lipgloss.AdaptiveColor{Dark: "#282a36", Light: "#f8f8f2"},
			Foreground: lipgloss.AdaptiveColor{Dark: "#f8f8f2", Light: "#282a36"},
			Primary:    lipgloss.AdaptiveColor{Dark: "#bd93f9", Light: "#7e57c2"},
			Secondary:  lipgloss.AdaptiveColor{Dark: "#ff79c6", Light: "#d81b60"},
			Accent:     lipgloss.AdaptiveColor{Dark: "#8be9fd", Light: "#0097a7"},
			Muted:      lipgloss.AdaptiveColor{Dark: "#6272a4", Light: "#9e9e9e"},
			Border:     lipgloss.AdaptiveColor{Dark: "#44475a", Light: "#bdbdbd"},
			Error:      lipgloss.AdaptiveColor{Dark: "#ff5555", Light: "#e53935"},
			Warning:    lipgloss.AdaptiveColor{Dark: "#f1fa8c", Light: "#f9a825"},
			Success:    lipgloss.AdaptiveColor{Dark: "#50fa7b", Light: "#43a047"},
		},
	})
}
