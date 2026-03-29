# nagare-go Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite nagare's core loop in Go for ~5ms startup and single-binary distribution.

**Architecture:** Single binary with cobra subcommands (`pick`, `hook-state`, `setup`). All code in `internal/` packages. Bubble Tea for TUI. State files compatible with Python version.

**Tech Stack:** Go 1.23+, cobra, bubbletea, bubbles, lipgloss, BurntSushi/toml

---

## File Structure

```
nagare-go/
├── go.mod
├── go.sum
├── main.go                         # cobra root + subcommands
├── internal/
│   ├── config/
│   │   ├── config.go               # TOML loading, Config structs
│   │   └── config_test.go
│   ├── models/
│   │   ├── models.go               # Session, SessionStatus, AgentType, icons
│   │   └── models_test.go
│   ├── tmux/
│   │   ├── tmux.go                 # runTmux helper
│   │   ├── scanner.go              # list-panes parsing, agent detection
│   │   ├── scanner_test.go
│   │   ├── status.go               # pane content scraping, regex patterns
│   │   └── status_test.go
│   ├── state/
│   │   ├── state.go                # state file read/write, conflict resolution
│   │   ├── state_test.go
│   │   ├── registry.go             # session registry JSON
│   │   └── registry_test.go
│   ├── hooks/
│   │   ├── hooks.go                # stdin JSON handler, event mapping, notify dispatch
│   │   └── hooks_test.go
│   ├── notifications/
│   │   ├── deliver.go              # toast, bell, os_notify
│   │   ├── deliver_test.go
│   │   ├── store.go                # notification persistence
│   │   └── store_test.go
│   ├── picker/
│   │   ├── picker.go               # Bubble Tea Model, Update, View
│   │   ├── keys.go                 # key bindings
│   │   ├── styles.go               # lipgloss styles, themes
│   │   └── preview.go              # tmux pane preview
│   └── mcp/
│       └── mcp.go                  # interface stubs
```

---

### Task 1: Project Scaffolding + CLI Skeleton

**Files:**
- Create: `go.mod`
- Create: `main.go`

- [ ] **Step 1: Initialize Go module**

Run:
```bash
cd /home/nemke/Hobby/nagare-go
go mod init github.com/nemke/nagare-go
```

Expected: `go.mod` created with module path.

- [ ] **Step 2: Install dependencies**

Run:
```bash
go get github.com/spf13/cobra@latest
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/BurntSushi/toml@latest
```

- [ ] **Step 3: Write main.go with cobra subcommands**

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "nagare-go",
		Short: "tmux session manager for AI coding agents",
	}

	pickCmd := &cobra.Command{
		Use:   "pick",
		Short: "Launch session picker TUI",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("picker: not implemented yet")
		},
	}

	hookStateCmd := &cobra.Command{
		Use:   "hook-state",
		Short: "Handle Claude Code hook events from stdin",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("hook-state: not implemented yet")
		},
	}

	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Install hooks to ~/.claude/settings.json",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("setup: not implemented yet")
		},
	}

	rootCmd.AddCommand(pickCmd, hookStateCmd, setupCmd)

	// Default to "pick" when no subcommand given
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return pickCmd.RunE(pickCmd, args)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Build and verify**

Run:
```bash
go build -o nagare-go . && ./nagare-go --help && ./nagare-go hook-state
```

Expected: Help text showing three subcommands. `hook-state` prints placeholder.

- [ ] **Step 5: Commit**

```bash
git init
git add go.mod go.sum main.go .claude/
git commit -m "feat: project scaffolding with cobra CLI skeleton"
```

---

### Task 2: Models Package

**Files:**
- Create: `internal/models/models.go`
- Create: `internal/models/models_test.go`

- [ ] **Step 1: Write the test**

```go
package models

import "testing"

func TestSessionStatusString(t *testing.T) {
	tests := []struct {
		status SessionStatus
		want   string
	}{
		{StatusWaitingInput, "waiting_input"},
		{StatusRunning, "running"},
		{StatusIdle, "idle"},
		{StatusDead, "dead"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("SessionStatus = %q, want %q", tt.status, tt.want)
		}
	}
}

func TestAgentTypeString(t *testing.T) {
	tests := []struct {
		agent AgentType
		want  string
	}{
		{AgentClaude, "claude"},
		{AgentOpenCode, "opencode"},
		{AgentGemini, "gemini"},
		{AgentUnknown, "unknown"},
	}
	for _, tt := range tests {
		if string(tt.agent) != tt.want {
			t.Errorf("AgentType = %q, want %q", tt.agent, tt.want)
		}
	}
}

func TestStatusLabel(t *testing.T) {
	if got := StatusLabel(StatusIdle); got != "Idle" {
		t.Errorf("StatusLabel(Idle) = %q, want %q", got, "Idle")
	}
	if got := StatusLabel(StatusWaitingInput); got != "Waiting for input" {
		t.Errorf("StatusLabel(WaitingInput) = %q, want %q", got, "Waiting for input")
	}
}

func TestAgentLabel(t *testing.T) {
	if got := AgentLabel(AgentClaude); got != "Claude" {
		t.Errorf("AgentLabel(Claude) = %q, want %q", got, "Claude")
	}
}

func TestStatusColor(t *testing.T) {
	if got := StatusColor(StatusIdle); got != "#00D26A" {
		t.Errorf("StatusColor(Idle) = %q, want %q", got, "#00D26A")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/models/ -v`
Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Write models.go**

```go
package models

// SessionStatus represents the current state of an agent session.
type SessionStatus string

const (
	StatusWaitingInput SessionStatus = "waiting_input"
	StatusRunning      SessionStatus = "running"
	StatusIdle         SessionStatus = "idle"
	StatusDead         SessionStatus = "dead"
)

// AgentType represents which AI coding agent is running.
type AgentType string

const (
	AgentClaude   AgentType = "claude"
	AgentOpenCode AgentType = "opencode"
	AgentGemini   AgentType = "gemini"
	AgentUnknown  AgentType = "unknown"
)

// SessionDetails holds metadata extracted from pane content.
type SessionDetails struct {
	GitBranch    string
	Model        string
	ContextUsage string
}

// Session represents a discovered agent pane in tmux.
type Session struct {
	Name        string
	SessionID   string
	Path        string
	WindowIndex int
	PaneIndex   int
	Status      SessionStatus
	AgentType   AgentType
	Details     SessionDetails
	LastMessage string
}

// SessionState is the JSON-serializable hook state written to disk.
type SessionState struct {
	State            string `json:"state"`
	SessionID        string `json:"session_id"`
	Cwd              string `json:"cwd"`
	Event            string `json:"event"`
	NotificationType string `json:"notification_type,omitempty"`
	LastMessage      string `json:"last_message,omitempty"`
	Timestamp        string `json:"timestamp"`
}

// StatusColor returns the hex color for a status (tokyonight palette).
func StatusColor(s SessionStatus) string {
	switch s {
	case StatusWaitingInput:
		return "#db4b4b"
	case StatusRunning:
		return "#e0af68"
	case StatusIdle:
		return "#00D26A"
	case StatusDead:
		return "#565f89"
	default:
		return "#565f89"
	}
}

// StatusLabel returns the human-readable label for a status.
func StatusLabel(s SessionStatus) string {
	switch s {
	case StatusWaitingInput:
		return "Waiting for input"
	case StatusRunning:
		return "Working"
	case StatusIdle:
		return "Idle"
	case StatusDead:
		return "Exited"
	default:
		return "Unknown"
	}
}

// AgentLabel returns the human-readable label for an agent type.
func AgentLabel(a AgentType) string {
	switch a {
	case AgentClaude:
		return "Claude"
	case AgentOpenCode:
		return "OpenCode"
	case AgentGemini:
		return "Gemini"
	default:
		return "Unknown"
	}
}

// AgentColor returns the foreground hex color for an agent type.
func AgentColor(a AgentType) string {
	switch a {
	case AgentClaude:
		return "#da7756"
	case AgentOpenCode:
		return "#00e5ff"
	case AgentGemini:
		return "#4285f4"
	default:
		return "#565f89"
	}
}

// AgentBgColor returns the background hex color for an agent type.
func AgentBgColor(a AgentType) string {
	switch a {
	case AgentClaude:
		return "#3b2820"
	case AgentOpenCode:
		return "#002b33"
	case AgentGemini:
		return "#1a2744"
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/models/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/models/
git commit -m "feat: add models package with Session, Status, AgentType"
```

---

### Task 3: Config Package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the test**

