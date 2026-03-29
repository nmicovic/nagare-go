package theme

import "github.com/charmbracelet/lipgloss"

func init() {
	Register("monokai", &Theme{
		Name: "monokai",
		Colors: Colors{
			Background: lipgloss.AdaptiveColor{Dark: "#2d2a2e", Light: "#fafafa"},
			Foreground: lipgloss.AdaptiveColor{Dark: "#fcfcfa", Light: "#2c292d"},
			Primary:    lipgloss.AdaptiveColor{Dark: "#78dce8", Light: "#0b8ec4"},
			Secondary:  lipgloss.AdaptiveColor{Dark: "#ab9df2", Light: "#6e4db2"},
			Accent:     lipgloss.AdaptiveColor{Dark: "#a9dc76", Light: "#4b830d"},
			Muted:      lipgloss.AdaptiveColor{Dark: "#727072", Light: "#9e9e9e"},
			Border:     lipgloss.AdaptiveColor{Dark: "#403e41", Light: "#d6d6d6"},
			Error:      lipgloss.AdaptiveColor{Dark: "#ff6188", Light: "#e53935"},
			Warning:    lipgloss.AdaptiveColor{Dark: "#ffd866", Light: "#f9a825"},
			Success:    lipgloss.AdaptiveColor{Dark: "#a9dc76", Light: "#43a047"},
		},
	})
}
