package theme

import "github.com/charmbracelet/lipgloss"

func init() {
	Register("onedarkpro", &Theme{
		Name: "onedarkpro",
		Colors: Colors{
			Background: lipgloss.AdaptiveColor{Dark: "#1e222a", Light: "#f5f6f8"},
			Foreground: lipgloss.AdaptiveColor{Dark: "#abb2bf", Light: "#2b303b"},
			Primary:    lipgloss.AdaptiveColor{Dark: "#61afef", Light: "#528bff"},
			Secondary:  lipgloss.AdaptiveColor{Dark: "#56b6c2", Light: "#61afef"},
			Accent:     lipgloss.AdaptiveColor{Dark: "#e06c75", Light: "#d85462"},
			Muted:      lipgloss.AdaptiveColor{Dark: "#5c6370", Light: "#6a717d"},
			Border:     lipgloss.AdaptiveColor{Dark: "#5c6370", Light: "#6a717d"},
			Error:      lipgloss.AdaptiveColor{Dark: "#e06c75", Light: "#e06c75"},
			Warning:    lipgloss.AdaptiveColor{Dark: "#e5c07b", Light: "#d19a66"},
			Success:    lipgloss.AdaptiveColor{Dark: "#98c379", Light: "#4fa66d"},
		},
	})
}
