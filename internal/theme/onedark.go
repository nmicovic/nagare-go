package theme

func init() {
	Register("one-dark", &Theme{
		Name: "one-dark",
		Colors: Colors{
			Background: adapt("#282c34", "#fafafa"),
			Foreground: adapt("#abb2bf", "#383a42"),
			Primary:    adapt("#61afef", "#4078f2"),
			Secondary:  adapt("#56b6c2", "#0184bc"),
			Accent:     adapt("#c678dd", "#a626a4"),
			Muted:      adapt("#5c6370", "#a0a1a7"),
			Border:     adapt("#5c6370", "#a0a1a7"),
			SelBg:      adapt("#5c6370", "#a0a1a7"),
			Error:      adapt("#e06c75", "#e45649"),
			Warning:    adapt("#e5c07b", "#c18401"),
			Success:    adapt("#98c379", "#50a14f"),
		},
	})
}