```go
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

func TestLoadMissingFile(t *testing.T) {
	cfg, err := LoadFrom("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatal("missing file should return defaults, not error")
	}
	if cfg.Appearance.Theme != "tokyonight" {
		t.Errorf("theme = %q, want default %q", cfg.Appearance.Theme, "tokyonight")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL

- [ ] **Step 3: Write config.go**

```go
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config package with TOML loading and defaults"
```

---

### Task 4: Tmux Helper + Status Detection

**Files:**
- Create: `internal/tmux/tmux.go`
- Create: `internal/tmux/status.go`
- Create: `internal/tmux/status_test.go`

- [ ] **Step 1: Write the test for status detection**

```go
package tmux

import "testing"

func TestDetectStatus_Empty(t *testing.T) {
	if got := DetectStatus(""); got != "dead" {
		t.Errorf("empty content: got %q, want %q", got, "dead")
	}
}

func TestDetectStatus_BarePrompt(t *testing.T) {
	content := "some output\n❯\n"
	if got := DetectStatus(content); got != "idle" {
		t.Errorf("bare prompt: got %q, want %q", got, "idle")
	}
}

func TestDetectStatus_ChoicePrompt(t *testing.T) {
	content := "some output\n❯ 1. Yes\n❯ 2. No\n"
	if got := DetectStatus(content); got != "waiting_input" {
		t.Errorf("choice prompt: got %q, want %q", got, "waiting_input")
	}
}

func TestDetectStatus_DoYouWant(t *testing.T) {
	content := "Do you want to proceed?\n"
	if got := DetectStatus(content); got != "waiting_input" {
		t.Errorf("do you want: got %q, want %q", got, "waiting_input")
	}
}

func TestDetectStatus_EscToCancel(t *testing.T) {
	content := "Choose an option\nEsc to cancel\n"
	if got := DetectStatus(content); got != "waiting_input" {
		t.Errorf("esc to cancel: got %q, want %q", got, "waiting_input")
	}
}

func TestDetectStatus_Spinner(t *testing.T) {
	content := "Processing ⠙ loading...\n"
	if got := DetectStatus(content); got != "running" {
		t.Errorf("spinner: got %q, want %q", got, "running")
	}
}

func TestDetectStatus_RunningTag(t *testing.T) {
	content := "something (running) here\n"
	if got := DetectStatus(content); got != "running" {
		t.Errorf("running tag: got %q, want %q", got, "running")
	}
}

func TestDetectStatus_FastForward(t *testing.T) {
	content := "status bar ⏵⏵ active\n"
	if got := DetectStatus(content); got != "running" {
		t.Errorf("fast-forward: got %q, want %q", got, "running")
	}
}

func TestParseDetails_WithStatusBar(t *testing.T) {
	content := "line1\nline2\nuser@host:/path (git:main) | Opus 4.6 | ctx:42%\n"
	details := ParseDetails(content)
	if details.GitBranch != "main" {
		t.Errorf("git branch = %q, want %q", details.GitBranch, "main")
	}
	if details.Model != "Opus 4.6" {
		t.Errorf("model = %q, want %q", details.Model, "Opus 4.6")
	}
	if details.ContextUsage != "42%" {
		t.Errorf("context = %q, want %q", details.ContextUsage, "42%")
	}
}

func TestParseDetails_NoStatusBar(t *testing.T) {
	content := "just some normal output\n"
	details := ParseDetails(content)
	if details.GitBranch != "" {
		t.Errorf("git branch should be empty, got %q", details.GitBranch)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tmux/ -v`
Expected: FAIL

- [ ] **Step 3: Write tmux.go (helper)**

```go
package tmux

import (
	"os/exec"
	"strings"
)

// RunTmux runs a tmux command and returns stdout. Returns empty string on error.
func RunTmux(args ...string) string {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}
```

- [ ] **Step 4: Write status.go**

```go
package tmux

import (
	"regexp"
	"strings"

	"github.com/nemke/nagare-go/internal/models"
)

var (
	// Bare ❯ prompt on its own line
	waitingPromptRe = regexp.MustCompile(`(?m)^❯\s*$`)

	// Choice/confirmation prompts
	waitingChoicePatterns = []*regexp.Regexp{
		regexp.MustCompile(`❯\s+\d+\.\s+(Yes|No)`),
		regexp.MustCompile(`Do you want to`),
		regexp.MustCompile(`Esc to cancel`),
	}

	// Running indicators
	runningPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\(running\)`),
		regexp.MustCompile(`[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏⠐⠂]`),
	}

	// Status bar: (git:branch) | Model | ctx:NN%
	statusBarRe = regexp.MustCompile(`\(git:(?P<branch>[^)]+)\)\s*\|\s*(?P<model>[^|]+?)\s*\|\s*ctx:(?P<ctx>\d+%)`)
)

// tail returns the last n lines of s.
func tail(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// DetectStatus determines session status by scraping pane content.
func DetectStatus(paneContent string) string {
	if strings.TrimSpace(paneContent) == "" {
		return string(models.StatusDead)
	}

	last15 := tail(paneContent, 15)

	// Choice/confirmation prompts → waiting_input
	for _, pat := range waitingChoicePatterns {
		if pat.MatchString(last15) {
			return string(models.StatusWaitingInput)
		}
	}

	// Running indicators
	for _, pat := range runningPatterns {
		if pat.MatchString(last15) {
			return string(models.StatusRunning)
		}
	}

	// Bare ❯ prompt → idle
	if waitingPromptRe.MatchString(last15) {
		return string(models.StatusIdle)
	}

	// Fast-forward status bar
	if strings.Contains(last15, "⏵⏵") {
		return string(models.StatusRunning)
	}

	return string(models.StatusIdle)
}

// ParseDetails extracts git branch, model, context usage from pane status bar.
func ParseDetails(paneContent string) models.SessionDetails {
	if paneContent == "" {
		return models.SessionDetails{}
	}

	last5 := tail(paneContent, 5)
	match := statusBarRe.FindStringSubmatch(last5)
	if match == nil {
		return models.SessionDetails{}
	}

	result := models.SessionDetails{}
	for i, name := range statusBarRe.SubexpNames() {
		if i == 0 {
			continue
		}
		switch name {
		case "branch":
			result.GitBranch = strings.TrimSpace(match[i])
		case "model":
			result.Model = strings.TrimSpace(match[i])
		case "ctx":
			result.ContextUsage = strings.TrimSpace(match[i])
		}
	}
	return result
}
```

Note: The test file uses `DetectStatus` returning a string. The implementation returns `string(models.StatusXxx)` so the test values match.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/tmux/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tmux/
git commit -m "feat: add tmux helper and status detection with pane scraping"
```

---

### Task 5: State Package

**Files:**
- Create: `internal/state/state.go`
- Create: `internal/state/state_test.go`

- [ ] **Step 1: Write the test**

```go
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nemke/nagare-go/internal/models"
)

func writeState(t *testing.T, dir string, filename string, s models.SessionState) {
	t.Helper()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadAllStates_Empty(t *testing.T) {
	dir := t.TempDir()
	states := LoadAllStates(dir)
	if len(states) != 0 {
		t.Errorf("expected 0 states, got %d", len(states))
	}
}

func TestLoadAllStates_SingleFile(t *testing.T) {
	dir := t.TempDir()
	writeState(t, dir, "abc.json", models.SessionState{
		State:     "idle",
		SessionID: "abc",
		Cwd:       "/home/user/project",
		Event:     "Stop",
		Timestamp: "2026-03-29T10:00:00Z",
	})

	states := LoadAllStates(dir)
	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}
	s, ok := states["/home/user/project"]
	if !ok {
		t.Fatal("expected state keyed by cwd")
	}
	if s.State != "idle" {
		t.Errorf("state = %q, want %q", s.State, "idle")
	}
}

func TestLoadAllStates_ConflictLiveOverDead(t *testing.T) {
	dir := t.TempDir()
	writeState(t, dir, "dead.json", models.SessionState{
		State:     "dead",
		SessionID: "dead-id",
		Cwd:       "/home/user/project",
		Event:     "SessionEnd",
		Timestamp: "2026-03-29T12:00:00Z",
	})
	writeState(t, dir, "live.json", models.SessionState{
		State:     "working",
		SessionID: "live-id",
		Cwd:       "/home/user/project",
		Event:     "UserPromptSubmit",
		Timestamp: "2026-03-29T10:00:00Z",
	})

	states := LoadAllStates(dir)
	s := states["/home/user/project"]
	if s.State != "working" {
		t.Errorf("live should beat dead: got %q", s.State)
	}
}

func TestLoadAllStates_ConflictNewerWins(t *testing.T) {
	dir := t.TempDir()
	writeState(t, dir, "old.json", models.SessionState{
		State:     "idle",
		SessionID: "old-id",
		Cwd:       "/home/user/project",
		Event:     "Stop",
		Timestamp: "2026-03-29T10:00:00Z",
	})
	writeState(t, dir, "new.json", models.SessionState{
		State:     "working",
		SessionID: "new-id",
		Cwd:       "/home/user/project",
		Event:     "UserPromptSubmit",
		Timestamp: "2026-03-29T12:00:00Z",
	})

	states := LoadAllStates(dir)
	s := states["/home/user/project"]
	if s.State != "working" {
		t.Errorf("newer should win: got %q", s.State)
	}
}

func TestWriteState(t *testing.T) {
	dir := t.TempDir()
	s := models.SessionState{
		State:     "idle",
		SessionID: "test-id",
		Cwd:       "/home/user/project",
		Event:     "Stop",
		Timestamp: "2026-03-29T10:00:00Z",
	}

	err := WriteState(dir, s)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "test-id.json"))
	if err != nil {
		t.Fatal(err)
	}

	var loaded models.SessionState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.State != "idle" {
		t.Errorf("state = %q, want %q", loaded.State, "idle")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/state/ -v`
Expected: FAIL

- [ ] **Step 3: Write state.go**

```go
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/nemke/nagare-go/internal/models"
)

