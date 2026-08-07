package theme

func init() {
	Register("gruvbox", &Theme{
		Name: "gruvbox",
		Colors: Colors{
			Background: adapt("#282828", "#fbf1c7"),
			Foreground: adapt("#ebdbb2", "#3c3836"),
			Primary:    adapt("#83a598", "#076678"),
			Secondary:  adapt("#d3869b", "#8f3f71"),
			Accent:     adapt("#8ec07c", "#79740e"),
			Muted:      adapt("#928374", "#928374"),
			Border:     adapt("#504945", "#d5c4a1"),
			SelBg:      adapt("#504945", "#d5c4a1"),
			Error:      adapt("#fb4934", "#cc241d"),
			Warning:    adapt("#fabd2f", "#d79921"),
			Success:    adapt("#b8bb26", "#98971a"),
		},
	})
}
