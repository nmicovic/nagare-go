package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// NotificationEventConfig controls delivery for a single event type.
type NotificationEventConfig struct {
	Toast             bool `toml:"toast"`
	Bell              bool `toml:"bell"`
	OsNotify          bool `toml:"os_notify"`
	Popup             bool `toml:"popup"`
	PopupTimeout      int  `toml:"popup_timeout"`
	MinWorkingSeconds int  `toml:"min_working_seconds"`
}

// NotificationConfig controls all notification behavior.
type NotificationConfig struct {
	Enabled      bool                    `toml:"enabled"`
	NeedsInput   NotificationEventConfig `toml:"needs_input"`
	TaskComplete NotificationEventConfig `toml:"task_complete"`
}

// PickerConfig controls picker TUI behavior.
type PickerConfig struct {
	QuickProjectPath    string  `toml:"quick_project_path"`
	PopupWidth          string  `toml:"popup_width"`
	PopupHeight         string  `toml:"popup_height"`
	GridRefreshInterval float64 `toml:"grid_refresh_interval"`
}

// AppearanceConfig controls visual settings.
type AppearanceConfig struct {
	Theme     string `toml:"theme"`
	IconStyle string `toml:"icon_style"`
}

// NagareConfig is the top-level configuration.
type NagareConfig struct {
	Notifications        NotificationConfig `toml:"notifications"`
	Picker               PickerConfig       `toml:"picker"`
	Appearance           AppearanceConfig   `toml:"appearance"`
	NotificationDuration int                `toml:"notification_duration"`
}

// Default returns the default configuration.
func Default() NagareConfig {
	return NagareConfig{
		Notifications: NotificationConfig{
			Enabled: true,
			NeedsInput: NotificationEventConfig{
				Toast:             true,
				Bell:              true,
				OsNotify:          true,
				Popup:             false,
				PopupTimeout:      10,
				MinWorkingSeconds: 0,
			},
			TaskComplete: NotificationEventConfig{
				Toast:             true,
				Bell:              false,
				OsNotify:          false,
				Popup:             false,
				PopupTimeout:      10,
				MinWorkingSeconds: 30,
			},
		},
		Picker: PickerConfig{
			QuickProjectPath:    "~/Prototypes",
			PopupWidth:          "80%",
			PopupHeight:         "80%",
			GridRefreshInterval: 0.5,
		},
		Appearance: AppearanceConfig{
			Theme:     "tokyonight",
			IconStyle: "emoji",
		},
		NotificationDuration: 3000,
	}
}

// DefaultPath returns the default config file path.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "nagare", "config.toml")
}

// LoadFrom loads config from a specific path. Returns defaults if file doesn't exist.
func LoadFrom(path string) (NagareConfig, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Load loads config from the default path.
func Load() (NagareConfig, error) {
	return LoadFrom(DefaultPath())
}
