package theme

func init() {
	Register("tokyonight", &Theme{
		Name: "tokyonight",
		Colors: Colors{
			Background: adapt("#1a1b26", "#d5d6db"),
			Foreground: adapt("#c0caf5", "#343b58"),
			Primary:    adapt("#7aa2f7", "#34548a"),
			Secondary:  adapt("#bb9af7", "#5a4a78"),
			Accent:     adapt("#7dcfff", "#0f4b6e"),
			Muted:      adapt("#565f89", "#9699a3"),
			Border:     adapt("#3b4261", "#a9b1d6"),
			SelBg:      adapt("#3b4261", "#a9b1d6"),
			Error:      adapt("#db4b4b", "#8c4351"),
			Warning:    adapt("#e0af68", "#8f5e15"),
			Success:    adapt("#00D26A", "#33635c"),
		},
	})
}
