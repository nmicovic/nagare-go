package theme

func init() {
	Register("kanagawa", &Theme{
		Name: "kanagawa",
		Colors: Colors{
			Background: adapt("#1F1F28", "#F2E9DE"),
			Foreground: adapt("#DCD7BA", "#54433A"),
			Primary:    adapt("#7E9CD8", "#2D4F67"),
			Secondary:  adapt("#76946A", "#76946A"),
			Accent:     adapt("#D27E99", "#D27E99"),
			Muted:      adapt("#727169", "#9E9389"),
			Border:     adapt("#727169", "#9E9389"),
			SelBg:      adapt("#727169", "#9E9389"),
			Error:      adapt("#E82424", "#E82424"),
			Warning:    adapt("#D7A657", "#D7A657"),
			Success:    adapt("#98BB6C", "#98BB6C"),
		},
	})
}
