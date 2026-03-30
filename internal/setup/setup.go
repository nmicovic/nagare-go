package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nemke/nagare-go/internal/bin"
)

var hookEvents = []string{
	"UserPromptSubmit",
	"Stop",
	"PreToolUse",
	"SessionStart",
	"SessionEnd",
}

// notificationEvent has a matcher, handled separately.
const notificationMatcher = "idle_prompt|permission_prompt|elicitation_dialog"

// Run installs nagare-go hooks into Claude Code settings.
func Run() error {
	// Ensure data directory exists
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	dataDir := filepath.Join(home, ".local", "share", "nagare")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("cannot create data directory: %w", err)
	}
	fmt.Printf("  Data directory: %s\n", dataDir)

	// Ensure states directory exists
	statesDir := filepath.Join(dataDir, "states")
	if err := os.MkdirAll(statesDir, 0755); err != nil {
		return fmt.Errorf("cannot create states directory: %w", err)
	}

	// Install hooks
	if err := installClaudeHooks(home); err != nil {
		return fmt.Errorf("failed to install hooks: %w", err)
	}

	// Register MCP server in all supported agents
	nagareBin := bin.FindSelf()

	// Claude Code + Gemini CLI use standard mcpServers format
	for _, mc := range []struct{ name, path string }{
		{"Claude Code", filepath.Join(home, ".claude.json")},
		{"Gemini CLI", filepath.Join(home, ".gemini", "settings.json")},
	} {
		if err := registerMCPStandard(mc.path, nagareBin); err != nil {
			fmt.Printf("  MCP server: %s — skipped (%v)\n", mc.name, err)
		} else {
			fmt.Printf("  MCP server: %s — %s\n", mc.name, mc.path)
		}
	}

	// OpenCode uses its own format
	ocPath := filepath.Join(home, ".config", "opencode", "config.json")
	if err := registerMCPOpenCode(ocPath, nagareBin); err != nil {
		fmt.Printf("  MCP server: OpenCode — skipped (%v)\n", err)
	} else {
		fmt.Printf("  MCP server: OpenCode — %s\n", ocPath)
	}

	// Install slash commands for all supported agent CLIs
	installCommands(home)

	fmt.Println("\nSetup complete!")
	return nil
}

// registerMCPStandard adds nagare to the standard mcpServers format
// used by Claude Code, Gemini CLI, Cursor, etc.
func registerMCPStandard(configPath, nagareBin string) error {
	cfg, err := loadJSON(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot read %s: %w", configPath, err)
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	servers, _ := cfg["mcpServers"].(map[string]interface{})
	if servers == nil {
		servers = make(map[string]interface{})
	}

	servers["nagare"] = map[string]interface{}{
		"command": nagareBin,
		"args":    []string{"mcp"},
	}
	cfg["mcpServers"] = servers

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0644)
}

// registerMCPOpenCode adds nagare to OpenCode's config format.
// OpenCode uses "mcp" key with type/command array format.
func registerMCPOpenCode(configPath, nagareBin string) error {
	cfg, err := loadJSON(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot read %s: %w", configPath, err)
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	mcpMap, _ := cfg["mcp"].(map[string]interface{})
	if mcpMap == nil {
		mcpMap = make(map[string]interface{})
	}

	mcpMap["nagare"] = map[string]interface{}{
		"type":    "local",
		"command": []string{nagareBin, "mcp"},
		"enabled": true,
	}
	cfg["mcp"] = mcpMap

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0644)
}

func installClaudeHooks(home string) error {
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	// Find our binary
	nagareBin := bin.FindSelf()
	hookCmd := nagareBin + " hook-state"

	// Load existing settings
	settings, err := loadJSON(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot read %s: %w", settingsPath, err)
	}
	if settings == nil {
		settings = make(map[string]interface{})
	}

	// Get or create hooks map
	hooksMap, _ := settings["hooks"].(map[string]interface{})
	if hooksMap == nil {
		hooksMap = make(map[string]interface{})
	}

	// Remove stale nagare hooks from all events
	for event := range hooksMap {
		hooksMap[event] = removeNagareHooks(hooksMap[event], "nagare-go hook-state")
		// Also remove old Python nagare hooks
		hooksMap[event] = removeNagareHooks(hooksMap[event], "nagare hook-state")
	}

	// Hook command entry
	hookEntry := map[string]interface{}{
		"type":    "command",
		"command": hookCmd,
		"timeout": 5,
	}

	// Standard events: matcher="" matches all
	for _, event := range hookEvents {
		hooksMap[event] = appendHookEntry(hooksMap[event], map[string]interface{}{
			"matcher": "",
			"hooks":   []interface{}{hookEntry},
		})
	}

	// Notification event has a specific matcher
	hooksMap["Notification"] = appendHookEntry(
		removeNagareHooks(hooksMap["Notification"], "nagare-go hook-state"),
		map[string]interface{}{
			"matcher": notificationMatcher,
			"hooks":   []interface{}{hookEntry},
		},
	)

	settings["hooks"] = hooksMap

	// Write back
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		return err
	}

	fmt.Printf("  Hooks installed: %s\n", settingsPath)
	fmt.Printf("  Command: %s\n", hookCmd)
	fmt.Printf("  Events: %s, Notification\n", strings.Join(hookEvents, ", "))
	return nil
}

// loadJSON reads a JSON file into a map.
func loadJSON(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", path, err)
	}
	return result, nil
}

// removeNagareHooks filters out hook entries containing the given command substring.
func removeNagareHooks(eventVal interface{}, cmdSubstr string) interface{} {
	arr, ok := eventVal.([]interface{})
	if !ok {
		return eventVal
	}
	var kept []interface{}
	for _, item := range arr {
		if containsCommand(item, cmdSubstr) {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

// containsCommand checks if a hook entry or group contains a command with the given substring.
func containsCommand(item interface{}, substr string) bool {
	m, ok := item.(map[string]interface{})
	if !ok {
		return false
	}
	// Direct hook entry: {"type": "command", "command": "..."}
	if cmd, ok := m["command"].(string); ok {
		if strings.Contains(cmd, substr) {
			return true
		}
	}
	// Hook group with nested hooks array
	if hooks, ok := m["hooks"].([]interface{}); ok {
		for _, h := range hooks {
			if containsCommand(h, substr) {
				return true
			}
		}
	}
	return false
}

// appendHookEntry appends a hook entry to an event's array.
func appendHookEntry(eventVal interface{}, entry map[string]interface{}) []interface{} {
	arr, _ := eventVal.([]interface{})
	return append(arr, entry)
}
