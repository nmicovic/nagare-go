package picker

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nemke/nagare-go/internal/log"
	"github.com/nemke/nagare-go/internal/theme"
)

// handleThemePickKey handles keys when the theme picker overlay is open.
func (m Model) handleThemePickKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case keyEscape:
		theme.Set(m.themeOriginal)
		m.showThemePick = false
	case keyEnter:
		log.Info("theme confirmed: %s", m.themeNames[m.themeCursor])
		m.showThemePick = false
	case keyUp:
		if m.themeCursor > 0 {
			m.themeCursor--
			theme.Set(m.themeNames[m.themeCursor])
		}
	case keyDown:
		if m.themeCursor < len(m.themeNames)-1 {
			m.themeCursor++
			theme.Set(m.themeNames[m.themeCursor])
		}
	}
	return m, nil
}

// themePickOverlay renders the theme selection dialog.
func themePickOverlay(names []string, cursor int, width, height int) string {
	c := theme.Current().Colors

	title := titleStyle().Render("Select Theme")
	hint := mutedStyle().Render("↑/↓ preview  Enter confirm  Esc cancel")

	itemWidth := 24
	lines := []string{title, ""}

	for i, name := range names {
		// Color swatch from the theme's palette
		swatch := ""
		if t := theme.Get(name); t != nil {
			tc := t.Colors
			swatch = lipgloss.NewStyle().Foreground(tc.Primary).Render("●") +
				lipgloss.NewStyle().Foreground(tc.Secondary).Render("●") +
				lipgloss.NewStyle().Foreground(tc.Accent).Render("●") +
				lipgloss.NewStyle().Foreground(tc.Success).Render("●") +
				lipgloss.NewStyle().Foreground(tc.Warning).Render("●") +
				lipgloss.NewStyle().Foreground(tc.Error).Render("●")
		}

		label := fmt.Sprintf("  %s  %s", name, swatch)
		style := lipgloss.NewStyle().Foreground(c.Foreground).Width(itemWidth)
		if i == cursor {
			style = lipgloss.NewStyle().
				Background(c.Primary).Foreground(c.Background).Bold(true).Width(itemWidth)
		}
		lines = append(lines, style.Render(label))
	}

	lines = append(lines, "", hint)

	return dialogStyle().
		Width(itemWidth + 8).
		Height(len(names) + 8).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
}
