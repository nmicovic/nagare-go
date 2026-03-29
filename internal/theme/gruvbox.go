package theme

import "github.com/charmbracelet/lipgloss"

func init() {
	Register("gruvbox", &Theme{
		Name: "gruvbox",
		Colors: Colors{
			Background: lipgloss.AdaptiveColor{Dark: "#282828", Light: "#fbf1c7"},
			Foreground: lipgloss.AdaptiveColor{Dark: "#ebdbb2", Light: "#3c3836"},
			Primary:    lipgloss.AdaptiveColor{Dark: "#83a598", Light: "#076678"},
			Secondary:  lipgloss.AdaptiveColor{Dark: "#d3869b", Light: "#8f3f71"},
			Accent:     lipgloss.AdaptiveColor{Dark: "#8ec07c", Light: "#79740e"},
			Muted:      lipgloss.AdaptiveColor{Dark: "#928374", Light: "#928374"},
			Border:     lipgloss.AdaptiveColor{Dark: "#504945", Light: "#d5c4a1"},
			Error:      lipgloss.AdaptiveColor{Dark: "#fb4934", Light: "#cc241d"},
			Warning:    lipgloss.AdaptiveColor{Dark: "#fabd2f", Light: "#d79921"},
			Success:    lipgloss.AdaptiveColor{Dark: "#b8bb26", Light: "#98971a"},
		},
	})
}
