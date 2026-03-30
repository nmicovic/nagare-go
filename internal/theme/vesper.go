package theme

import "github.com/charmbracelet/lipgloss"

func init() {
	Register("vesper", &Theme{
		Name: "vesper",
		Colors: Colors{
			Background: lipgloss.AdaptiveColor{Dark: "#101010", Light: "#F0F0F0"},
			Foreground: lipgloss.AdaptiveColor{Dark: "#FFF", Light: "#101010"},
			Primary:    lipgloss.AdaptiveColor{Dark: "#FFC799", Light: "#FFC799"},
			Secondary:  lipgloss.AdaptiveColor{Dark: "#FFC799", Light: "#FFC799"},
			Accent:     lipgloss.AdaptiveColor{Dark: "#FF8080", Light: "#B30000"},
			Muted:      lipgloss.AdaptiveColor{Dark: "#8b8b8b", Light: "#7a7a7a"},
			Border:     lipgloss.AdaptiveColor{Dark: "#8b8b8b", Light: "#7a7a7a"},
			Error:      lipgloss.AdaptiveColor{Dark: "#FF8080", Light: "#FF8080"},
			Warning:    lipgloss.AdaptiveColor{Dark: "#FFC799", Light: "#FFC799"},
			Success:    lipgloss.AdaptiveColor{Dark: "#99FFE4", Light: "#99FFE4"},
		},
	})
}
