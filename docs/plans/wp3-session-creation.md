# WP3: Session Creation — Implementation Plan

**Goal:** Create new tmux sessions with AI agents from the CLI and the picker.

**Codebase context:**
- Registry: `internal/state/registry.go` — `Register(name, path, agent)`, `Find(name)`
- Config: `internal/config/config.go` — `PickerConfig.QuickProjectPath`
- Tmux: `internal/tmux/tmux.go` — `RunTmux(args...)`
- Picker: `internal/picker/picker.go` — handleKey switch for Ctrl+n, Ctrl+r
- Picker keys: `internal/picker/keys.go` — add new constants
- CLI: `main.go` — cobra commands
- Logger: `internal/log/log.go`
- Theme: `internal/theme/` — for form styling

---

## Task 1: Session creation core logic

**Create:** `internal/session/session.go`

```go
package session

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/nemke/nagare-go/internal/config"
    "github.com/nemke/nagare-go/internal/log"
    "github.com/nemke/nagare-go/internal/state"
    "github.com/nemke/nagare-go/internal/tmux"
)
```

### Functions to implement:

**ResolvePath(path string) string**
- If path contains no `/` and no `~`: treat as bare name, prepend `config.QuickProjectPath`
- Otherwise return as-is
```go
func ResolvePath(path string) string {
    if !strings.Contains(path, "/") && !strings.Contains(path, "~") {
        cfg, _ := config.Load()
        return filepath.Join(cfg.Picker.QuickProjectPath, path)
    }
    return path
}
```

**ExpandPath(path string) string**
- Expand `~` to home directory
- Return absolute path
```go
func ExpandPath(path string) string {
    if strings.HasPrefix(path, "~/") {
        home, _ := os.UserHomeDir()
        path = filepath.Join(home, path[2:])
    }
    abs, err := filepath.Abs(path)
    if err != nil {
        return path
    }
    return abs
}
```

**UniqueName(name string) string**
- Check existing tmux sessions
- If name is unique, return it
- Otherwise try `name-2`, `name-3`, ... up to `name-99`
```go
func UniqueName(name string) string {
    existing := make(map[string]bool)
    raw := tmux.RunTmux("list-sessions", "-F", "#{session_name}")
    for _, line := range strings.Split(raw, "\n") {
        existing[strings.TrimSpace(line)] = true
    }
    if !existing[name] {
        return name
    }
    for i := 2; i < 100; i++ {
        candidate := fmt.Sprintf("%s-%d", name, i)
        if !existing[candidate] {
            return candidate
        }
    }
    return fmt.Sprintf("%s-%d", name, os.Getpid())
}
```

**Create(path, name, agent string, continueSession bool) (string, error)**
- Resolve and expand path
- Create directory if needed
- Generate name from path basename if empty
- Make name unique
- Create tmux session
- Launch agent
- Register in registry
- Return session name
```go
func Create(path, name, agent string, continueSession bool) (string, error) {
    path = ExpandPath(ResolvePath(path))

    if err := os.MkdirAll(path, 0755); err != nil {
        return "", fmt.Errorf("cannot create directory: %w", err)
    }

    if name == "" {
        name = filepath.Base(path)
    }
    name = UniqueName(name)

    // Create tmux session
    tmux.RunTmux("new-session", "-d", "-s", name, "-c", path)

    // Launch agent
    cmd := agentCommand(agent, continueSession)
    tmux.RunTmux("send-keys", "-t", name, cmd, "Enter")

    // Register
    reg := state.NewRegistry(state.DefaultRegistryPath())
    reg.Register(name, path, agent)

    log.Info("created session %s (%s) at %s", name, agent, path)
    return name, nil
}
```

**agentCommand(agent string, continueSession bool) string**
```go
func agentCommand(agent string, continueSession bool) string {
    switch agent {
    case "opencode":
        if continueSession {
            return "opencode -c"
        }
        return "opencode"
    case "gemini":
        return "gemini" // gemini auto-continues, no -c flag
    default: // claude
        if continueSession {
            return "claude -c"
        }
        return "claude"
    }
}
```

**ListDirectories(partial string, maxResults int) []string**
- For path autocomplete in the form
- Expand `~`, list subdirectories matching prefix
- Skip hidden directories
```go
func ListDirectories(partial string, maxResults int) []string {
    partial = ExpandPath(partial)

    dir := partial
    prefix := ""
    if !strings.HasSuffix(partial, "/") {
        dir = filepath.Dir(partial)
        prefix = strings.ToLower(filepath.Base(partial))
    }

    entries, err := os.ReadDir(dir)
    if err != nil {
        return nil
    }

    var results []string
    for _, e := range entries {
        if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
            continue
        }
        if prefix != "" && !strings.HasPrefix(strings.ToLower(e.Name()), prefix) {
            continue
        }
        results = append(results, filepath.Join(dir, e.Name()))
        if len(results) >= maxResults {
            break
        }
    }
    return results
}
```

