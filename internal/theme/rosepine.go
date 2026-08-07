package theme

func init() {
	Register("rosepine", &Theme{
		Name: "rosepine",
		Colors: Colors{
			Background: adapt("#191724", "#faf4ed"),
			Foreground: adapt("#e0def4", "#575279"),
			Primary:    adapt("#9ccfd8", "#31748f"),
			Secondary:  adapt("#9ccfd8", "#56949f"),
			Accent:     adapt("#ebbcba", "#d7827e"),
			Muted:      adapt("#6e6a86", "#9893a5"),
			Border:     adapt("#6e6a86", "#9893a5"),
			SelBg:      adapt("#6e6a86", "#9893a5"),
			Error:      adapt("#eb6f92", "#b4637a"),
			Warning:    adapt("#f6c177", "#ea9d34"),
			Success:    adapt("#31748f", "#286983"),
		},
	})
}
