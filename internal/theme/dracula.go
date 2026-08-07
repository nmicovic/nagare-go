package theme

func init() {
	Register("dracula", &Theme{
		Name: "dracula",
		Colors: Colors{
			Background: adapt("#282a36", "#f8f8f2"),
			Foreground: adapt("#f8f8f2", "#282a36"),
			Primary:    adapt("#bd93f9", "#7e57c2"),
			Secondary:  adapt("#ff79c6", "#d81b60"),
			Accent:     adapt("#8be9fd", "#0097a7"),
			Muted:      adapt("#6272a4", "#9e9e9e"),
			Border:     adapt("#44475a", "#bdbdbd"),
			SelBg:      adapt("#44475a", "#bdbdbd"),
			Error:      adapt("#ff5555", "#e53935"),
			Warning:    adapt("#f1fa8c", "#f9a825"),
			Success:    adapt("#50fa7b", "#43a047"),
		},
	})
}
