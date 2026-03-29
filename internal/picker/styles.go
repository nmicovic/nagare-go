package picker

import "github.com/charmbracelet/lipgloss"

// Theme holds the color palette for the picker.
type Theme struct {
	Name       string
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
		Name:       "tokyonight",
		Background: lipgloss.Color("#1a1b26"),
		Foreground: lipgloss.Color("#c0caf5"),
		Primary:    lipgloss.Color("#7aa2f7"),
		Secondary:  lipgloss.Color("#bb9af7"),
		Accent:     lipgloss.Color("#7dcfff"),
		Muted:      lipgloss.Color("#565f89"),
		Border:     lipgloss.Color("#3b4261"),
	},
	"catppuccin": {
		Name:       "catppuccin",
		Background: lipgloss.Color("#1e1e2e"),
		Foreground: lipgloss.Color("#cdd6f4"),
		Primary:    lipgloss.Color("#89b4fa"),
		Secondary:  lipgloss.Color("#cba6f7"),
		Accent:     lipgloss.Color("#89dceb"),
		Muted:      lipgloss.Color("#6c7086"),
		Border:     lipgloss.Color("#45475a"),
	},
	"gruvbox": {
		Name:       "gruvbox",
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
	StatusBar    lipgloss.Style
	Title        lipgloss.Style
	Muted        lipgloss.Style
}

// NewStyles creates styles from a theme name. Falls back to tokyonight.
func NewStyles(themeName string) Styles {
	theme, ok := themes[themeName]
	if !ok {
		theme = themes["tokyonight"]
	}

	return Styles{
		Theme: theme,
		SessionList: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(theme.Border).
			Padding(1),
		SessionItem: lipgloss.NewStyle().
			PaddingLeft(2),
		SelectedItem: lipgloss.NewStyle().
			PaddingLeft(1).
			Foreground(theme.Primary).
			Bold(true),
		DetailPanel: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(theme.Border).
			Padding(1),
		PreviewPanel: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(theme.Muted).
			Padding(0, 1),
		SearchInput: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(theme.Accent).
			Padding(0, 1),
		StatusBar: lipgloss.NewStyle().
			Foreground(theme.Muted),
		Title: lipgloss.NewStyle().
			Foreground(theme.Primary).
			Bold(true),
		Muted: lipgloss.NewStyle().
			Foreground(theme.Muted),
	}
}
