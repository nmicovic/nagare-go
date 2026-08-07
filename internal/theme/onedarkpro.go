package theme

func init() {
	Register("onedarkpro", &Theme{
		Name: "onedarkpro",
		Colors: Colors{
			Background: adapt("#1e222a", "#f5f6f8"),
			Foreground: adapt("#abb2bf", "#2b303b"),
			Primary:    adapt("#61afef", "#528bff"),
			Secondary:  adapt("#56b6c2", "#61afef"),
			Accent:     adapt("#e06c75", "#d85462"),
			Muted:      adapt("#5c6370", "#6a717d"),
			Border:     adapt("#5c6370", "#6a717d"),
			SelBg:      adapt("#5c6370", "#6a717d"),
			Error:      adapt("#e06c75", "#e06c75"),
			Warning:    adapt("#e5c07b", "#d19a66"),
			Success:    adapt("#98c379", "#4fa66d"),
		},
	})
}
