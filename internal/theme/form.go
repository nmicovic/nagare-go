package theme

import (
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// FormTheme adapts the active Nagare palette to huh forms.
type FormTheme struct{}

// Theme implements huh.Theme.
func (FormTheme) Theme(isDark bool) *huh.Styles {
	styles := huh.ThemeBase(isDark)
	colors := Current().Colors

	styles.Focused.Base = styles.Focused.Base.BorderForeground(colors.Primary)
	styles.Focused.Title = lipgloss.NewStyle().Foreground(colors.Primary).Bold(true)
	styles.Focused.Description = lipgloss.NewStyle().Foreground(colors.Muted)
	styles.Focused.SelectSelector = lipgloss.NewStyle().Foreground(colors.Accent).Bold(true)
	styles.Focused.SelectedOption = lipgloss.NewStyle().Foreground(colors.Accent).Bold(true)
	styles.Focused.UnselectedOption = lipgloss.NewStyle().Foreground(colors.Foreground)
	styles.Focused.FocusedButton = lipgloss.NewStyle().
		Background(colors.Primary).
		Foreground(colors.Background).
		Padding(0, 2)
	styles.Focused.BlurredButton = lipgloss.NewStyle().
		Foreground(colors.Foreground).
		Padding(0, 2)
	styles.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(colors.Accent)
	styles.Focused.TextInput.Placeholder = lipgloss.NewStyle().Foreground(colors.Muted)
	styles.Focused.ErrorIndicator = lipgloss.NewStyle().Foreground(colors.Error)
	styles.Focused.ErrorMessage = lipgloss.NewStyle().Foreground(colors.Error)

	styles.Blurred.Title = lipgloss.NewStyle().Foreground(colors.Muted)
	styles.Blurred.Description = lipgloss.NewStyle().Foreground(colors.Muted)

	styles.Help.FullKey = lipgloss.NewStyle().Foreground(colors.Muted)
	styles.Help.FullDesc = lipgloss.NewStyle().Foreground(colors.Muted)
	styles.Help.ShortKey = lipgloss.NewStyle().Foreground(colors.Muted)
	styles.Help.ShortDesc = lipgloss.NewStyle().Foreground(colors.Muted)

	return styles
}
