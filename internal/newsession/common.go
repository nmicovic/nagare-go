package newsession

import (
	"errors"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/nemke/nagare-go/internal/theme"
)

var errEmptyName = errors.New("name is required")

// nagareFormTheme implements huh.Theme by building a *huh.Styles on demand
// from the active nagare theme. Honours the terminal's dark/light preference
// via the isDark flag that huh passes in at render time.
type nagareFormTheme struct{}

func (nagareFormTheme) Theme(isDark bool) *huh.Styles {
	t := huh.ThemeBase(isDark)
	c := theme.Current().Colors

	t.Focused.Base = t.Focused.Base.BorderForeground(c.Primary)
	t.Focused.Title = lipgloss.NewStyle().Foreground(c.Primary).Bold(true)
	t.Focused.Description = lipgloss.NewStyle().Foreground(c.Muted)
	t.Focused.SelectSelector = lipgloss.NewStyle().Foreground(c.Accent).Bold(true)
	t.Focused.SelectedOption = lipgloss.NewStyle().Foreground(c.Accent).Bold(true)
	t.Focused.UnselectedOption = lipgloss.NewStyle().Foreground(c.Foreground)
	t.Focused.FocusedButton = lipgloss.NewStyle().
		Background(c.Primary).
		Foreground(c.Background).
		Padding(0, 2)
	t.Focused.BlurredButton = lipgloss.NewStyle().
		Foreground(c.Foreground).
		Padding(0, 2)
	t.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(c.Accent)
	t.Focused.TextInput.Placeholder = lipgloss.NewStyle().Foreground(c.Muted)
	t.Focused.ErrorIndicator = lipgloss.NewStyle().Foreground(c.Error)
	t.Focused.ErrorMessage = lipgloss.NewStyle().Foreground(c.Error)

	t.Blurred.Title = lipgloss.NewStyle().Foreground(c.Muted)
	t.Blurred.Description = lipgloss.NewStyle().Foreground(c.Muted)

	t.Help.FullKey = lipgloss.NewStyle().Foreground(c.Muted)
	t.Help.FullDesc = lipgloss.NewStyle().Foreground(c.Muted)
	t.Help.ShortKey = lipgloss.NewStyle().Foreground(c.Muted)
	t.Help.ShortDesc = lipgloss.NewStyle().Foreground(c.Muted)

	return t
}

func formTheme() huh.Theme { return nagareFormTheme{} }
