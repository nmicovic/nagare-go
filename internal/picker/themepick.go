package picker

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nemke/nagare-go/internal/log"
	"github.com/nemke/nagare-go/internal/theme"
)

// handleThemePickKey handles keys when the theme picker overlay is open.
func (m Model) handleThemePickKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case keyEscape:
		// Cancel — restore original theme
		theme.Set(m.themeOriginal)
		m.showThemePick = false
		return m, nil
	case keyEnter:
		// Confirm current selection
		log.Info("theme confirmed: %s", m.themeNames[m.themeCursor])
		m.showThemePick = false
		return m, nil
	case keyUp:
		if m.themeCursor > 0 {
			m.themeCursor--
			theme.Set(m.themeNames[m.themeCursor])
		}
		return m, nil
	case keyDown:
		if m.themeCursor < len(m.themeNames)-1 {
			m.themeCursor++
			theme.Set(m.themeNames[m.themeCursor])
		}
		return m, nil
	}
	return m, nil
}

// themePickOverlay renders the theme selection dialog (centered by placeOverlay).
func themePickOverlay(names []string, cursor int, width, height int) string {
	c := theme.Current().Colors

	title := lipgloss.NewStyle().
		Foreground(c.Primary).
		Bold(true).
		Render("Select Theme")

	hint := mutedStyle().Render("↑/↓ preview  Enter confirm  Esc cancel")

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")

	itemWidth := 24

	for i, name := range names {
		// Show a color swatch for the theme
		t := getThemeColors(name)
		swatch := ""
		if t != nil {
			swatch = lipgloss.NewStyle().Foreground(t.Primary).Render("●") +
				lipgloss.NewStyle().Foreground(t.Secondary).Render("●") +
				lipgloss.NewStyle().Foreground(t.Accent).Render("●") +
				lipgloss.NewStyle().Foreground(t.Success).Render("●") +
				lipgloss.NewStyle().Foreground(t.Warning).Render("●") +
				lipgloss.NewStyle().Foreground(t.Error).Render("●")
		}

		label := fmt.Sprintf("  %s  %s", name, swatch)

		if i == cursor {
			line := lipgloss.NewStyle().
				Background(c.Primary).
				Foreground(c.Background).
				Bold(true).
				Width(itemWidth).
				Render(label)
			lines = append(lines, line)
		} else {
			line := lipgloss.NewStyle().
				Foreground(c.Foreground).
				Width(itemWidth).
				Render(label)
			lines = append(lines, line)
		}
	}

	lines = append(lines, "")
	lines = append(lines, hint)

	content := ""
	for i, l := range lines {
		if i > 0 {
			content += "\n"
		}
		content += l
	}

	dialogWidth := itemWidth + 8
	dialogHeight := len(names) + 8

	return lipgloss.NewStyle().
		Background(c.Background).
		Foreground(c.Foreground).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(c.Primary).
		BorderBackground(c.Background).
		Width(dialogWidth).
		Height(dialogHeight).
		Padding(1, 2).
		Render(content)
}

// getThemeColors returns the Colors for a theme by name, or nil.
func getThemeColors(name string) *theme.Colors {
	t := theme.Get(name)
	if t == nil {
		return nil
	}
	return &t.Colors
}
