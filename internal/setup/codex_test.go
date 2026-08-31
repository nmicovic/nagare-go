package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nemke/nagare-go/internal/hooks"
)

func TestInstallCodexHooks_NewFile(t *testing.T) {
	home := t.TempDir()
	if err := installCodexHooks(home, "/opt/bin/nagare-go"); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadJSON(filepath.Join(home, ".codex", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	hooksMap, ok := cfg["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("hooks key missing")
	}

	for _, event := range codexHookEvents {
		arr, ok := hooksMap[event].([]interface{})
		if !ok || len(arr) != 1 {
			t.Fatalf("event %q missing or not a single group: %v", event, hooksMap[event])
		}
		group := arr[0].(map[string]interface{})
		entries, ok := group["hooks"].([]interface{})
		if !ok || len(entries) != 1 {
			t.Fatalf("event %q has no hook entry", event)
		}
		entry := entries[0].(map[string]interface{})
		if entry["command"] != "/opt/bin/nagare-go hook-state" {
			t.Errorf("event %q command = %v", event, entry["command"])
		}
		// Codex reads "timeout" here and ignores "timeoutSec" without warning,
		// leaving a hung hook to sit on its 600s default.
		if entry["timeout"] == nil {
			t.Errorf("event %q has no timeout", event)
		}
	}
}

// Every event nagare subscribes to must map to a real state, or the hook fires
// and reports "unknown".
func TestCodexHookEventsAreMapped(t *testing.T) {
	for _, event := range codexHookEvents {
		if state := hooks.EventToState(event, ""); state == "unknown" {
			t.Errorf("event %q is not mapped by EventToState", event)
		}
	}
}

func TestInstallCodexHooks_PreservesAndRefreshes(t *testing.T) {
	home := t.TempDir()
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}
	existing := `{
	  "hooks": {
	    "Stop": [
	      {"hooks": [{"type": "command", "command": "my-own-hook"}]},
	      {"hooks": [{"type": "command", "command": "/old/path/nagare-go hook-state"}]}
	    ]
	  },
	  "unrelated": true
	}`
	if err := os.WriteFile(filepath.Join(codexDir, "hooks.json"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	if err := installCodexHooks(home, "/new/path/nagare-go"); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadJSON(filepath.Join(codexDir, "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg["unrelated"] != true {
		t.Error("unrelated keys should be preserved")
	}
	stop := cfg["hooks"].(map[string]interface{})["Stop"].([]interface{})
	if len(stop) != 2 {
		t.Fatalf("Stop should hold the user's hook plus one fresh nagare hook, got %d", len(stop))
	}
	first := stop[0].(map[string]interface{})["hooks"].([]interface{})[0].(map[string]interface{})
	if first["command"] != "my-own-hook" {
		t.Errorf("user hook should come first, got %v", first["command"])
	}
	second := stop[1].(map[string]interface{})["hooks"].([]interface{})[0].(map[string]interface{})
	if second["command"] != "/new/path/nagare-go hook-state" {
		t.Errorf("stale nagare hook should be replaced, got %v", second["command"])
	}
}

func TestRegisterMCPCodex_NewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex", "config.toml")
	if err := registerMCPCodex(path, "/opt/bin/nagare-go"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	want := "[mcp_servers.nagare]\ncommand = \"/opt/bin/nagare-go\"\nargs = [\"mcp\"]\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// Registering twice must leave a well-formed file, not one whose final newline
// went out with the table it replaced.
func TestRegisterMCPCodex_IsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	for range 3 {
		if err := registerMCPCodex(path, "/opt/bin/nagare-go"); err != nil {
			t.Fatal(err)
		}
	}
	got := readFile(t, path)
	want := "[mcp_servers.nagare]\ncommand = \"/opt/bin/nagare-go\"\nargs = [\"mcp\"]\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

// config.toml is hand-written: comments and per-project tables must survive.
func TestRegisterMCPCodex_PreservesExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	existing := "# my settings\napprovals_reviewer = \"auto_review\"\n\n[projects.\"/home/u/proj\"]\ntrust_level = \"trusted\"\n"
	if err := os.WriteFile(path, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	if err := registerMCPCodex(path, "/opt/bin/nagare-go"); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, path)
	for _, keep := range []string{"# my settings", `approvals_reviewer = "auto_review"`, `[projects."/home/u/proj"]`, `trust_level = "trusted"`} {
		if !strings.Contains(got, keep) {
			t.Errorf("lost %q from config.toml:\n%s", keep, got)
		}
	}
	if !strings.Contains(got, "[mcp_servers.nagare]") {
		t.Errorf("nagare not registered:\n%s", got)
	}
}

// Re-running setup must move the binary path, not add a second table — two
// [mcp_servers.nagare] headers make the file invalid TOML.
func TestRegisterMCPCodex_ReplacesOwnTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	existing := "model = \"gpt-5\"\n\n[mcp_servers.nagare]\ncommand = \"/old/nagare-go\"\nargs = [\"mcp\"]\n\n[mcp_servers.other]\ncommand = \"other\"\n"
	if err := os.WriteFile(path, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	if err := registerMCPCodex(path, "/new/nagare-go"); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, path)
	if n := strings.Count(got, "[mcp_servers.nagare]"); n != 1 {
		t.Errorf("expected exactly one nagare table, got %d:\n%s", n, got)
	}
	if strings.Contains(got, "/old/nagare-go") {
		t.Errorf("old path survived:\n%s", got)
	}
	if !strings.Contains(got, "[mcp_servers.other]") || !strings.Contains(got, `model = "gpt-5"`) {
		t.Errorf("other tables were disturbed:\n%s", got)
	}
	// The replacement must stay above the table that followed it.
	if strings.Index(got, "[mcp_servers.nagare]") > strings.Index(got, "[mcp_servers.other]") {
		t.Errorf("nagare table moved past its neighbour:\n%s", got)
	}
}

// A sub-table of nagare's own entry belongs to nagare and must be replaced with
// it; leaving [mcp_servers.nagare.env] behind would orphan it under whatever
// table happened to follow.
func TestRegisterMCPCodex_ReplacesSubTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	existing := "[mcp_servers.nagare]\ncommand = \"/old/nagare-go\"\n\n[mcp_servers.nagare.env]\nFOO = \"bar\"\n\n[tui]\ntheme = \"dark\"\n"
	if err := os.WriteFile(path, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	if err := registerMCPCodex(path, "/new/nagare-go"); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, path)
	if strings.Contains(got, "[mcp_servers.nagare.env]") || strings.Contains(got, `FOO = "bar"`) {
		t.Errorf("nagare sub-table survived:\n%s", got)
	}
	if !strings.Contains(got, "[tui]") {
		t.Errorf("unrelated table lost:\n%s", got)
	}
}

// A quoted key is the same table as an unquoted one, so it must be replaced
// rather than duplicated.
func TestRegisterMCPCodex_ReplacesQuotedTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[mcp_servers.\"nagare\"]\ncommand = \"/old/nagare-go\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := registerMCPCodex(path, "/new/nagare-go"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if strings.Contains(got, "/old/nagare-go") {
		t.Errorf("quoted table not recognised as nagare's:\n%s", got)
	}
}

func TestInstallCodexSkill(t *testing.T) {
	home := t.TempDir()
	installCodexSkill(home)

	got := readFile(t, filepath.Join(home, ".codex", "skills", "nagare", "SKILL.md"))
	// Codex needs front matter with a name matching the directory.
	if !strings.HasPrefix(got, "---\nname: nagare\ndescription: ") {
		t.Errorf("skill is missing its front matter:\n%s", got)
	}
	if !strings.Contains(got, "list_agents()") {
		t.Error("skill does not document the messaging tools")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
