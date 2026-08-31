package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Codex hook events nagare subscribes to. Codex writes command hooks the same
// way Claude Code does — a JSON object on stdin carrying hook_event_name,
// session_id, cwd and last_assistant_message — so the names below are the ones
// hooks.EventToState already maps and no per-agent translation is needed.
//
// Codex has no Notification or Elicitation event, so a permission prompt is
// reported once, by PermissionRequest. PostToolUse is left out for the same
// reason as in Claude: it doubles the process spawns per turn without adding
// state PreToolUse has not already reported.
var codexHookEvents = []string{
	"UserPromptSubmit",
	"PreToolUse",
	"PermissionRequest",
	"Stop",
	"SessionStart",
	"SessionEnd",
}

// codexHookCommand is the substring identifying nagare's own entries in a
// hooks.json that may also hold the user's hooks.
const codexHookCommand = "nagare-go hook-state"

// installCodexHooks writes nagare's status hooks into ~/.codex/hooks.json.
//
// Codex will not run a new or changed hook until the user has trusted it: the
// TUI offers that review on the next start. Setup says so rather than leaving
// the user with hooks that look installed and never fire.
func installCodexHooks(home, nagareBin string) error {
	hooksPath := filepath.Join(home, ".codex", "hooks.json")
	hookCmd := nagareBin + " hook-state"

	cfg, err := loadJSON(hooksPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot read %s: %w", hooksPath, err)
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	hooksMap, _ := cfg["hooks"].(map[string]interface{})
	if hooksMap == nil {
		hooksMap = make(map[string]interface{})
	}

	// Drop stale nagare entries — including ones left by an earlier install at
	// a different binary path — before adding the current ones.
	for event := range hooksMap {
		hooksMap[event] = removeNagareHooks(hooksMap[event], codexHookCommand)
	}

	// "timeout", in seconds, as in Claude Code's settings.json — Codex reports
	// the field back as "timeoutSec" but ignores that spelling in the file, and
	// silently falls back to its 600s default.
	hookEntry := map[string]interface{}{
		"type":    "command",
		"command": hookCmd,
		"timeout": 5,
	}

	for _, event := range codexHookEvents {
		hooksMap[event] = appendHookEntry(hooksMap[event], map[string]interface{}{
			"hooks": []interface{}{hookEntry},
		})
	}

	cfg["hooks"] = hooksMap

	if err := os.MkdirAll(filepath.Dir(hooksPath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(hooksPath, data, 0644); err != nil {
		return err
	}

	fmt.Printf("  Hooks installed: Codex — %s\n", hooksPath)
	fmt.Printf("  Events: %s\n", strings.Join(codexHookEvents, ", "))
	fmt.Println("  Codex asks you to review new hooks on its next start — trust them there, or with /hooks.")
	return nil
}

// registerMCPCodex adds nagare to the [mcp_servers] table of Codex's
// config.toml.
//
// The file is edited as text instead of being decoded into a map and
// re-encoded: config.toml is hand-written and holds comments and per-project
// tables, all of which a round trip through a generic map would drop or
// reorder. Only nagare's own table is rewritten.
func registerMCPCodex(configPath, nagareBin string) error {
	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot read %s: %w", configPath, err)
	}

	block := fmt.Sprintf("[mcp_servers.nagare]\ncommand = %q\nargs = [\"mcp\"]\n", nagareBin)
	out := replaceTOMLTable(string(data), "mcp_servers.nagare", block)

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(configPath, []byte(out), 0644)
}

// tomlTableHeader returns the dotted name of the table a line declares, if it
// declares one. Array-of-table headers ("[[x]]") are not tables and never match.
func tomlTableHeader(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "[") || strings.HasPrefix(line, "[[") {
		return "", false
	}
	end := strings.LastIndex(line, "]")
	if end <= 0 {
		return "", false
	}
	return strings.TrimSpace(line[1:end]), true
}

// tableMatches reports whether header names the table want or one of its
// sub-tables. Quotes around a key are optional in TOML, so they are stripped
// before comparing — a path used as a key ([projects."/home/x"]) loses its
// quotes too, but cannot collide with the dotted names this is asked about.
func tableMatches(header, want string) bool {
	header = strings.NewReplacer(`"`, "", `'`, "", " ", "").Replace(header)
	return header == want || strings.HasPrefix(header, want+".")
}

// replaceTOMLTable returns doc with the table named want, and any sub-tables of
// it, replaced by block. When the table is absent, block is appended.
func replaceTOMLTable(doc, want, block string) string {
	lines := strings.Split(doc, "\n")

	var kept []string
	insertAt := -1
	inTarget := false
	for _, line := range lines {
		if header, ok := tomlTableHeader(line); ok {
			inTarget = tableMatches(header, want)
			if inTarget && insertAt < 0 {
				// Keep the blank line above the old table for the new one.
				insertAt = len(kept)
			}
		}
		if inTarget {
			continue
		}
		kept = append(kept, line)
	}

	newLines := strings.Split(strings.TrimRight(block, "\n"), "\n")
	if insertAt < 0 {
		kept = trimTrailingBlanks(kept)
		if len(kept) > 0 {
			kept = append(kept, "")
		}
		kept = append(kept, newLines...)
	} else {
		rest := append([]string{}, kept[insertAt:]...)
		kept = append(kept[:insertAt], newLines...)
		kept = append(kept, rest...)
	}
	// Exactly one trailing newline, whichever branch ran: dropping a table at
	// the end of the file takes the file's own final newline with it.
	return strings.TrimRight(strings.Join(kept, "\n"), "\n") + "\n"
}

func trimTrailingBlanks(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// installCodexSkill writes the nagare messaging skill to
// ~/.codex/skills/nagare/SKILL.md. Codex discovers skills there for every
// project; unlike Claude Code and OpenCode it has no user-level slash commands
// to install, so it gets a skill, as Crush does.
func installCodexSkill(home string) {
	dir := filepath.Join(home, ".codex", "skills", "nagare")
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("  Skill: Codex — skipped (%v)\n", err)
		return
	}
	// Codex requires front matter naming the skill; the name must match the
	// directory, and the description is what it matches a request against.
	skill := "---\nname: nagare\ndescription: " +
		"Communicate with other AI agent sessions — list them, send messages, and check the inbox.\n" +
		"---\n\n" + nagareSkillBody
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(skill), 0644); err != nil {
		fmt.Printf("  Skill: Codex — skipped (%v)\n", err)
		return
	}
	fmt.Printf("  Skill: Codex — %s\n", dir)
}
