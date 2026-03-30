package theme

import "github.com/charmbracelet/lipgloss"

func init() {
	Register("flexoki", &Theme{
		Name: "flexoki",
		Colors: Colors{
			Background: lipgloss.AdaptiveColor{Dark: "#100F0F", Light: "#FFFCF0"},
			Foreground: lipgloss.AdaptiveColor{Dark: "#CECDC3", Light: "#100F0F"},
			Primary:    lipgloss.AdaptiveColor{Dark: "#DA702C", Light: "#205EA6"},
			Secondary:  lipgloss.AdaptiveColor{Dark: "#3AA99F", Light: "#24837B"},
			Accent:     lipgloss.AdaptiveColor{Dark: "#8B7EC8", Light: "#BC5215"},
			Muted:      lipgloss.AdaptiveColor{Dark: "#6F6E69", Light: "#6F6E69"},
			Border:     lipgloss.AdaptiveColor{Dark: "#6F6E69", Light: "#6F6E69"},
			Error:      lipgloss.AdaptiveColor{Dark: "#D14D41", Light: "#AF3029"},
			Warning:    lipgloss.AdaptiveColor{Dark: "#DA702C", Light: "#BC5215"},
			Success:    lipgloss.AdaptiveColor{Dark: "#879A39", Light: "#66800B"},
		},
	})
}