// DefaultStatesDir returns the default states directory path.
func DefaultStatesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "nagare", "states")
}

// LoadAllStates loads all state files from dir, keyed by cwd.
// Conflict resolution: live beats dead, then newer timestamp wins.
func LoadAllStates(dir string) map[string]models.SessionState {
	states := make(map[string]models.SessionState)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return states
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var s models.SessionState
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}

		if s.Cwd == "" {
			continue
		}

		existing, exists := states[s.Cwd]
		if !exists {
			states[s.Cwd] = s
			continue
		}

		// Live beats dead
		if existing.State == "dead" && s.State != "dead" {
			states[s.Cwd] = s
		} else if existing.State != "dead" && s.State == "dead" {
			// Keep existing live state
		} else if s.Timestamp > existing.Timestamp {
			// Same liveness: newer wins
			states[s.Cwd] = s
		}
	}

	return states
}

// WriteState writes a session state to dir/{session_id}.json.
func WriteState(dir string, s models.SessionState) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	path := filepath.Join(dir, s.SessionID+".json")
	return os.WriteFile(path, data, 0644)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/state/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/state/state.go internal/state/state_test.go
git commit -m "feat: add state package with read/write and conflict resolution"
```

---

### Task 6: Registry Package

**Files:**
- Create: `internal/state/registry.go`
- Create: `internal/state/registry_test.go`

- [ ] **Step 1: Write the test**

```go
package state

import (
	"path/filepath"
	"testing"
)

func TestRegistryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	reg := NewRegistry(path)

	// Register a session
	reg.Register("my-session", "/home/user/project", "claude")

	sessions := reg.ListAll()
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Name != "my-session" {
		t.Errorf("name = %q, want %q", sessions[0].Name, "my-session")
	}
	if sessions[0].Agent != "claude" {
		t.Errorf("agent = %q, want %q", sessions[0].Agent, "claude")
	}

	// Find by name
	s := reg.Find("my-session")
	if s == nil {
		t.Fatal("Find returned nil")
	}

	// Find by path
	s = reg.FindByPath("/home/user/project")
	if s == nil {
		t.Fatal("FindByPath returned nil")
	}

	// Toggle star
	reg.ToggleStar("my-session")
	s = reg.Find("my-session")
	if !s.Starred {
		t.Error("session should be starred")
	}

	// Remove
	reg.Remove("my-session")
	if len(reg.ListAll()) != 0 {
		t.Error("session should be removed")
	}
}

func TestRegistryPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	reg1 := NewRegistry(path)
	reg1.Register("test", "/tmp/test", "opencode")

	// Load from same file
	reg2 := NewRegistry(path)
	sessions := reg2.ListAll()
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session after reload, got %d", len(sessions))
	}
	if sessions[0].Agent != "opencode" {
		t.Errorf("agent = %q, want %q", sessions[0].Agent, "opencode")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/state/ -v -run TestRegistry`
Expected: FAIL

- [ ] **Step 3: Write registry.go**

```go
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// RegisteredSession is a session tracked in the registry.
type RegisteredSession struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Agent        string `json:"agent"`
	LastAccessed string `json:"last_accessed"`
	Starred      bool   `json:"starred"`
}

// Registry manages the persistent session registry.
type Registry struct {
	path     string
	sessions []RegisteredSession
}

// DefaultRegistryPath returns the default registry file path.
func DefaultRegistryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "nagare", "sessions.json")
}

// NewRegistry loads or creates a registry at the given path.
func NewRegistry(path string) *Registry {
	r := &Registry{path: path}
	r.load()
	return r
}

func (r *Registry) load() {
	data, err := os.ReadFile(r.path)
	if err != nil {
		r.sessions = nil
		return
	}
	json.Unmarshal(data, &r.sessions)
}

func (r *Registry) save() {
	data, _ := json.MarshalIndent(r.sessions, "", "  ")
	os.MkdirAll(filepath.Dir(r.path), 0755)
	os.WriteFile(r.path, data, 0644)
}

// ListAll returns all registered sessions.
func (r *Registry) ListAll() []RegisteredSession {
	return r.sessions
}

// Find returns a session by name, or nil.
func (r *Registry) Find(name string) *RegisteredSession {
	for i := range r.sessions {
		if r.sessions[i].Name == name {
			return &r.sessions[i]
		}
	}
	return nil
}

// FindByPath returns a session by path, or nil.
func (r *Registry) FindByPath(path string) *RegisteredSession {
	for i := range r.sessions {
		if r.sessions[i].Path == path {
			return &r.sessions[i]
		}
	}
	return nil
}

// Register adds or updates a session. Saves to disk.
func (r *Registry) Register(name, path, agent string) {
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range r.sessions {
		if r.sessions[i].Name == name {
			r.sessions[i].Path = path
			r.sessions[i].Agent = agent
			r.sessions[i].LastAccessed = now
			r.save()
			return
		}
	}
	r.sessions = append(r.sessions, RegisteredSession{
		Name:         name,
		Path:         path,
		Agent:        agent,
		LastAccessed: now,
	})
	r.save()
}

// Remove deletes a session by name. Saves to disk.
func (r *Registry) Remove(name string) {
	for i := range r.sessions {
		if r.sessions[i].Name == name {
			r.sessions = append(r.sessions[:i], r.sessions[i+1:]...)
			r.save()
			return
		}
	}
}

// ToggleStar toggles the starred flag for a session. Saves to disk.
func (r *Registry) ToggleStar(name string) {
	for i := range r.sessions {
		if r.sessions[i].Name == name {
			r.sessions[i].Starred = !r.sessions[i].Starred
			r.save()
			return
		}
	}
}

// Touch updates the last_accessed timestamp. Saves to disk.
func (r *Registry) Touch(name string) {
	for i := range r.sessions {
		if r.sessions[i].Name == name {
			r.sessions[i].LastAccessed = time.Now().UTC().Format(time.RFC3339)
			r.save()
			return
		}
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/state/ -v`
Expected: PASS (all state + registry tests)

- [ ] **Step 5: Commit**

```bash
git add internal/state/registry.go internal/state/registry_test.go
git commit -m "feat: add session registry with persistence"
```

---

### Task 7: Scanner Package

**Files:**
- Create: `internal/tmux/scanner.go`
- Create: `internal/tmux/scanner_test.go`

- [ ] **Step 1: Write the test**

```go
package tmux

import (
	"testing"

	"github.com/nemke/nagare-go/internal/models"
)

func TestParseSessions(t *testing.T) {
	raw := "my-project:$0:/home/user/project\nother:$1:/tmp/other\n"
	sessions := ParseSessions(raw)
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].Name != "my-project" {
		t.Errorf("name = %q, want %q", sessions[0].Name, "my-project")
	}
	if sessions[0].SessionID != "$0" {
		t.Errorf("id = %q, want %q", sessions[0].SessionID, "$0")
	}
	if sessions[0].Path != "/home/user/project" {
		t.Errorf("path = %q, want %q", sessions[0].Path, "/home/user/project")
	}
}

func TestParseSessionsEmpty(t *testing.T) {
	sessions := ParseSessions("")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestParseAllPanes(t *testing.T) {
	raw := "my-project:0:0:claude:12345\nmy-project:0:1:zsh:12346\nother:0:0:opencode:12347\n"
	panes := ParseAllPanes(raw)

	myPanes, ok := panes["my-project"]
	if !ok {
		t.Fatal("expected my-project panes")
	}
	if len(myPanes) != 1 {
		t.Fatalf("expected 1 agent pane, got %d", len(myPanes))
	}
	if myPanes[0].AgentType != models.AgentClaude {
		t.Errorf("agent = %q, want %q", myPanes[0].AgentType, models.AgentClaude)
	}

	otherPanes := panes["other"]
	if len(otherPanes) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(otherPanes))
	}
	if otherPanes[0].AgentType != models.AgentOpenCode {
		t.Errorf("agent = %q, want %q", otherPanes[0].AgentType, models.AgentOpenCode)
	}
}

