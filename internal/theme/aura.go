package theme

import "github.com/charmbracelet/lipgloss"

func init() {
	Register("aura", &Theme{
		Name: "aura",
		Colors: Colors{
			Background: lipgloss.AdaptiveColor{Dark: "#15141b", Light: "#f5f0ff"},
			Foreground: lipgloss.AdaptiveColor{Dark: "#edecee", Light: "#2d2640"},
			Primary:    lipgloss.AdaptiveColor{Dark: "#a277ff", Light: "#a277ff"},
			Secondary:  lipgloss.AdaptiveColor{Dark: "#82e2ff", Light: "#5bb8d9"},
			Accent:     lipgloss.AdaptiveColor{Dark: "#ff6767", Light: "#d94f4f"},
			Muted:      lipgloss.AdaptiveColor{Dark: "#6d6a7e", Light: "#8d88a3"},
			Border:     lipgloss.AdaptiveColor{Dark: "#6d6a7e", Light: "#8d88a3"},
			Error:      lipgloss.AdaptiveColor{Dark: "#ff6767", Light: "#d94f4f"},
			Warning:    lipgloss.AdaptiveColor{Dark: "#ffca85", Light: "#d9a24a"},
			Success:    lipgloss.AdaptiveColor{Dark: "#61ffca", Light: "#40bf7a"},
		},
	})
}
