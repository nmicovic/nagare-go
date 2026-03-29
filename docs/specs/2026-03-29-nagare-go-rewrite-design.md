# nagare-go: Go Rewrite Design Spec

## Motivation

Rewrite nagare (Python tmux session manager for AI coding agents) in Go for:
- **Startup speed:** Single compiled binary (~5ms) vs Python interpreter boot (~500ms+)
- **Binary distribution:** One file, no runtime, no venv, no dependencies on target machine
- **Cross-platform:** Cross-compile from Linux to macOS (Intel/ARM) and Windows/WSL

## V1 Scope

Core loop only — enough to validate startup speed and replace the Python version for daily use.

### Included in v1

| Module | Responsibility | Python equivalent |
|---|---|---|
| `config` | Load TOML from `~/.config/nagare/config.toml` | `config.py` |
| `models` | Session, SessionStatus, AgentType structs | `models.py` |
| `tmux/scanner` | `tmux list-panes -a`, parse output, build Sessions | `tmux/scanner.py` |
| `tmux/status` | Pane content scraping fallback for status | `tmux/status.py` |
| `state` | Read/write `~/.local/share/nagare/states/*.json` and `sessions.json` | `state.py`, `registry.py` |
| `hooks` | Stdin JSON handler → state files → notification dispatch | `hooks.py` |
| `notifications` | Deliver (toast/bell/os_notify) + persistent store | `notifications/` |
| `picker` | Bubble Tea TUI — list view, grid view, preview, search, keybindings | `pick.py` |

### Stubbed (interfaces only)

| Module | Purpose |
|---|---|
| `mcp` | Interface definitions for MCP server (list_agents, send_message, etc.) |

### Deferred to v2

- Sounds engine (CESP/openpeon)
- Voice engine (TTS)
- Token tracking
- Popup notification TUI
- Notification center TUI
- Session manager TUI
- Quick prototype form
- New session form
- Claude history reader
- Setup command (partial — hook installation will be in v1)

## Architecture

### Approach: Monolith with Internal Packages

Single binary, subcommand-based CLI. All application code lives in `internal/` packages (compiler-enforced private — cannot be imported by external projects).

```
nagare-go pick          # launch picker TUI (default)
nagare-go hook-state    # handle Claude Code hook events via stdin
nagare-go notifs        # notification center (v2, stubbed)
nagare-go setup         # install hooks to ~/.claude/settings.json
```

### Project Layout

```
nagare-go/
├── go.mod
├── go.sum
├── main.go                      # cobra root command + subcommands
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── models/
│   │   └── models.go
│   ├── tmux/
│   │   ├── scanner.go
│   │   └── status.go
│   ├── state/
│   │   ├── state.go
│   │   └── registry.go
│   ├── hooks/
│   │   └── hooks.go
│   ├── notifications/
│   │   ├── deliver.go
│   │   └── store.go
│   ├── picker/
│   │   ├── picker.go           # Model, Update, View
│   │   ├── keys.go             # key bindings
│   │   ├── styles.go           # lipgloss styles / themes
│   │   └── preview.go          # pane preview logic
│   └── mcp/
│       └── mcp.go              # interface stubs only
```

### Data Flow

```
Hook events (stdin JSON) → hooks → state files (JSON)
                                 → notifications (toast/bell/os)

Picker poll (2s) → scanner (tmux list-panes) → state reader → Session structs → TUI update
Picker poll (1s) → tmux capture-pane → preview panel
```

## Data Models

### SessionStatus

```go
type SessionStatus string

const (
    StatusWaitingInput SessionStatus = "waiting_input"
    StatusRunning      SessionStatus = "running"
    StatusIdle         SessionStatus = "idle"
    StatusDead         SessionStatus = "dead"
)
```

### AgentType

```go
type AgentType string

const (
    AgentClaude   AgentType = "claude"
    AgentOpenCode AgentType = "opencode"
    AgentGemini   AgentType = "gemini"
    AgentUnknown  AgentType = "unknown"
)
```

### Session

```go
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

type SessionDetails struct {
    GitBranch    string
    Model        string
    ContextUsage string
}
```

### SessionState (JSON-compatible with Python version)