func TestParseAllPanes_IgnoresNonAgent(t *testing.T) {
	raw := "sess:0:0:zsh:12345\nsess:0:1:vim:12346\n"
	panes := ParseAllPanes(raw)
	if len(panes) != 0 {
		t.Errorf("expected 0 agent sessions, got %d", len(panes))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tmux/ -v -run TestParse`
Expected: FAIL

- [ ] **Step 3: Write scanner.go**

```go
package tmux

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nemke/nagare-go/internal/models"
)

// RawSession is a parsed tmux session from list-sessions.
type RawSession struct {
	Name      string
	SessionID string
	Path      string
}

// PaneInfo is a parsed agent pane from list-panes.
type PaneInfo struct {
	WindowIndex int
	PaneIndex   int
	AgentType   models.AgentType
}

var agentProcesses = map[string]models.AgentType{
	"claude":   models.AgentClaude,
	"opencode": models.AgentOpenCode,
}

// ParseSessions parses `tmux list-sessions -F "#{session_name}:#{session_id}:#{session_path}"`.
func ParseSessions(raw string) []RawSession {
	var sessions []RawSession
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		sessions = append(sessions, RawSession{
			Name:      parts[0],
			SessionID: parts[1],
			Path:      parts[2],
		})
	}
	return sessions
}

// ParseAllPanes parses `tmux list-panes -a -F "#{session_name}:#{window_index}:#{pane_index}:#{pane_current_command}:#{pane_pid}"`.
// Returns agent panes grouped by session name.
func ParseAllPanes(raw string) map[string][]PaneInfo {
	result := make(map[string][]PaneInfo)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 5)
		if len(parts) != 5 {
			continue
		}
		sessionName := parts[0]
		windowIdx, _ := strconv.Atoi(parts[1])
		paneIdx, _ := strconv.Atoi(parts[2])
		cmd := strings.TrimSpace(parts[3])
		pid := parts[4]

		agentType, ok := agentProcesses[cmd]
		if !ok && cmd == "node" {
			agentType, ok = resolveNodeAgent(pid)
			if !ok {
				continue
			}
		} else if !ok {
			continue
		}

		result[sessionName] = append(result[sessionName], PaneInfo{
			WindowIndex: windowIdx,
			PaneIndex:   paneIdx,
			AgentType:   agentType,
		})
	}
	return result
}

// resolveNodeAgent checks /proc to identify Gemini running under node.
func resolveNodeAgent(pid string) (models.AgentType, bool) {
	childrenPath := fmt.Sprintf("/proc/%s/task/%s/children", pid, pid)
	data, err := os.ReadFile(childrenPath)
	if err != nil {
		return "", false
	}
	for _, childPid := range strings.Fields(string(data)) {
		cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%s/cmdline", childPid))
		if err != nil {
			continue
		}
		args := strings.Split(string(cmdline), "\x00")
		for _, arg := range args {
			basename := arg
			if idx := strings.LastIndex(arg, "/"); idx >= 0 {
				basename = arg[idx+1:]
			}
			if basename == "gemini" {
				return models.AgentGemini, true
			}
		}
	}
	return "", false
}

// hookStateMap maps hook state strings to SessionStatus.
var hookStateMap = map[string]models.SessionStatus{
	"working":       models.StatusRunning,
	"waiting_input": models.StatusWaitingInput,
	"idle":          models.StatusIdle,
	"dead":          models.StatusDead,
}

// ScanSessions discovers all agent sessions in tmux.
func ScanSessions(statesDir string) []models.Session {
	// 1. Get all tmux sessions
	rawSessions := RunTmux("list-sessions", "-F", "#{session_name}:#{session_id}:#{session_path}")
	sessions := ParseSessions(rawSessions)

	// 2. Load hook state files
	hookStates := loadAllStatesImport(statesDir)

	// 3. Get all agent panes
	rawPanes := RunTmux("list-panes", "-a", "-F", "#{session_name}:#{window_index}:#{pane_index}:#{pane_current_command}:#{pane_pid}")
	allPanes := ParseAllPanes(rawPanes)

	// 4. Build Session objects
	var result []models.Session
	for _, sess := range sessions {
		panes, ok := allPanes[sess.Name]
		if !ok {
			continue
		}
		for _, pane := range panes {
			hookState, hasHook := hookStates[sess.Path]

			var status models.SessionStatus
			var lastMessage string
			var details models.SessionDetails

			if hasHook {
				s, ok := hookStateMap[hookState.State]
				if ok {
					status = s
				} else {
					status = models.StatusIdle
				}
				lastMessage = hookState.LastMessage
			} else {
				paneContent := RunTmux("capture-pane", "-t",
					fmt.Sprintf("%s:%d.%d", sess.Name, pane.WindowIndex, pane.PaneIndex), "-p")
				details = ParseDetails(paneContent)
				status = models.SessionStatus(DetectStatus(paneContent))
			}

			result = append(result, models.Session{
				Name:        sess.Name,
				SessionID:   sess.SessionID,
				Path:        sess.Path,
				WindowIndex: pane.WindowIndex,
				PaneIndex:   pane.PaneIndex,
				Status:      status,
				AgentType:   pane.AgentType,
				Details:     details,
				LastMessage: lastMessage,
			})
		}
	}
	return result
}

// loadAllStatesImport is a bridge to the state package to avoid import cycles.
// In the real wiring, ScanSessions receives pre-loaded states.
func loadAllStatesImport(statesDir string) map[string]models.SessionState {
	// This will be replaced with proper dependency injection in the picker.
	// For now, inline the logic to avoid circular imports.
	states := make(map[string]models.SessionState)
	entries, err := os.ReadDir(statesDir)
	if err != nil {
		return states
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(fmt.Sprintf("%s/%s", statesDir, entry.Name()))
		if err != nil {
			continue
		}
		var s models.SessionState
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		if s.Cwd == "" {
			continue
		}
		existing, exists := states[s.Cwd]
		if !exists {
			states[s.Cwd] = s
		} else if existing.State == "dead" && s.State != "dead" {
			states[s.Cwd] = s
		} else if existing.State != "dead" && s.State == "dead" {
			// keep live
		} else if s.Timestamp > existing.Timestamp {
			states[s.Cwd] = s
		}
	}
	return states
}
```

Wait — duplicating state loading is bad. Let me fix the architecture. The scanner should accept pre-loaded states.

**Revised scanner.go** — replace `ScanSessions` and remove `loadAllStatesImport`:

```go
// ScanSessions discovers all agent sessions in tmux.
// hookStates should be pre-loaded via state.LoadAllStates().
func ScanSessions(hookStates map[string]models.SessionState) []models.Session {
	rawSessions := RunTmux("list-sessions", "-F", "#{session_name}:#{session_id}:#{session_path}")
	sessions := ParseSessions(rawSessions)

	rawPanes := RunTmux("list-panes", "-a", "-F", "#{session_name}:#{window_index}:#{pane_index}:#{pane_current_command}:#{pane_pid}")
	allPanes := ParseAllPanes(rawPanes)

	var result []models.Session
	for _, sess := range sessions {
		panes, ok := allPanes[sess.Name]
		if !ok {
			continue
		}
		for _, pane := range panes {
			hookState, hasHook := hookStates[sess.Path]

			var status models.SessionStatus
			var lastMessage string
			var details models.SessionDetails

			if hasHook {
				s, ok := hookStateMap[hookState.State]
				if ok {
					status = s
				} else {
					status = models.StatusIdle
				}
				lastMessage = hookState.LastMessage
			} else {
				paneContent := RunTmux("capture-pane", "-t",
					fmt.Sprintf("%s:%d.%d", sess.Name, pane.WindowIndex, pane.PaneIndex), "-p")
				details = ParseDetails(paneContent)
				status = models.SessionStatus(DetectStatus(paneContent))
			}

			result = append(result, models.Session{
				Name:        sess.Name,
				SessionID:   sess.SessionID,
				Path:        sess.Path,
				WindowIndex: pane.WindowIndex,
				PaneIndex:   pane.PaneIndex,
				Status:      status,
				AgentType:   pane.AgentType,
				Details:     details,
				LastMessage: lastMessage,
			})
		}
	}
	return result
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tmux/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tmux/scanner.go internal/tmux/scanner_test.go
git commit -m "feat: add tmux scanner with session/pane parsing and agent detection"
```

---

### Task 8: Notification Delivery

**Files:**
- Create: `internal/notifications/deliver.go`
- Create: `internal/notifications/deliver_test.go`

- [ ] **Step 1: Write the test**

```go
package notifications

import (
	"runtime"
	"testing"
)

func TestDetectOsNotifyCmd_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only test")
	}
	// Just test that it doesn't panic and returns a result
	cmd := DetectOsNotifyCmd()
	// cmd may be nil if notify-send isn't installed — that's ok
	_ = cmd
}

