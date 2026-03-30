package theme

import "github.com/charmbracelet/lipgloss"

func init() {
	Register("rosepine", &Theme{
		Name: "rosepine",
		Colors: Colors{
			Background: lipgloss.AdaptiveColor{Dark: "#191724", Light: "#faf4ed"},
			Foreground: lipgloss.AdaptiveColor{Dark: "#e0def4", Light: "#575279"},
			Primary:    lipgloss.AdaptiveColor{Dark: "#9ccfd8", Light: "#31748f"},
			Secondary:  lipgloss.AdaptiveColor{Dark: "#9ccfd8", Light: "#56949f"},
			Accent:     lipgloss.AdaptiveColor{Dark: "#ebbcba", Light: "#d7827e"},
			Muted:      lipgloss.AdaptiveColor{Dark: "#6e6a86", Light: "#9893a5"},
			Border:     lipgloss.AdaptiveColor{Dark: "#6e6a86", Light: "#9893a5"},
			Error:      lipgloss.AdaptiveColor{Dark: "#eb6f92", Light: "#b4637a"},
			Warning:    lipgloss.AdaptiveColor{Dark: "#f6c177", Light: "#ea9d34"},
			Success:    lipgloss.AdaptiveColor{Dark: "#31748f", Light: "#286983"},
		},
	})
}
