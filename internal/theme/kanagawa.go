package theme

import "github.com/charmbracelet/lipgloss"

func init() {
	Register("kanagawa", &Theme{
		Name: "kanagawa",
		Colors: Colors{
			Background: lipgloss.AdaptiveColor{Dark: "#1F1F28", Light: "#F2E9DE"},
			Foreground: lipgloss.AdaptiveColor{Dark: "#DCD7BA", Light: "#54433A"},
			Primary:    lipgloss.AdaptiveColor{Dark: "#7E9CD8", Light: "#2D4F67"},
			Secondary:  lipgloss.AdaptiveColor{Dark: "#76946A", Light: "#76946A"},
			Accent:     lipgloss.AdaptiveColor{Dark: "#D27E99", Light: "#D27E99"},
			Muted:      lipgloss.AdaptiveColor{Dark: "#727169", Light: "#9E9389"},
			Border:     lipgloss.AdaptiveColor{Dark: "#727169", Light: "#9E9389"},
			Error:      lipgloss.AdaptiveColor{Dark: "#E82424", Light: "#E82424"},
			Warning:    lipgloss.AdaptiveColor{Dark: "#D7A657", Light: "#D7A657"},
			Success:    lipgloss.AdaptiveColor{Dark: "#98BB6C", Light: "#98BB6C"},
		},
	})
}