func TestBuildToastMessage(t *testing.T) {
	msg := BuildToastMessage("my-session", "needs_input", "permission_prompt")
	if msg == "" {
		t.Error("message should not be empty")
	}
}

func TestBuildToastMessage_TaskComplete(t *testing.T) {
	msg := BuildToastMessage("my-session", "task_complete", "")
	if msg == "" {
		t.Error("message should not be empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/notifications/ -v`
Expected: FAIL

- [ ] **Step 3: Write deliver.go**

```go
package notifications

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/nemke/nagare-go/internal/tmux"
)

// BuildToastMessage creates a human-readable notification message.
func BuildToastMessage(sessionName, eventType, notificationType string) string {
	switch eventType {
	case "needs_input":
		if notificationType == "permission_prompt" {
			return fmt.Sprintf("🔴 %s needs permission", sessionName)
		}
		return fmt.Sprintf("🔴 %s needs input", sessionName)
	case "task_complete":
		return fmt.Sprintf("✅ %s finished", sessionName)
	default:
		return fmt.Sprintf("📢 %s: %s", sessionName, eventType)
	}
}

// SendToast sends a tmux status bar message.
func SendToast(message string, durationMs int) {
	// Find the active client
	client := tmux.RunTmux("list-clients", "-F", "#{client_name}")
	lines := strings.Split(client, "\n")
	if len(lines) == 0 || lines[0] == "" {
		return
	}
	tmux.RunTmux("display-message", "-t", lines[0], "-d", fmt.Sprintf("%d", durationMs), message)
}

// SendBell sends a terminal bell character.
func SendBell() {
	exec.Command("tmux", "run-shell", `printf '\a'`).Run()
}

// DetectOsNotifyCmd returns the best available OS notification command, or nil.
func DetectOsNotifyCmd() []string {
	// Check for WSL
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		if path, err := exec.LookPath("wsl-notify-send"); err == nil {
			return []string{path}
		}
		return nil
	}

	// Linux
	if path, err := exec.LookPath("notify-send"); err == nil {
		return []string{path}
	}

	return nil
}

// SendOsNotify sends a native OS notification.
func SendOsNotify(title, message string) {
	cmd := DetectOsNotifyCmd()
	if cmd == nil {
		return
	}
	args := append(cmd, title, message)
	exec.Command(args[0], args[1:]...).Start()
}

// Deliver dispatches notifications based on config flags.
func Deliver(sessionName, eventType, notificationType string, toast, bell, osNotify bool, durationMs int) {
	message := BuildToastMessage(sessionName, eventType, notificationType)

	if toast {
		SendToast(message, durationMs)
	}
	if bell {
		SendBell()
	}
	if osNotify {
		SendOsNotify("nagare", message)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/notifications/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/notifications/deliver.go internal/notifications/deliver_test.go
git commit -m "feat: add notification delivery (toast, bell, os_notify)"
```

---

### Task 9: Notification Store

**Files:**
- Create: `internal/notifications/store.go`
- Create: `internal/notifications/store_test.go`

- [ ] **Step 1: Write the test**

```go
package notifications

import (
	"path/filepath"
	"testing"
)

func TestStoreAddAndList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications.json")
	store := NewStore(path)

	store.Add("my-session", "task completed")
	store.Add("other-session", "needs input")

	all := store.ListAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(all))
	}
	// Newest first
	if all[0].SessionName != "other-session" {
		t.Errorf("newest first: got %q", all[0].SessionName)
	}
}

func TestStoreMarkRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications.json")
	store := NewStore(path)

	store.Add("test", "message")
	all := store.ListAll()
	id := all[0].ID

	store.MarkRead(id)
	all = store.ListAll()
	if !all[0].Read {
		t.Error("notification should be marked read")
	}
}

func TestStoreDismiss(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications.json")
	store := NewStore(path)

	store.Add("test", "message")
	all := store.ListAll()
	id := all[0].ID

	store.Dismiss(id)
	if len(store.ListAll()) != 0 {
		t.Error("notification should be dismissed")
	}
}

func TestStoreUnreadCount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications.json")
	store := NewStore(path)

	store.Add("a", "msg1")
	store.Add("b", "msg2")

	if store.UnreadCount() != 2 {
		t.Errorf("unread = %d, want 2", store.UnreadCount())
	}

	all := store.ListAll()
	store.MarkRead(all[0].ID)
	if store.UnreadCount() != 1 {
		t.Errorf("unread = %d, want 1", store.UnreadCount())
	}
}

func TestStorePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications.json")

	store1 := NewStore(path)
	store1.Add("test", "persisted")

	store2 := NewStore(path)
	if len(store2.ListAll()) != 1 {
		t.Error("notification should persist across loads")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/notifications/ -v -run TestStore`
Expected: FAIL

- [ ] **Step 3: Write store.go**

```go
package notifications

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

// Notification is a stored notification entry.
type Notification struct {
	ID          string `json:"id"`
	SessionName string `json:"session_name"`
	Message     string `json:"message"`
	Timestamp   string `json:"timestamp"`
	Read        bool   `json:"read"`
}

// Store manages persistent notifications.
type Store struct {
	path          string
	notifications []Notification
}

// DefaultStorePath returns the default notification store path.
func DefaultStorePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "nagare", "notifications.json")
}

// NewStore loads or creates a notification store.
func NewStore(path string) *Store {
	s := &Store{path: path}
	s.load()
	return s
}

func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		s.notifications = nil
		return
	}
	json.Unmarshal(data, &s.notifications)
}

func (s *Store) save() {
	data, _ := json.MarshalIndent(s.notifications, "", "  ")
	os.MkdirAll(filepath.Dir(s.path), 0755)
	os.WriteFile(s.path, data, 0644)
}

// Add appends a new notification.
func (s *Store) Add(sessionName, message string) {
	s.notifications = append(s.notifications, Notification{
		ID:          uuid.New().String(),
		SessionName: sessionName,
		Message:     message,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Read:        false,
	})
	s.save()
}

// ListAll returns notifications in reverse chronological order (newest first).
func (s *Store) ListAll() []Notification {
	sorted := make([]Notification, len(s.notifications))
	copy(sorted, s.notifications)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp > sorted[j].Timestamp
	})
	return sorted
}

// MarkRead marks a notification as read by ID.
func (s *Store) MarkRead(id string) {
	for i := range s.notifications {
		if s.notifications[i].ID == id {
			s.notifications[i].Read = true
			s.save()
			return
		}
	}
}

// Dismiss removes a notification by ID.
func (s *Store) Dismiss(id string) {
	for i := range s.notifications {
		if s.notifications[i].ID == id {
			s.notifications = append(s.notifications[:i], s.notifications[i+1:]...)
			s.save()
			return
		}
	}
}

// DismissAll removes all notifications.
func (s *Store) DismissAll() {
	s.notifications = nil
	s.save()
}

// UnreadCount returns the number of unread notifications.
func (s *Store) UnreadCount() int {
	count := 0
	for _, n := range s.notifications {
		if !n.Read {
			count++
		}
	}
	return count
}
```

Note: This introduces a new dependency. Add it before running tests:

Run:
```bash
go get github.com/google/uuid@latest
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/notifications/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/notifications/store.go internal/notifications/store_test.go
git commit -m "feat: add notification store with persistence"
```

---

### Task 10: Hooks Package

**Files:**
- Create: `internal/hooks/hooks.go`
- Create: `internal/hooks/hooks_test.go`

- [ ] **Step 1: Write the test**

```go
package hooks

import "testing"

func TestEventToState(t *testing.T) {
	tests := []struct {
		event string
		ntype string
		want  string
	}{
		{"UserPromptSubmit", "", "working"},
		{"PreToolUse", "", "working"},
		{"BeforeAgent", "", "working"},
		{"BeforeTool", "", "working"},
		{"AfterTool", "", "working"},
		{"Stop", "", "idle"},
		{"AfterAgent", "", "idle"},
		{"SessionEnd", "", "dead"},
		{"SessionStart", "", "idle"},
		{"Notification", "permission_prompt", "waiting_input"},
		{"Notification", "elicitation_dialog", "waiting_input"},
		{"Notification", "other", "idle"},
		{"UnknownEvent", "", "unknown"},
	}
	for _, tt := range tests {
		got := EventToState(tt.event, tt.ntype)
		if got != tt.want {
			t.Errorf("EventToState(%q, %q) = %q, want %q", tt.event, tt.ntype, got, tt.want)
		}
	}
}

func TestShouldNotify_NeedsInput(t *testing.T) {
	eventType, _ := ShouldNotify("waiting_input", "", 0)
	if eventType != "needs_input" {
		t.Errorf("expected needs_input, got %q", eventType)
	}
}

func TestShouldNotify_TaskComplete(t *testing.T) {
	eventType, _ := ShouldNotify("idle", "working", 45)
	if eventType != "task_complete" {
		t.Errorf("expected task_complete, got %q", eventType)
	}
}

func TestShouldNotify_TaskCompleteTooShort(t *testing.T) {
	eventType, _ := ShouldNotify("idle", "working", 5)
	if eventType != "" {
		t.Errorf("expected empty (too short), got %q", eventType)
	}
}

func TestShouldNotify_NoNotification(t *testing.T) {
	eventType, _ := ShouldNotify("working", "idle", 0)
	if eventType != "" {
		t.Errorf("expected empty, got %q", eventType)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hooks/ -v`
Expected: FAIL

- [ ] **Step 3: Write hooks.go**

```go
package hooks

import (
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/nemke/nagare-go/internal/config"
	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/notifications"
	"github.com/nemke/nagare-go/internal/state"
)

// HookEvent is the JSON structure received from Claude Code hooks via stdin.
type HookEvent struct {
	HookEventName       string `json:"hook_event_name"`
	SessionID           string `json:"session_id"`
	Cwd                 string `json:"cwd"`
	LastAssistantMessage string `json:"last_assistant_message"`
	NotificationType    string `json:"notification_type"`
}

var needsInputTypes = map[string]bool{
	"permission_prompt":  true,
	"elicitation_dialog": true,
}

// EventToState maps a hook event name to a state string.
func EventToState(event, notificationType string) string {
	switch event {
	case "UserPromptSubmit", "PreToolUse", "BeforeAgent", "BeforeTool", "AfterTool":
		return "working"
	case "Stop", "AfterAgent":
		return "idle"
	case "Notification":
		if needsInputTypes[notificationType] {
			return "waiting_input"
		}
		return "idle"
	case "SessionEnd":
		return "dead"
	case "SessionStart":
		return "idle"
	default:
		return "unknown"
	}
}

// ShouldNotify determines if a notification should fire.
// Returns (eventType, workingSeconds). eventType is "" if no notification.
func ShouldNotify(newState, prevState string, workingSeconds int) (string, int) {
	if newState == "waiting_input" {
		return "needs_input", 0
	}

	if newState == "idle" && prevState == "working" && workingSeconds >= 30 {
		return "task_complete", workingSeconds
	}

	return "", 0
}

// Handle reads a hook event from stdin and processes it.
func Handle() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return
	}

	var event HookEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}

	newState := EventToState(event.HookEventName, event.NotificationType)
	now := time.Now().UTC().Format(time.RFC3339)
	statesDir := state.DefaultStatesDir()

	// Load previous state
	prevStates := state.LoadAllStates(statesDir)
	prevState, hasPrev := prevStates[event.Cwd]

	// Write new state
	newSessionState := models.SessionState{
		State:            newState,
		SessionID:        event.SessionID,
		Cwd:              event.Cwd,
		Event:            event.HookEventName,
		NotificationType: event.NotificationType,
		LastMessage:      event.LastAssistantMessage,
		Timestamp:        now,
	}
	state.WriteState(statesDir, newSessionState)

	// Determine working duration
	var workingSeconds int
	if hasPrev && prevState.State == "working" {
		prevTime, err := time.Parse(time.RFC3339, prevState.Timestamp)
		if err == nil {
			workingSeconds = int(time.Since(prevTime).Seconds())
		}
	}

	// Check if notification needed
	prevStateStr := ""
	if hasPrev {
		prevStateStr = prevState.State
	}
	eventType, _ := ShouldNotify(newState, prevStateStr, workingSeconds)
	if eventType == "" {
		return
	}

	// Load config and dispatch
	cfg, _ := config.Load()
	if !cfg.Notifications.Enabled {
		return
	}

	var eventCfg config.NotificationEventConfig
	switch eventType {
	case "needs_input":
		eventCfg = cfg.Notifications.NeedsInput
	case "task_complete":
		eventCfg = cfg.Notifications.TaskComplete
	}

	// Resolve session name from tmux
	sessionName := resolveSessionName(event.Cwd)

	notifications.Deliver(
		sessionName,
		eventType,
		event.NotificationType,
		eventCfg.Toast,
		eventCfg.Bell,
		eventCfg.OsNotify,
		cfg.NotificationDuration,
	)

	// Store notification
	store := notifications.NewStore(notifications.DefaultStorePath())
	message := notifications.BuildToastMessage(sessionName, eventType, event.NotificationType)
	store.Add(sessionName, message)
}

// resolveSessionName finds the tmux session name for a working directory.
func resolveSessionName(cwd string) string {
	// Try to match cwd to a tmux session path
	raw := runTmuxCmd("list-sessions", "-F", "#{session_name}:#{session_path}")
	for _, line := range splitLines(raw) {
		parts := splitN(line, ":", 2)
		if len(parts) == 2 && parts[1] == cwd {
			return parts[0]
		}
	}
	// Fallback: use last path component
	if idx := lastIndex(cwd, "/"); idx >= 0 {
		return cwd[idx+1:]
	}
	return cwd
}

func runTmuxCmd(args ...string) string {
	// Import from tmux package would create a cycle, so inline
	out, _ := exec.Command("tmux", args...).Output()
	return strings.TrimRight(string(out), "\n")
}

func splitLines(s string) []string {
	return strings.Split(s, "\n")
}

func splitN(s, sep string, n int) []string {
	return strings.SplitN(s, sep, n)
}

func lastIndex(s, sep string) int {
	return strings.LastIndex(s, sep)
}
```

Wait — there's an import cycle issue with `tmux` package and `exec` being inlined. Let me fix this. The hooks package should use `os/exec` directly for its minimal tmux calls (just `list-sessions`), avoiding any import of the `tmux` package.

Add these imports to hooks.go:

```go
import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nemke/nagare-go/internal/config"
	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/notifications"
	"github.com/nemke/nagare-go/internal/state"
)
```

And remove the helper functions, replacing with inline calls:

```go
func resolveSessionName(cwd string) string {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}:#{session_path}").Output()
	if err != nil {
		if idx := strings.LastIndex(cwd, "/"); idx >= 0 {
			return cwd[idx+1:]
		}
		return cwd
	}
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && parts[1] == cwd {
			return parts[0]
		}
	}
	if idx := strings.LastIndex(cwd, "/"); idx >= 0 {
		return cwd[idx+1:]
	}
	return cwd
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/hooks/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/hooks/
git commit -m "feat: add hooks package with event-to-state mapping and notification dispatch"
```

---

### Task 11: MCP Interface Stubs

**Files:**
- Create: `internal/mcp/mcp.go`

- [ ] **Step 1: Write mcp.go**

```go
package mcp

import "time"

// AgentInfo describes a discovered agent for MCP listing.
type AgentInfo struct {
	SessionName string
	AgentType   string
	Status      string
	Path        string
}

// Message is an inter-agent message.
type Message struct {
	ID        string
	From      string
	To        string
	Content   string
	Timestamp string
}

// Server defines the MCP server interface. Implementation deferred to v2.
type Server interface {
	ListAgents() ([]AgentInfo, error)
	SendMessage(target string, message string) error
	SendMessageAndWait(target string, message string, timeout time.Duration) (string, error)
	CheckMessages() ([]Message, error)
	Reply(messageID string, response string) error
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/mcp/`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add internal/mcp/
git commit -m "feat: add MCP interface stubs for v2"
```

---

### Task 12: Picker TUI — Styles and Key Bindings

**Files:**
- Create: `internal/picker/styles.go`
- Create: `internal/picker/keys.go`

- [ ] **Step 1: Write styles.go**

```go
package picker

import "github.com/charmbracelet/lipgloss"

// Theme holds the color palette for the picker.
type Theme struct {
	Name       string
	Background lipgloss.Color
	Foreground lipgloss.Color
	Primary    lipgloss.Color
	Secondary  lipgloss.Color
	Accent     lipgloss.Color
	Muted      lipgloss.Color
	Border     lipgloss.Color
}

var themes = map[string]Theme{
	"tokyonight": {
		Name:       "tokyonight",
		Background: lipgloss.Color("#1a1b26"),
		Foreground: lipgloss.Color("#c0caf5"),
		Primary:    lipgloss.Color("#7aa2f7"),
		Secondary:  lipgloss.Color("#bb9af7"),
		Accent:     lipgloss.Color("#7dcfff"),
		Muted:      lipgloss.Color("#565f89"),
		Border:     lipgloss.Color("#3b4261"),
	},
	"catppuccin": {
		Name:       "catppuccin",
		Background: lipgloss.Color("#1e1e2e"),
		Foreground: lipgloss.Color("#cdd6f4"),
		Primary:    lipgloss.Color("#89b4fa"),
		Secondary:  lipgloss.Color("#cba6f7"),
		Accent:     lipgloss.Color("#89dceb"),
		Muted:      lipgloss.Color("#6c7086"),
		Border:     lipgloss.Color("#45475a"),
	},
	"gruvbox": {
		Name:       "gruvbox",
		Background: lipgloss.Color("#282828"),
		Foreground: lipgloss.Color("#ebdbb2"),
		Primary:    lipgloss.Color("#83a598"),
		Secondary:  lipgloss.Color("#d3869b"),
		Accent:     lipgloss.Color("#8ec07c"),
		Muted:      lipgloss.Color("#928374"),
		Border:     lipgloss.Color("#504945"),
	},
}

// ThemeNames returns available theme names.
func ThemeNames() []string {
	names := make([]string, 0, len(themes))
	for name := range themes {
		names = append(names, name)
	}
	return names
}

// Styles holds all lipgloss styles derived from a theme.
type Styles struct {
	Theme         Theme
	SessionList   lipgloss.Style
	SessionItem   lipgloss.Style
	SelectedItem  lipgloss.Style
	DetailPanel   lipgloss.Style
	PreviewPanel  lipgloss.Style
	SearchInput   lipgloss.Style
	StatusBar     lipgloss.Style
	Title         lipgloss.Style
	Muted         lipgloss.Style
}

// NewStyles creates styles from a theme name. Falls back to tokyonight.
func NewStyles(themeName string) Styles {
	theme, ok := themes[themeName]
	if !ok {
		theme = themes["tokyonight"]
	}

	return Styles{
		Theme: theme,
		SessionList: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(theme.Border).
			Padding(1),
		SessionItem: lipgloss.NewStyle().
			PaddingLeft(2),
		SelectedItem: lipgloss.NewStyle().
			PaddingLeft(1).
			Foreground(theme.Primary).
			Bold(true).
			SetString("> "),
		DetailPanel: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(theme.Border).
			Padding(1),
		PreviewPanel: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(theme.Muted).
			Padding(0, 1),
		SearchInput: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(theme.Accent).
			Padding(0, 1),
		StatusBar: lipgloss.NewStyle().
			Foreground(theme.Muted),
		Title: lipgloss.NewStyle().
			Foreground(theme.Primary).
			Bold(true),
		Muted: lipgloss.NewStyle().
			Foreground(theme.Muted),
	}
}
```

- [ ] **Step 2: Write keys.go**

```go
package picker

import "github.com/charmbracelet/bubbletea"

// KeyMap defines all picker key bindings.
type KeyMap struct {
	Up           tea.Key
	Down         tea.Key
	Enter        tea.Key
	Search       tea.Key
	Quit         tea.Key
	Escape       tea.Key
	Approve      tea.Key
	ApproveAlways tea.Key
	InlinePrompt tea.Key
	ToggleView   tea.Key
	CycleTheme   tea.Key
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:            tea.Key{Type: tea.KeyUp},
		Down:          tea.Key{Type: tea.KeyDown},
		Enter:         tea.Key{Type: tea.KeyEnter},
		Search:        tea.Key{Type: tea.KeyRunes, Runes: []rune{'/'}},
		Quit:          tea.Key{Type: tea.KeyRunes, Runes: []rune{'q'}},
		Escape:        tea.Key{Type: tea.KeyEscape},
		Approve:       tea.Key{Type: tea.KeyCtrlY},
		ApproveAlways: tea.Key{Type: tea.KeyCtrlA},
		InlinePrompt:  tea.Key{Type: tea.KeyCtrlL},
		ToggleView:    tea.Key{Type: tea.KeyTab},
		CycleTheme:    tea.Key{Type: tea.KeyCtrlT},
	}
}
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/picker/`
Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add internal/picker/styles.go internal/picker/keys.go
git commit -m "feat: add picker styles (themes) and key bindings"
```

---

### Task 13: Picker TUI — Preview

**Files:**
- Create: `internal/picker/preview.go`

- [ ] **Step 1: Write preview.go**

```go
package picker

import (
	"fmt"

	"github.com/nemke/nagare-go/internal/tmux"
)

// CapturePreview captures the current pane content for a session.
func CapturePreview(sessionName string, windowIndex, paneIndex int) string {
	target := fmt.Sprintf("%s:%d.%d", sessionName, windowIndex, paneIndex)
	return tmux.RunTmux("capture-pane", "-t", target, "-p")
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/picker/`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add internal/picker/preview.go
git commit -m "feat: add picker preview (tmux capture-pane wrapper)"
```

---

### Task 14: Picker TUI — Main Model, Update, View

**Files:**
- Create: `internal/picker/picker.go`

- [ ] **Step 1: Write picker.go**

```go
package picker

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/state"
	"github.com/nemke/nagare-go/internal/tmux"
)

// ViewMode controls list vs grid layout.
type ViewMode int

const (
	ListView ViewMode = iota
	GridView
)

// SortMode controls session ordering.
type SortMode int

const (
	SortByStatus SortMode = iota
	SortByName
	SortByAgent
)

// SessionsUpdatedMsg carries refreshed session data.
type SessionsUpdatedMsg []models.Session

// PreviewUpdatedMsg carries refreshed pane preview.
type PreviewUpdatedMsg string

// Model is the Bubble Tea model for the picker TUI.
type Model struct {
	sessions   []models.Session
	filtered   []models.Session
	cursor     int
	search     string
	searchMode bool
	viewMode   ViewMode
	sortMode   SortMode
	preview    string
	width      int
	height     int
	styles     Styles
	themeIndex int
	statesDir  string

	searchInput textinput.Model
}

// New creates a new picker model.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 64

	return Model{
		styles:      NewStyles("tokyonight"),
		themeIndex:  0,
		statesDir:   state.DefaultStatesDir(),
		searchInput: ti,
	}
}

