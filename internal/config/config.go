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
// Uses a two-pass approach: first decode into a raw map to see which sections were
// specified, then only override defaults for sections that are present. This prevents
// a partial [notifications.needs_input] from zeroing out unspecified fields.
func LoadFrom(path string) (NagareConfig, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	// First pass: decode into raw map to detect which keys are present
	var raw map[string]interface{}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return cfg, err
	}

	// Second pass: decode into a zero-value struct
	var parsed NagareConfig
	if _, err := toml.Decode(string(data), &parsed); err != nil {
		return cfg, err
	}

	// Merge top-level fields
	if notifRaw, ok := raw["notifications"]; ok {
		notifMap, _ := notifRaw.(map[string]interface{})
		if _, ok := notifMap["enabled"]; ok {
			cfg.Notifications.Enabled = parsed.Notifications.Enabled
		}
		if _, ok := notifMap["needs_input"]; ok {
			cfg.Notifications.NeedsInput = mergeEventConfig(cfg.Notifications.NeedsInput, parsed.Notifications.NeedsInput, notifMap["needs_input"])
		}
		if _, ok := notifMap["task_complete"]; ok {
			cfg.Notifications.TaskComplete = mergeEventConfig(cfg.Notifications.TaskComplete, parsed.Notifications.TaskComplete, notifMap["task_complete"])
		}
	}
	if _, ok := raw["picker"]; ok {
		cfg.Picker = mergePicker(cfg.Picker, parsed.Picker, raw["picker"])
	}
	if _, ok := raw["appearance"]; ok {
		cfg.Appearance = mergeAppearance(cfg.Appearance, parsed.Appearance, raw["appearance"])
	}
	if _, ok := raw["notification_duration"]; ok {
		cfg.NotificationDuration = parsed.NotificationDuration
	}

	return cfg, nil
}

// mergeEventConfig merges only the fields that were specified in the TOML.
func mergeEventConfig(defaults, parsed NotificationEventConfig, rawVal interface{}) NotificationEventConfig {
	m, ok := rawVal.(map[string]interface{})
	if !ok {
		return defaults
	}
	result := defaults
	if _, ok := m["toast"]; ok {
		result.Toast = parsed.Toast
	}
	if _, ok := m["bell"]; ok {
		result.Bell = parsed.Bell
	}
	if _, ok := m["os_notify"]; ok {
		result.OsNotify = parsed.OsNotify
	}
	if _, ok := m["popup"]; ok {
		result.Popup = parsed.Popup
	}
	if _, ok := m["popup_timeout"]; ok {
		result.PopupTimeout = parsed.PopupTimeout
	}
	if _, ok := m["min_working_seconds"]; ok {
		result.MinWorkingSeconds = parsed.MinWorkingSeconds
	}
	return result
}

// mergePicker merges only specified picker fields.
func mergePicker(defaults, parsed PickerConfig, rawVal interface{}) PickerConfig {
	m, ok := rawVal.(map[string]interface{})
	if !ok {
		return defaults
	}
	result := defaults
	if _, ok := m["quick_project_path"]; ok {
		result.QuickProjectPath = parsed.QuickProjectPath
	}
	if _, ok := m["popup_width"]; ok {
		result.PopupWidth = parsed.PopupWidth
	}
	if _, ok := m["popup_height"]; ok {
		result.PopupHeight = parsed.PopupHeight
	}
	if _, ok := m["grid_refresh_interval"]; ok {
		result.GridRefreshInterval = parsed.GridRefreshInterval
	}
	return result
}

// mergeAppearance merges only specified appearance fields.
func mergeAppearance(defaults, parsed AppearanceConfig, rawVal interface{}) AppearanceConfig {
	m, ok := rawVal.(map[string]interface{})
	if !ok {
		return defaults
	}
	result := defaults
	if _, ok := m["theme"]; ok {
		result.Theme = parsed.Theme
	}
	if _, ok := m["icon_style"]; ok {
		result.IconStyle = parsed.IconStyle
	}
	return result
}

// Load loads config from the default path.
func Load() (NagareConfig, error) {
	return LoadFrom(DefaultPath())
}
