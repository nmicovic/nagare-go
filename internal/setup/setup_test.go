package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallClaudeHooks_NewFile(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	os.MkdirAll(claudeDir, 0755)

	if err := installClaudeHooks(home, "nagare-go-test"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("hooks key missing")
	}

	// Check standard events exist
	for _, event := range hookEvents {
		arr, ok := hooks[event].([]interface{})
		if !ok || len(arr) == 0 {
			t.Errorf("event %q missing or empty", event)
		}
	}

	// Check Notification event has matcher
	notifArr, ok := hooks["Notification"].([]interface{})
	if !ok || len(notifArr) == 0 {
		t.Fatal("Notification event missing")
	}
	notifEntry, ok := notifArr[0].(map[string]interface{})
	if !ok {
		t.Fatal("Notification entry is not a map")
	}
	if notifEntry["matcher"] != notificationMatcher {
		t.Errorf("matcher = %q, want %q", notifEntry["matcher"], notificationMatcher)
	}
}

func TestInstallClaudeHooks_PreservesExisting(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	os.MkdirAll(claudeDir, 0755)

	// Write existing settings with a custom hook
	existing := map[string]interface{}{
		"hooks": map[string]interface{}{
			"Stop": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": "my-custom-hook",
				},
			},
		},
		"other_setting": true,
	}
	data, _ := json.Marshal(existing)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644)

	if err := installClaudeHooks(home, "nagare-go-test"); err != nil {
		t.Fatal(err)
	}

	result, err := loadJSON(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}

	// other_setting preserved
	if result["other_setting"] != true {
		t.Error("other_setting should be preserved")
	}

	// Custom hook preserved
	hooks := result["hooks"].(map[string]interface{})
	stopArr := hooks["Stop"].([]interface{})
	if len(stopArr) < 2 {
		t.Fatalf("Stop should have custom + nagare hooks, got %d", len(stopArr))
	}

	// First should be the custom hook
	first := stopArr[0].(map[string]interface{})
	if first["command"] != "my-custom-hook" {
		t.Errorf("custom hook should be preserved, got %q", first["command"])
	}
}

func TestInstallClaudeHooks_RemovesStaleHooks(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	os.MkdirAll(claudeDir, 0755)

	// Write settings with old nagare hooks
	existing := map[string]interface{}{
		"hooks": map[string]interface{}{
			"Stop": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": "/old/path/nagare-go hook-state",
					"timeout": 5,
				},
				map[string]interface{}{
					"type":    "command",
					"command": "my-custom-hook",
				},
			},
		},
	}
	data, _ := json.Marshal(existing)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644)

	if err := installClaudeHooks(home, "nagare-go-test"); err != nil {
		t.Fatal(err)
	}

	result, _ := loadJSON(filepath.Join(claudeDir, "settings.json"))
	hooks := result["hooks"].(map[string]interface{})
	stopArr := hooks["Stop"].([]interface{})

	// Should have custom hook + new nagare hook (stale one removed)
	if len(stopArr) != 2 {
		t.Fatalf("expected 2 hooks (custom + fresh nagare), got %d", len(stopArr))
	}

	// First should be custom, second should be fresh nagare
	first := stopArr[0].(map[string]interface{})
	if first["command"] != "my-custom-hook" {
		t.Errorf("first hook should be custom, got %q", first["command"])
	}
}