// Init starts the polling ticks.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tickSessions(),
		tickPreview(),
	)
}

func tickSessions() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		hookStates := state.LoadAllStates(state.DefaultStatesDir())
		sessions := tmux.ScanSessions(hookStates)
		return SessionsUpdatedMsg(sessions)
	})
}

func tickPreview() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return PreviewUpdatedMsg("")
	})
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case SessionsUpdatedMsg:
		m.sessions = []models.Session(msg)
		m.applyFilter()
		return m, tickSessions()

	case PreviewUpdatedMsg:
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			s := m.filtered[m.cursor]
			m.preview = CapturePreview(s.Name, s.WindowIndex, s.PaneIndex)
		}
		return m, tickPreview()
	}

	if m.searchMode {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.search = m.searchInput.Value()
		m.applyFilter()
		return m, cmd
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// In search mode, handle special keys
	if m.searchMode {
		switch msg.Type {
		case tea.KeyEscape, tea.KeyEnter:
			m.searchMode = false
			m.searchInput.Blur()
			return m, nil
		default:
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			m.search = m.searchInput.Value()
			m.applyFilter()
			return m, cmd
		}
	}

	switch msg.String() {
	case "q", "esc":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}

	case "enter":
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			s := m.filtered[m.cursor]
			target := fmt.Sprintf("%s:%d.%d", s.Name, s.WindowIndex, s.PaneIndex)
			tmux.RunTmux("switch-client", "-t", target)
			return m, tea.Quit
		}

	case "/":
		m.searchMode = true
		m.searchInput.Focus()
		return m, textinput.Blink

	case "tab":
		if m.viewMode == ListView {
			m.viewMode = GridView
		} else {
			m.viewMode = ListView
		}

	case "ctrl+t":
		names := ThemeNames()
		m.themeIndex = (m.themeIndex + 1) % len(names)
		m.styles = NewStyles(names[m.themeIndex])

	case "ctrl+y":
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			s := m.filtered[m.cursor]
			if s.Status == models.StatusWaitingInput {
				target := fmt.Sprintf("%s:%d.%d", s.Name, s.WindowIndex, s.PaneIndex)
				tmux.RunTmux("send-keys", "-t", target, "y", "Enter")
			}
		}

	case "ctrl+a":
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			s := m.filtered[m.cursor]
			if s.Status == models.StatusWaitingInput {
				target := fmt.Sprintf("%s:%d.%d", s.Name, s.WindowIndex, s.PaneIndex)
				tmux.RunTmux("send-keys", "-t", target, "a", "Enter")
			}
		}

	case "ctrl+l":
		// Inline prompt — will be interactive, placeholder for now
	}

	return m, nil
}

