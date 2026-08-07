package theme

func init() {
	Register("monokai", &Theme{
		Name: "monokai",
		Colors: Colors{
			Background: adapt("#2d2a2e", "#fafafa"),
			Foreground: adapt("#fcfcfa", "#2c292d"),
			Primary:    adapt("#78dce8", "#0b8ec4"),
			Secondary:  adapt("#ab9df2", "#6e4db2"),
			Accent:     adapt("#a9dc76", "#4b830d"),
			Muted:      adapt("#727072", "#9e9e9e"),
			Border:     adapt("#403e41", "#d6d6d6"),
			SelBg:      adapt("#403e41", "#d6d6d6"),
			Error:      adapt("#ff6188", "#e53935"),
			Warning:    adapt("#ffd866", "#f9a825"),
			Success:    adapt("#a9dc76", "#43a047"),
		},
	})
}
