package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nemke/nagare-go/internal/hooks"
	"github.com/nemke/nagare-go/internal/mcp"
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

func TestInstallPiExtension(t *testing.T) {
	home := t.TempDir()
	if err := installPiExtension(home, "/opt/bin/nagare-go"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(home, ".pi", "agent", "extensions", "nagare.ts")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// The binary path must be embedded as a quoted JS string.
	if !strings.Contains(content, `const NAGARE = "/opt/bin/nagare-go"`) {
		t.Error("nagare binary path not embedded")
	}
	// No unexpanded Go format verbs may survive into the generated file.
	if strings.Contains(content, "%q") || strings.Contains(content, "%%") {
		t.Error("generated extension contains unexpanded format verbs")
	}
	// Every messaging tool must be registered, or pi loses parity with MCP agents.
	for _, tool := range mcp.ToolNames() {
		if !strings.Contains(content, `name: "`+tool+`"`) {
			t.Errorf("tool %q not registered in pi extension", tool)
		}
	}
	// agent_settled, not agent_end: pi may keep working after agent_end.
	if !strings.Contains(content, "agent_settled") {
		t.Error("extension does not subscribe to agent_settled")
	}
	if strings.Contains(content, `"agent_end"`) {
		t.Error("extension subscribes to agent_end, which fires before pi has settled")
	}
}

func TestInstallPiExtensionIsIdempotent(t *testing.T) {
	home := t.TempDir()
	if err := installPiExtension(home, "/a/nagare-go"); err != nil {
		t.Fatal(err)
	}
	if err := installPiExtension(home, "/b/nagare-go"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(home, ".pi", "agent", "extensions", "nagare.ts"))
	if strings.Contains(string(data), "/a/nagare-go") {
		t.Error("re-running setup should replace the old binary path")
	}
}

func TestInstallOpenCodePlugin(t *testing.T) {
	home := t.TempDir()
	if err := installOpenCodePlugin(home, "/opt/bin/nagare-go"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(home, ".config", "opencode", "plugins", "nagare.js")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, `const NAGARE = "/opt/bin/nagare-go"`) {
		t.Error("nagare binary path not embedded")
	}
	if strings.Contains(content, "%q") || strings.Contains(content, "%%") {
		t.Error("generated plugin contains unexpanded format verbs")
	}
	// The events the plugin forwards must be ones nagare maps to a real state.
	for _, event := range []string{"session.idle", "permission.asked", "session.status"} {
		if !strings.Contains(content, `"`+event+`"`) {
			t.Errorf("plugin does not forward %q", event)
		}
		if state := hooks.EventToState(event, ""); state == "unknown" {
			t.Errorf("forwarded event %q is not mapped by EventToState", event)
		}
	}
}

// OpenCode reads ~/.config/opencode/opencode.json; config.json is the old name
// that current versions ignore.
func TestRegisterMCPLocalOpenCodePath(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := registerMCPLocal(path, "/opt/bin/nagare-go"); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadJSON(path)
	if err != nil {
		t.Fatal(err)
	}
	mcpMap, ok := cfg["mcp"].(map[string]interface{})
	if !ok {
		t.Fatal("mcp key missing")
	}
	entry, ok := mcpMap["nagare"].(map[string]interface{})
	if !ok {
		t.Fatal("nagare server missing")
	}
	if entry["type"] != "local" {
		t.Errorf("type = %v, want local", entry["type"])
	}
	if entry["enabled"] != true {
		t.Errorf("enabled = %v, want true", entry["enabled"])
	}
}
