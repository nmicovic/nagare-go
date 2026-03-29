package theme

import "github.com/charmbracelet/lipgloss"

func init() {
	Register("catppuccin", &Theme{
		Name: "catppuccin",
		Colors: Colors{
			Background: lipgloss.AdaptiveColor{Dark: "#1e1e2e", Light: "#eff1f5"},
			Foreground: lipgloss.AdaptiveColor{Dark: "#cdd6f4", Light: "#4c4f69"},
			Primary:    lipgloss.AdaptiveColor{Dark: "#89b4fa", Light: "#1e66f5"},
			Secondary:  lipgloss.AdaptiveColor{Dark: "#cba6f7", Light: "#8839ef"},
			Accent:     lipgloss.AdaptiveColor{Dark: "#89dceb", Light: "#179299"},
			Muted:      lipgloss.AdaptiveColor{Dark: "#6c7086", Light: "#9ca0b0"},
			Border:     lipgloss.AdaptiveColor{Dark: "#45475a", Light: "#bcc0cc"},
			Error:      lipgloss.AdaptiveColor{Dark: "#f38ba8", Light: "#d20f39"},
			Warning:    lipgloss.AdaptiveColor{Dark: "#fab387", Light: "#df8e1d"},
			Success:    lipgloss.AdaptiveColor{Dark: "#a6e3a1", Light: "#40a02b"},
		},
	})
}
