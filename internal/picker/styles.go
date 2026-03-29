package picker

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/nemke/nagare-go/internal/theme"
)

// Style functions — always build fresh from theme.Current() so theme
// switches take effect on the next View() call.

func baseStyle() lipgloss.Style {
	c := theme.Current().Colors
	return lipgloss.NewStyle().
		Foreground(c.Foreground).
		Background(c.Background)
}

func panelStyle() lipgloss.Style {
	c := theme.Current().Colors
	return baseStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(c.Border).
		BorderBackground(c.Background).
		Padding(1)
}

func previewPanelStyle() lipgloss.Style {
	c := theme.Current().Colors
	return baseStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(c.Muted).
		BorderBackground(c.Background).
		Padding(0, 1)
}

func titleStyle() lipgloss.Style {
	c := theme.Current().Colors
	return lipgloss.NewStyle().
		Foreground(c.Primary).
		Bold(true)
}

func mutedStyle() lipgloss.Style {
	c := theme.Current().Colors
	return lipgloss.NewStyle().
		Foreground(c.Muted)
}

func selectedStyle() lipgloss.Style {
	c := theme.Current().Colors
	return lipgloss.NewStyle().
		Background(c.Background).
		Foreground(c.Primary).
		Bold(true).
		PaddingLeft(1)
}

func itemStyle() lipgloss.Style {
	c := theme.Current().Colors
	return lipgloss.NewStyle().
		Background(c.Background).
		Foreground(c.Foreground).
		PaddingLeft(2)
}

func dialogStyle() lipgloss.Style {
	c := theme.Current().Colors
	return lipgloss.NewStyle().
		Background(c.Background).
		Foreground(c.Foreground).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(c.Primary).
		BorderBackground(c.Background)
}
