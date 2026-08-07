package theme

func init() {
	Register("flexoki", &Theme{
		Name: "flexoki",
		Colors: Colors{
			Background: adapt("#100F0F", "#FFFCF0"),
			Foreground: adapt("#CECDC3", "#100F0F"),
			Primary:    adapt("#DA702C", "#205EA6"),
			Secondary:  adapt("#3AA99F", "#24837B"),
			Accent:     adapt("#8B7EC8", "#BC5215"),
			Muted:      adapt("#6F6E69", "#6F6E69"),
			Border:     adapt("#6F6E69", "#6F6E69"),
			SelBg:      adapt("#6F6E69", "#6F6E69"),
			Error:      adapt("#D14D41", "#AF3029"),
			Warning:    adapt("#DA702C", "#BC5215"),
			Success:    adapt("#879A39", "#66800B"),
		},
	})
}