**SwitchToSession(name string)**
```go
func SwitchToSession(name string) {
    // Inside tmux: switch client
    if os.Getenv("TMUX") != "" {
        tmux.RunTmux("switch-client", "-t", name)
    } else {
        // Outside tmux: attach
        tmux.RunTmux("attach-session", "-t", name)
    }
}
```

---

## Task 2: `nagare-go new` CLI command

**Update `main.go`:**

```go
newCmd := &cobra.Command{
    Use:   "new [path]",
    Short: "Create a new agent session",
    Args:  cobra.MaximumNArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        agent, _ := cmd.Flags().GetString("agent")
        name, _ := cmd.Flags().GetString("name")
        cont, _ := cmd.Flags().GetBool("continue")

        if len(args) == 0 {
            // No path: launch interactive form
            p := tea.NewProgram(newsession.New(), tea.WithAltScreen())
            _, err := p.Run()
            return err
        }

        // Direct creation
        sessionName, err := session.Create(args[0], name, agent, cont)
        if err != nil {
            return err
        }
        session.SwitchToSession(sessionName)
        return nil
    },
}
newCmd.Flags().StringP("agent", "a", "claude", "Agent type (claude, opencode, gemini)")
newCmd.Flags().StringP("name", "n", "", "Session name (default: path basename)")
newCmd.Flags().BoolP("continue", "c", true, "Continue previous session")
```

Add to rootCmd: `rootCmd.AddCommand(..., newCmd)`

---

## Task 3: New session form (Ctrl+n)

**Create:** `internal/newsession/newsession.go`

A Bubble Tea TUI for interactive session creation with 4 fields.

**Model struct:**
```go
type Model struct {
    pathInput    textinput.Model
    nameInput    textinput.Model
    agent        string   // "claude", "opencode", "gemini"
    continueSession bool
    focus        int      // 0=path, 1=name, 2=agent, 3=continue
    suggestions  []string // path autocomplete
    sugCursor    int
    showSugs     bool
    width        int
    height       int
    done         bool     // true when form submitted
    result       string   // created session name (for picker to switch to)
    err          error
}
```

**Fields layout:**
```
╭─ New Session ──────────────────────────────╮
│                                            │
│  Path:   [~/Projects/my-project        ]   │
│          ~/Projects/my-app                 │
│          ~/Projects/my-lib                 │
│                                            │
│  Name:   [my-project                   ]   │
│                                            │
│  Agent:  (●) Claude  ( ) OpenCode  ( ) Gemini │
│                                            │
│  [x] Continue previous session             │
│                                            │
│  Enter: Create  Tab: Next  Esc: Cancel     │
╰────────────────────────────────────────────╯
```

**Key handling:**
```
Tab       → cycle focus to next field
Shift+Tab → cycle focus to previous field (if bubbletea supports it, otherwise skip)
Enter     → if on path field: confirm path, move to name, auto-fill name from basename
            if on other fields: submit form (create session)
Esc       → cancel, return to picker
← →       → on agent field: cycle agent selection
            on continue field: toggle
up/down   → on path field with suggestions: browse suggestions
```

**Path autocomplete:**
- On each keystroke in path field, call `session.ListDirectories(pathInput.Value(), 5)`
- Show suggestions below the input
- Up/down to browse, Tab to accept suggestion

**Form submission:**
```go
func (m Model) submit() (tea.Model, tea.Cmd) {
    path := m.pathInput.Value()
    name := m.nameInput.Value()
    if path == "" {
        return m, nil
    }
    sessionName, err := session.Create(path, name, m.agent, m.continueSession)
    if err != nil {
        m.err = err
        return m, nil
    }
    m.done = true
    m.result = sessionName
    return m, tea.Quit
}
```

**View:** Use `dialogStyle()` from picker patterns. Center the form on screen.

---

## Task 4: Quick prototype form (Ctrl+r)

**Create:** `internal/newsession/quickproto.go` (in same package)

Simpler form — just name + agent.

**Model struct:**
```go
type QuickModel struct {
    nameInput textinput.Model
    agent     string
    focus     int    // 0=name, 1=agent
    width     int
    height    int
    done      bool
    result    string
    err       error
}
```

**Layout:**
```
╭─ Quick Prototype ─────────────────────╮
│                                       │
│  Name:   [my-prototype            ]   │
│                                       │
│  Agent:  (●) Claude  ( ) OpenCode     │
│                                       │
│  Enter: Create  Esc: Cancel           │
╰───────────────────────────────────────╯
```

**Key handling:**
```
Tab   → cycle focus
Enter → submit (create at quick_project_path/name)
Esc   → cancel
← →   → on agent: cycle
```

