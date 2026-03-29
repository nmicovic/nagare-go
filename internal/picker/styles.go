package picker

import "github.com/charmbracelet/lipgloss"

// Theme holds the color palette for the picker.
type Theme struct {
	Background lipgloss.Color
	Foreground lipgloss.Color
	Primary    lipgloss.Color
	Secondary  lipgloss.Color
	Accent     lipgloss.Color
	Muted      lipgloss.Color
	Border     lipgloss.Color
}

var themes = map[string]Theme{
	"tokyonight": {
		Background: lipgloss.Color("#1a1b26"),
		Foreground: lipgloss.Color("#c0caf5"),
		Primary:    lipgloss.Color("#7aa2f7"),
		Secondary:  lipgloss.Color("#bb9af7"),
		Accent:     lipgloss.Color("#7dcfff"),
		Muted:      lipgloss.Color("#565f89"),
		Border:     lipgloss.Color("#3b4261"),
	},
	"catppuccin": {
		Background: lipgloss.Color("#1e1e2e"),
		Foreground: lipgloss.Color("#cdd6f4"),
		Primary:    lipgloss.Color("#89b4fa"),
		Secondary:  lipgloss.Color("#cba6f7"),
		Accent:     lipgloss.Color("#89dceb"),
		Muted:      lipgloss.Color("#6c7086"),
		Border:     lipgloss.Color("#45475a"),
	},
	"gruvbox": {
		Background: lipgloss.Color("#282828"),
		Foreground: lipgloss.Color("#ebdbb2"),
		Primary:    lipgloss.Color("#83a598"),
		Secondary:  lipgloss.Color("#d3869b"),
		Accent:     lipgloss.Color("#8ec07c"),
		Muted:      lipgloss.Color("#928374"),
		Border:     lipgloss.Color("#504945"),
	},
}

// ThemeNames returns available theme names in a stable order.
func ThemeNames() []string {
	return []string{"tokyonight", "catppuccin", "gruvbox"}
}

// Styles holds all lipgloss styles derived from a theme.
type Styles struct {
	Theme        Theme
	SessionList  lipgloss.Style
	SessionItem  lipgloss.Style
	SelectedItem lipgloss.Style
	DetailPanel  lipgloss.Style
	PreviewPanel lipgloss.Style
	SearchInput  lipgloss.Style
	Title        lipgloss.Style
	Muted        lipgloss.Style
	Text         lipgloss.Style
}

// NewStyles creates styles from a theme name. Falls back to tokyonight.
func NewStyles(themeName string) Styles {
	t, ok := themes[themeName]
	if !ok {
		t = themes["tokyonight"]
	}

	base := lipgloss.NewStyle().
		Foreground(t.Foreground).
		Background(t.Background)

	return Styles{
		Theme: t,
		SessionList: base.
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(t.Border).
			BorderBackground(t.Background).
			Padding(1),
		SessionItem: base.
			PaddingLeft(2),
		SelectedItem: base.
			PaddingLeft(1).
			Foreground(t.Primary).
			Bold(true),
		DetailPanel: base.
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(t.Border).
			BorderBackground(t.Background).
			Padding(1),
		PreviewPanel: base.
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(t.Muted).
			BorderBackground(t.Background).
			Padding(0, 1),
		SearchInput: base.
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(t.Accent).
			BorderBackground(t.Background).
			Padding(0, 1),
		Title: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true),
		Muted: lipgloss.NewStyle().
			Foreground(t.Muted),
		Text: base,
	}
}
