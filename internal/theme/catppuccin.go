package theme

func init() {
	Register("catppuccin", &Theme{
		Name: "catppuccin",
		Colors: Colors{
			Background: adapt("#1e1e2e", "#eff1f5"),
			Foreground: adapt("#cdd6f4", "#4c4f69"),
			Primary:    adapt("#89b4fa", "#1e66f5"),
			Secondary:  adapt("#cba6f7", "#8839ef"),
			Accent:     adapt("#89dceb", "#179299"),
			Muted:      adapt("#6c7086", "#9ca0b0"),
			Border:     adapt("#45475a", "#bcc0cc"),
			SelBg:      adapt("#45475a", "#bcc0cc"),
			Error:      adapt("#f38ba8", "#d20f39"),
			Warning:    adapt("#fab387", "#df8e1d"),
			Success:    adapt("#a6e3a1", "#40a02b"),
		},
	})
}
