package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultConfig(t *testing.T) {
	cfg := Default()

	if !cfg.Notifications.Enabled {
		t.Error("notifications should be enabled by default")
	}
	if !cfg.Notifications.NeedsInput.Toast {
		t.Error("needs_input.toast should be true by default")
	}
	if !cfg.Notifications.NeedsInput.Bell {
		t.Error("needs_input.bell should be true by default")
	}
	if cfg.Notifications.TaskComplete.Bell {
		t.Error("task_complete.bell should be false by default")
	}
	if cfg.Notifications.TaskComplete.MinWorkingSeconds != 30 {
		t.Errorf("task_complete.min_working_seconds = %d, want 30", cfg.Notifications.TaskComplete.MinWorkingSeconds)
	}
	if cfg.Appearance.Theme != "tokyonight" {
		t.Errorf("theme = %q, want %q", cfg.Appearance.Theme, "tokyonight")
	}
}

func TestLoadFromTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	tomlContent := `
[notifications]
enabled = false

[notifications.task_complete]
min_working_seconds = 60

[appearance]
theme = "catppuccin"
`
	if err := os.WriteFile(path, []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Notifications.Enabled {
		t.Error("notifications should be disabled")
	}
	if cfg.Notifications.TaskComplete.MinWorkingSeconds != 60 {
		t.Errorf("min_working_seconds = %d, want 60", cfg.Notifications.TaskComplete.MinWorkingSeconds)
	}
	if cfg.Appearance.Theme != "catppuccin" {
		t.Errorf("theme = %q, want %q", cfg.Appearance.Theme, "catppuccin")
	}
	// Unset fields keep defaults
	if !cfg.Notifications.NeedsInput.Toast {
		t.Error("needs_input.toast should keep default true")
	}
}

func TestLoadPartialNestedOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Only override toast in needs_input — other fields should keep defaults
	tomlContent := `
[notifications.needs_input]
toast = false
`
	if err := os.WriteFile(path, []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Notifications.NeedsInput.Toast {
		t.Error("toast should be overridden to false")
	}
	// These must keep their defaults, not be zeroed
	if !cfg.Notifications.NeedsInput.Bell {
		t.Error("bell should keep default true")
	}
	if !cfg.Notifications.NeedsInput.OsNotify {
		t.Error("os_notify should keep default true")
	}
	if cfg.Notifications.NeedsInput.PopupTimeout != 10 {
		t.Errorf("popup_timeout = %d, want default 10", cfg.Notifications.NeedsInput.PopupTimeout)
	}
	// task_complete should be completely untouched
	if cfg.Notifications.TaskComplete.MinWorkingSeconds != 30 {
		t.Errorf("task_complete.min_working_seconds = %d, want default 30", cfg.Notifications.TaskComplete.MinWorkingSeconds)
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := LoadFrom("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatal("missing file should return defaults, not error")
	}
	if cfg.Appearance.Theme != "tokyonight" {
		t.Errorf("theme = %q, want default %q", cfg.Appearance.Theme, "tokyonight")
	}
}
