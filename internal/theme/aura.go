package theme

func init() {
	Register("aura", &Theme{
		Name: "aura",
		Colors: Colors{
			Background: adapt("#15141b", "#f5f0ff"),
			Foreground: adapt("#edecee", "#2d2640"),
			Primary:    adapt("#a277ff", "#a277ff"),
			Secondary:  adapt("#82e2ff", "#5bb8d9"),
			Accent:     adapt("#ff6767", "#d94f4f"),
			Muted:      adapt("#6d6a7e", "#8d88a3"),
			Border:     adapt("#6d6a7e", "#8d88a3"),
			SelBg:      adapt("#6d6a7e", "#8d88a3"),
			Error:      adapt("#ff6767", "#d94f4f"),
			Warning:    adapt("#ffca85", "#d9a24a"),
			Success:    adapt("#61ffca", "#40bf7a"),
		},
	})
}
