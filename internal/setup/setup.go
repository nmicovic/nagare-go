package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	fmt.Println("\nSetup complete!")
	return nil
}

func installClaudeHooks(home string) error {
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	// Find our binary
	bin := findBinary()
	hookCmd := bin + " hook-state"

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

	// Install hooks for standard events
	hookEntry := map[string]interface{}{
		"type":    "command",
		"command": hookCmd,
		"timeout": 5,
	}

	for _, event := range hookEvents {
		hooksMap[event] = appendHookEntry(hooksMap[event], hookEntry)
	}

	// Notification event has a matcher
	notifEntry := map[string]interface{}{
		"matcher": notificationMatcher,
		"hooks": []interface{}{
			hookEntry,
		},
	}
	hooksMap["Notification"] = appendHookEntry(
		removeNagareHooks(hooksMap["Notification"], "nagare-go hook-state"),
		notifEntry,
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

// findBinary locates the nagare-go binary path.
func findBinary() string {
	// Try to find in PATH
	if path, err := exec.LookPath("nagare-go"); err == nil {
		return path
	}
	// Try the binary next to the running executable
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "nagare-go"
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