**Submission:**
```go
func (m QuickModel) submit() (tea.Model, tea.Cmd) {
    name := strings.TrimSpace(m.nameInput.Value())
    if name == "" {
        return m, nil
    }
    // Quick prototype always creates fresh (no continue)
    sessionName, err := session.Create(name, name, m.agent, false)
    if err != nil {
        m.err = err
        return m, nil
    }
    m.done = true
    m.result = sessionName
    return m, tea.Quit
}
```

Note: passing `name` as path — `session.Create` calls `ResolvePath` which prepends `QuickProjectPath` for bare names.

---

## Task 5: Wire Ctrl+n and Ctrl+r in picker

**Add to `internal/picker/keys.go`:**
```go
keyNewSession   = "ctrl+n"
keyQuickProto   = "ctrl+r"
```

**The challenge:** Ctrl+n and Ctrl+r need to launch a sub-TUI (the form) which is a separate Bubble Tea program. Options:

**Option A (recommended): Return a special quit message and handle in main.go**

The picker returns a result indicating what to do next:
```go
// In picker package:
type Result struct {
    Action string // "jump", "new", "quickproto", ""
    Target string // session name for jump
}
```

When Ctrl+n is pressed, set `m.result = Result{Action: "new"}` and quit. In main.go, check the result and launch the appropriate form.

**Option B: Launch form as overlay inside picker**

More complex — the form would be another overlay like the theme picker. But forms need focused text inputs which conflict with the always-active search.

**Go with Option A.** Update picker:

```go
// Add to Model:
result Result

// In handleKey:
case keyNewSession:
    m.result = Result{Action: "new"}
    return m, tea.Quit
case keyQuickProto:
    m.result = Result{Action: "quickproto"}
    return m, tea.Quit
```

Export a method to get the result:
```go
func (m Model) Result() Result { return m.result }
```

**Update main.go pick command** to handle the loop:
```go
pickCmd := &cobra.Command{
    Use:   "pick",
    Short: "Launch session picker TUI",
    RunE: func(cmd *cobra.Command, args []string) error {
        for {
            m := picker.New()
            p := tea.NewProgram(m, tea.WithAltScreen())
            result, err := p.Run()
            if err != nil {
                return err
            }

            pickerModel, ok := result.(picker.Model)
            if !ok {
                return nil
            }

            switch pickerModel.Result().Action {
            case "new":
                p := tea.NewProgram(newsession.New(), tea.WithAltScreen())
                if _, err := p.Run(); err != nil {
                    return err
                }
                // Loop back to picker after form closes
                continue
            case "quickproto":
                p := tea.NewProgram(newsession.NewQuick(), tea.WithAltScreen())
                if _, err := p.Run(); err != nil {
                    return err
                }
                continue
            default:
                return nil
            }
        }
    },
}
```

---

## Task 6: Update help bar

**Add to `internal/picker/help.go` helpBar pairs:**
```go
{"Ctrl+n", "New"},
{"Ctrl+r", "Proto"},
```

**Add to helpOverlay Actions section:**
```go
line("Ctrl+n", "Create new session"),
line("Ctrl+r", "Quick prototype"),
```

---

## Summary of Files

| File | Action | Description |
|------|--------|-------------|
| `internal/session/session.go` | Create | Core logic: Create, ResolvePath, ExpandPath, UniqueName, ListDirectories, SwitchToSession, agentCommand |
| `internal/newsession/newsession.go` | Create | New session form TUI (path autocomplete, name, agent, continue) |
| `internal/newsession/quickproto.go` | Create | Quick prototype form TUI (name + agent) |
| `internal/picker/keys.go` | Modify | Add keyNewSession, keyQuickProto |
| `internal/picker/picker.go` | Modify | Add Result type, handle Ctrl+n/Ctrl+r, export Result() |
| `internal/picker/help.go` | Modify | Add new keybindings to help bar + overlay |
| `main.go` | Modify | Add `new` command, update pick command for form loop |

---

## Testing

1. **CLI direct creation:**
   ```bash
   ./nagare-go new ~/test-project --agent claude
   # Should create tmux session, launch claude, switch to it
   ```

2. **CLI bare name:**
   ```bash
   ./nagare-go new mytest --agent opencode
   # Should create at ~/Prototypes/mytest (or configured quick_project_path)
   ```

3. **CLI no args (form):**
   ```bash
   ./nagare-go new
   # Should show interactive form
   ```

4. **Picker Ctrl+n:**
   - Open picker, press Ctrl+n → form opens
   - Type path, see autocomplete suggestions
   - Tab to name → auto-fills from path
   - Enter → session created, switches to it

5. **Picker Ctrl+r:**
   - Open picker, press Ctrl+r → quick form opens
   - Type name, select agent, Enter → session created

6. **Edge cases:**
   - Duplicate session name → gets `-2` suffix
   - Bare name resolves to quick_project_path
   - `~` expands correctly
   - Empty path/name → no-op
