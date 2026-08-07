package theme

func init() {
	Register("nord", &Theme{
		Name: "nord",
		Colors: Colors{
			Background: adapt("#2e3440", "#eceff4"),
			Foreground: adapt("#d8dee9", "#2e3440"),
			Primary:    adapt("#88c0d0", "#5e81ac"),
			Secondary:  adapt("#b48ead", "#8a4f8a"),
			Accent:     adapt("#81a1c1", "#4c7a9e"),
			Muted:      adapt("#4c566a", "#9ea7b0"),
			Border:     adapt("#3b4252", "#d8dee9"),
			SelBg:      adapt("#3b4252", "#d8dee9"),
			Error:      adapt("#bf616a", "#bf616a"),
			Warning:    adapt("#ebcb8b", "#d08770"),
			Success:    adapt("#a3be8c", "#a3be8c"),
		},
	})
}