```go
type SessionState struct {
    State            string `json:"state"`
    SessionID        string `json:"session_id"`
    Cwd              string `json:"cwd"`
    Event            string `json:"event"`
    NotificationType string `json:"notification_type,omitempty"`
    LastMessage      string `json:"last_message,omitempty"`
    Timestamp        string `json:"timestamp"`
}
```

## Picker TUI

Built with Bubble Tea (Elm architecture: Model → Update → View).

### Layout

```
┌─────────────────────┬──────────────────────────┐
│  Dashboard stats    │  Session details         │
│─────────────────────│                          │
│  [Search: ___]      │  Name, path, agent       │
│─────────────────────│  Git branch, model       │
│  > Session 1  IDLE  │  Context usage           │
│    Session 2  RUN   │──────────────────────────│
│    Session 3  WAIT  │  Live tmux preview       │
│    Session 4  IDLE  │  (capture-pane output)   │
│                     │                          │
└─────────────────────┴──────────────────────────┘
```

### Key Bindings (v1)

| Key | Action |
|---|---|
| ↑/↓, j/k | Navigate sessions |
| Enter | Jump to session |
| / | Toggle search |
| Ctrl+y | Approve permission (WAITING_INPUT) |
| Ctrl+a | Approve always |
| Ctrl+l | Send inline prompt |
| Tab | Toggle list/grid view |
| Ctrl+t | Cycle theme |
| q/Esc | Quit |

### Polling

- Session refresh: `tea.Tick` every 2s → runs scanner in goroutine → `SessionsUpdatedMsg`
- Preview refresh: `tea.Tick` every 1s → runs `tmux capture-pane` in goroutine → `PreviewUpdatedMsg`

## Hooks & Notifications

### Hook Handler (`nagare-go hook-state`)

1. Read one JSON object from stdin
2. Map event to state (`Stop` → `idle`, `UserPromptSubmit` → `working`, etc.)
3. Write state file to `~/.local/share/nagare/states/{session_id}.json`
4. Determine if notification is needed
5. Dispatch notifications and exit

### Notification Channels (v1)

| Channel | Implementation |
|---|---|
| Toast | `tmux display-message` |
| Bell | `tmux run-shell` with `printf '\a'` |
| OS Notify | `notify-send` (Linux), `wsl-notify-send` (WSL), `osascript` (macOS) |

All fire-and-forget via `exec.Command`. Platform detection: `runtime.GOOS` + `/proc/version` for WSL.

### Task Complete Detection

Track working → idle transitions. Only notify if working duration > `min_working_seconds` (default 30s).

### Notification Store

Same JSON file as Python (`~/.local/share/nagare/notifications.json`). Read/unread tracking.

## Configuration

TOML format, same path: `~/.config/nagare/config.toml`. Same structure as Python version.

Parsed with `github.com/BurntSushi/toml` into Go structs with toml tags.

## State File Compatibility

Full read/write compatibility with the Python version:
- Same paths: `~/.local/share/nagare/states/`, `~/.local/share/nagare/sessions.json`
- Same JSON schema
- Go and Python versions can coexist

## Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI subcommands |
| `github.com/charmbracelet/bubbletea` | TUI framework |
| `github.com/charmbracelet/bubbles` | Pre-built TUI components |
| `github.com/charmbracelet/lipgloss` | TUI styling |
| `github.com/BurntSushi/toml` | TOML config parsing |

5 dependencies total.

## Build & Distribution

```bash
go build -o nagare-go .                                    # Linux
GOOS=darwin GOARCH=arm64 go build -o nagare-go-mac-arm .   # macOS ARM
GOOS=darwin GOARCH=amd64 go build -o nagare-go-mac .       # macOS Intel
```

Expected binary size: ~10-15MB (strippable to ~8MB with `-ldflags="-s -w"`).

## Testing

Go built-in testing (`go test ./...`). Test files colocated with source (`scanner_test.go` next to `scanner.go`).

## MCP Stub (v1)

Interface definitions only — no implementation:

```go
type MCPServer interface {
    ListAgents() ([]AgentInfo, error)
    SendMessage(target string, message string) error
    SendMessageAndWait(target string, message string, timeout time.Duration) (string, error)
    CheckMessages() ([]Message, error)
    Reply(messageID string, response string) error
}
```

Implementation deferred to v2 pending Go MCP SDK evaluation.