func (m *Model) applyFilter() {
	if m.search == "" {
		m.filtered = m.sessions
	} else {
		lower := strings.ToLower(m.search)
		m.filtered = nil
		for _, s := range m.sessions {
			if strings.Contains(strings.ToLower(s.Name), lower) ||
				strings.Contains(strings.ToLower(s.Path), lower) {
				m.filtered = append(m.filtered, s)
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

// View renders the TUI.
func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	leftWidth := m.width*2/5 - 2
	rightWidth := m.width - leftWidth - 5

	// Left panel: dashboard + search + session list
	left := m.renderLeft(leftWidth)

	// Right panel: details + preview
	right := m.renderRight(rightWidth)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) renderLeft(width int) string {
	var b strings.Builder

	// Dashboard stats
	counts := m.statusCounts()
	dashboard := fmt.Sprintf(" %d sessions  ", len(m.filtered))
	if counts[models.StatusWaitingInput] > 0 {
		dashboard += fmt.Sprintf("⬤ %d waiting  ", counts[models.StatusWaitingInput])
	}
	if counts[models.StatusRunning] > 0 {
		dashboard += fmt.Sprintf("⬤ %d running  ", counts[models.StatusRunning])
	}
	b.WriteString(m.styles.Title.Render(dashboard))
	b.WriteString("\n")

	// Search bar
	if m.searchMode {
		b.WriteString(m.searchInput.View())
	} else if m.search != "" {
		b.WriteString(m.styles.Muted.Render(fmt.Sprintf(" Filter: %s", m.search)))
	}
	b.WriteString("\n\n")

	// Session list
	for i, s := range m.filtered {
		statusColor := lipgloss.Color(models.StatusColor(s.Status))
		indicator := lipgloss.NewStyle().Foreground(statusColor).Render("●")
		label := models.StatusLabel(s.Status)
		agentBadge := lipgloss.NewStyle().
			Foreground(lipgloss.Color(models.AgentColor(s.AgentType))).
			Background(lipgloss.Color(models.AgentBgColor(s.AgentType))).
			Padding(0, 1).
			Render(string(s.AgentType[0]-32)) // uppercase first letter

		line := fmt.Sprintf(" %s %s  %s  %s", indicator, s.Name, agentBadge, label)

		if i == m.cursor {
			b.WriteString(m.styles.SelectedItem.Render(line))
		} else {
			b.WriteString(m.styles.SessionItem.Render(line))
		}
		b.WriteString("\n")
	}

	return m.styles.SessionList.Width(width).Render(b.String())
}

func (m Model) renderRight(width int) string {
	var b strings.Builder

	if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
		s := m.filtered[m.cursor]

		// Session details
		b.WriteString(m.styles.Title.Render(s.Name))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  Path:   %s\n", s.Path))
		b.WriteString(fmt.Sprintf("  Agent:  %s\n", models.AgentLabel(s.AgentType)))
		b.WriteString(fmt.Sprintf("  Status: %s\n", models.StatusLabel(s.Status)))

		if s.Details.GitBranch != "" {
			b.WriteString(fmt.Sprintf("  Branch: %s\n", s.Details.GitBranch))
		}
		if s.Details.Model != "" {
			b.WriteString(fmt.Sprintf("  Model:  %s\n", s.Details.Model))
		}
		if s.Details.ContextUsage != "" {
			b.WriteString(fmt.Sprintf("  Context: %s\n", s.Details.ContextUsage))
		}

		b.WriteString("\n")

		// Preview
		if m.preview != "" {
			preview := m.styles.PreviewPanel.Width(width - 4).Render(m.preview)
			b.WriteString(preview)
		}
	} else {
		b.WriteString(m.styles.Muted.Render("No sessions found"))
	}

	return m.styles.DetailPanel.Width(width).Render(b.String())
}

func (m Model) statusCounts() map[models.SessionStatus]int {
	counts := make(map[models.SessionStatus]int)
	for _, s := range m.filtered {
		counts[s.Status]++
	}
	return counts
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/picker/`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add internal/picker/picker.go
git commit -m "feat: add picker TUI with list view, search, preview, and key bindings"
```

---

### Task 15: Wire CLI to Real Implementations

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Update main.go**

```go
package main

import (
	"os"

	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nemke/nagare-go/internal/hooks"
	"github.com/nemke/nagare-go/internal/picker"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "nagare-go",
		Short: "tmux session manager for AI coding agents",
	}

	pickCmd := &cobra.Command{
		Use:   "pick",
		Short: "Launch session picker TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := tea.NewProgram(picker.New(), tea.WithAltScreen())
			_, err := p.Run()
			return err
		},
	}

	hookStateCmd := &cobra.Command{
		Use:   "hook-state",
		Short: "Handle Claude Code hook events from stdin",
		Run: func(cmd *cobra.Command, args []string) {
			hooks.Handle()
		},
	}

	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Install hooks to ~/.claude/settings.json",
		Run: func(cmd *cobra.Command, args []string) {
			// TODO: v1 setup implementation
			println("setup: not implemented yet")
		},
	}

	rootCmd.AddCommand(pickCmd, hookStateCmd, setupCmd)

	// Default to "pick" when no subcommand given
	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		pickCmd.RunE(pickCmd, args)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build the full binary**

Run:
```bash
go build -o nagare-go .
```

Expected: Single binary `nagare-go` produced.

- [ ] **Step 3: Verify binary size and startup speed**

Run:
```bash
ls -lh nagare-go
time ./nagare-go --help
```

Expected: Binary ~10-15MB. Help output in <50ms.

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat: wire CLI subcommands to real implementations"
```

---

### Task 16: Integration Test — Full Build + All Unit Tests

**Files:** None (verification only)

- [ ] **Step 1: Run all tests**

Run:
```bash
go test ./... -v
```

Expected: All tests pass across all packages.

- [ ] **Step 2: Run vet and build**

Run:
```bash
go vet ./... && go build -o nagare-go .
```

Expected: No warnings, clean build.

- [ ] **Step 3: Test hook-state with sample input**

Run:
```bash
echo '{"hook_event_name":"Stop","session_id":"test-123","cwd":"/tmp/test"}' | ./nagare-go hook-state
ls ~/.local/share/nagare/states/test-123.json
cat ~/.local/share/nagare/states/test-123.json
```

Expected: State file created with `"state": "idle"`.

- [ ] **Step 4: Measure startup speed**

Run:
```bash
hyperfine './nagare-go --help' 'python -c "import nagare"' --warmup 3
```

Expected: Go version significantly faster (5-10ms vs 200-500ms+).

- [ ] **Step 5: Commit any fixes from integration testing**

```bash
git add -A
git commit -m "fix: integration test fixes"
```

(Only if fixes were needed.)

---

### Task 17: CLAUDE.md for the Go Project

**Files:**
- Create: `CLAUDE.md`

- [ ] **Step 1: Write CLAUDE.md**

```markdown
# nagare-go

Go rewrite of [nagare](../nagare) — tmux session manager for AI coding agents.

## Build & Test

```bash
go build -o nagare-go .    # build
go test ./... -v           # run all tests
go vet ./...               # lint
```

## Architecture

Single binary with cobra subcommands. All code in `internal/` packages.

- `internal/models` — Session, SessionStatus, AgentType
- `internal/config` — TOML config loading
- `internal/tmux` — scanner (list-panes), status detection (pane scraping)
- `internal/state` — state files + session registry
- `internal/hooks` — Claude Code hook handler (stdin JSON)
- `internal/notifications` — delivery (toast/bell/os) + persistent store
- `internal/picker` — Bubble Tea TUI
- `internal/mcp` — interface stubs (v2)

## State Files

Compatible with Python version. Same paths, same JSON schema:
- `~/.local/share/nagare/states/*.json`
- `~/.local/share/nagare/sessions.json`
- `~/.local/share/nagare/notifications.json`
- `~/.config/nagare/config.toml`

## Conventions

- Follow Effective Go (go.dev/doc/effective_go)
- Use `gofmt` — non-negotiable
- Tests colocated: `foo_test.go` next to `foo.go`
- No underscores in names — MixedCaps for exported, mixedCaps for unexported
- Always check errors
- Tokyonight color palette: idle=#00D26A, running=#e0af68, waiting=#db4b4b, dead=#565f89
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add CLAUDE.md for the Go project"
```
