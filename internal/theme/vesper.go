package theme

func init() {
	Register("vesper", &Theme{
		Name: "vesper",
		Colors: Colors{
			Background: adapt("#101010", "#F0F0F0"),
			Foreground: adapt("#FFF", "#101010"),
			Primary:    adapt("#FFC799", "#FFC799"),
			Secondary:  adapt("#FFC799", "#FFC799"),
			Accent:     adapt("#FF8080", "#B30000"),
			Muted:      adapt("#8b8b8b", "#7a7a7a"),
			Border:     adapt("#8b8b8b", "#7a7a7a"),
			SelBg:      adapt("#8b8b8b", "#7a7a7a"),
			Error:      adapt("#FF8080", "#FF8080"),
			Warning:    adapt("#FFC799", "#FFC799"),
			Success:    adapt("#99FFE4", "#99FFE4"),
		},
	})
}
