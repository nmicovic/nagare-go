# WP1: Picker Actions — Implementation Plan

**Goal:** Add the missing picker keybindings that make nagare-go a fully functional daily-use tool.

**Codebase context:**
- Picker TUI: `internal/picker/picker.go` — Bubble Tea Model with `handleKey` switch
- Key constants: `internal/picker/keys.go`
- Registry: `internal/state/registry.go` — has `ToggleStar`, `Register`, `Remove`, `Find`
- Tmux helper: `internal/tmux/tmux.go` — `RunTmux(args...)`, `PaneTarget(name, win, pane)`
- Models: `internal/models/models.go` — `Session`, `SessionStatus` (`StatusWaitingInput` etc.)
- Logger: `internal/log/log.go` — `log.Info(fmt, args...)`, `log.Debug(fmt, args...)`
- Style functions: `internal/picker/styles.go` — `mutedStyle()`, `titleStyle()`, etc.
- Help bar: `internal/picker/help.go` — update `helpBar()` key list after adding new bindings

**How the picker works:**
- `handleKey(msg tea.KeyMsg)` is a switch on `msg.String()` (e.g. `"ctrl+y"`, `"f2"`)
- The search input is always focused. Special keys (Enter, Esc, arrows, ctrl combos, F-keys) are caught in the switch before the `default` branch which forwards to the textinput.
- `m.filtered` is the current visible session list. `m.cursor` is the index into it.
- `m.filtered[m.cursor]` gives the selected `models.Session`.
- After state-changing actions (kill, rename), trigger a rescan with `doScan(m.statesDir)`.

---

## Task 1: Fix Ctrl+y and Ctrl+a (Approve)

**Problem:** Current implementation sends wrong keys and doesn't check session status.

**Current code** in `internal/picker/picker.go` handleKey switch:
```go
case keyApprove:
    if len(m.filtered) > 0 {
        s := m.filtered[m.cursor]
        tmux.RunTmux("send-keys", "-t", tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex), "y", "Enter")
    }
    return m, nil
case keyApproveAlways:
    if len(m.filtered) > 0 {
        s := m.filtered[m.cursor]
        tmux.RunTmux("send-keys", "-t", tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex), "a", "Enter")
    }
    return m, nil
```

**Fix — replace both cases with:**
```go
case keyApprove:
    if s, ok := m.selectedSession(); ok && s.Status == models.StatusWaitingInput {
        tmux.RunTmux("send-keys", "-t", tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex), "Enter")
        log.Info("approved %s", s.Name)
    }
    return m, nil
case keyApproveAlways:
    if s, ok := m.selectedSession(); ok && s.Status == models.StatusWaitingInput {
        tmux.RunTmux("send-keys", "-t", tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex), "Down", "Enter")
        log.Info("approved always %s", s.Name)
    }
    return m, nil
```

**Also add this helper method** (avoids repeating bounds check everywhere):
```go
// selectedSession returns the currently selected session, if any.
func (m Model) selectedSession() (models.Session, bool) {
    if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
        return models.Session{}, false
    }
    return m.filtered[m.cursor], true
}
```

**Key differences from current code:**
- Ctrl+y sends `Enter` only (not `"y", "Enter"`)
- Ctrl+a sends `Down`, `Enter` (not `"a", "Enter"`)
- Both check `s.Status == models.StatusWaitingInput` before sending
- Uses `selectedSession()` helper for safe access

---

## Task 2: Add Ctrl+w (Unload Agent Pane)

**What it does:** Kills just the agent's pane, leaving the tmux session alive.

**Add to keys.go:**
```go
keyUnload = "ctrl+w"
```

**Add to handleKey switch:**
```go
case keyUnload:
    if s, ok := m.selectedSession(); ok {
        // Mark state as dead before killing
        statesDir := state.DefaultStatesDir()
        deadState := models.SessionState{
            State:     "dead",
            SessionID: s.SessionID,
            Cwd:       s.Path,
            Event:     "ManualKill",
            Timestamp: time.Now().UTC().Format(time.RFC3339),
        }
        state.WriteState(statesDir, deadState)
        tmux.RunTmux("kill-pane", "-t", tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex))
        log.Info("unloaded pane %s", s.Name)
        return m, doScan(m.statesDir)
    }
    return m, nil
```

**Required imports in picker.go:** `time` (already imported), `state` (already imported).

**Tmux command:** `tmux kill-pane -t {session}:{window}.{pane}`

---

## Task 3: Add Ctrl+x (Kill Tmux Session)

**What it does:** Kills the entire tmux session (all windows and panes).

**Add to keys.go:**
```go
keyKillSession = "ctrl+x"
```

**Add to handleKey switch:**
```go
case keyKillSession:
    if s, ok := m.selectedSession(); ok {
        statesDir := state.DefaultStatesDir()
        deadState := models.SessionState{
            State:     "dead",
            SessionID: s.SessionID,
            Cwd:       s.Path,
            Event:     "ManualKill",
            Timestamp: time.Now().UTC().Format(time.RFC3339),
        }
        state.WriteState(statesDir, deadState)
        tmux.RunTmux("kill-session", "-t", s.Name)
        log.Info("killed session %s", s.Name)
        return m, doScan(m.statesDir)
    }
    return m, nil
```

**Tmux command:** `tmux kill-session -t {session_name}`

---

## Task 4: Add Ctrl+f (Toggle Star)

**What it does:** Toggles the starred/favorite flag on the selected session. Starred sessions sort to the top.

**Add to keys.go:**
```go
keyStar = "ctrl+f"
```

**First, update `internal/state/registry.go`** — `ToggleStar` currently returns nothing. Change it to return the new starred state:

```go
// ToggleStar toggles the starred flag. Saves to disk. Returns new state.
func (r *Registry) ToggleStar(name string) bool {
    for i := range r.sessions {
        if r.sessions[i].Name == name {
            r.sessions[i].Starred = !r.sessions[i].Starred
            r.save()
            return r.sessions[i].Starred
        }
    }
    return false
}
```

**Add to handleKey switch:**
```go
case keyStar:
    if s, ok := m.selectedSession(); ok {
        reg := state.NewRegistry(state.DefaultRegistryPath())
        // Auto-register if not in registry
        if reg.Find(s.Name) == nil {
            reg.Register(s.Name, s.Path, string(s.AgentType))
        }
        starred := reg.ToggleStar(s.Name)
        if starred {
            log.Info("starred %s", s.Name)
        } else {
            log.Info("unstarred %s", s.Name)
        }
    }
    return m, nil
```

**Sorting integration:** Update `sortFiltered()` in picker.go so starred sessions always sort first, regardless of sort mode. Modify the sort function:

```go
func (m *Model) sortFiltered() {
    sort.SliceStable(m.filtered, func(i, j int) bool {
        // Starred sessions always first
        si := m.isStarred(m.filtered[i].Name)
        sj := m.isStarred(m.filtered[j].Name)
        if si != sj {
            return si
        }
        switch m.sortMode {
        case SortByName:
            return m.filtered[i].Name < m.filtered[j].Name
        case SortByAgent:
            return m.filtered[i].AgentType < m.filtered[j].AgentType
        default:
            return statusOrder(m.filtered[i].Status) < statusOrder(m.filtered[j].Status)
        }
    })
}

func (m Model) isStarred(name string) bool {
    reg := state.NewRegistry(state.DefaultRegistryPath())
    s := reg.Find(name)
    return s != nil && s.Starred
}
```

**Note:** Creating a new Registry on every sort is wasteful. A better approach: cache the registry in the Model. Add `registry *state.Registry` to the Model struct, initialize it in `New()`, and reload it after star toggles.

**Better approach for the Model:**
```go
// In Model struct:
registry *state.Registry

// In New():
registry: state.NewRegistry(state.DefaultRegistryPath()),

// In isStarred:
func (m Model) isStarred(name string) bool {
    s := m.registry.Find(name)
    return s != nil && s.Starred
}

// After toggling star, reload registry:
m.registry = state.NewRegistry(state.DefaultRegistryPath())
```

---

## Task 5: Add Ctrl+o (Cycle Sort Mode)

**Add to keys.go:**
```go
keyCycleSort = "ctrl+o"
```

**Add to handleKey switch:**
```go
case keyCycleSort:
    switch m.sortMode {
    case SortByStatus:
        m.sortMode = SortByName
    case SortByName:
        m.sortMode = SortByAgent
    case SortByAgent:
        m.sortMode = SortByStatus
    }
    m.applyFilter()
    log.Info("sort mode: %d", m.sortMode)
    return m, nil
```

**No additional changes needed** — `applyFilter()` already calls `sortFiltered()` which uses `m.sortMode`.

**Optional:** Show current sort mode in the help bar. Update `helpBar()` in help.go to include the sort indicator. The sort mode names can be derived from the `SortMode` constants.

---

## Task 6: Add F2 (Rename Session)

**This is the most complex action.** It repurposes the search input for rename mode.

**Add to keys.go:**
```go
keyRename = "f2"
```

**Add to Model struct:**
```go
renameMode    bool           // true when F2 rename is active
renameSession models.Session // session being renamed
```

**Add to handleKey — at the TOP of the function, before other cases:**

The rename mode intercepts keys before anything else (similar to how theme picker works):

```go
if m.renameMode {
    return m.handleRenameKey(key)
}
```

**Add handleRenameKey method:**
```go
func (m Model) handleRenameKey(key string) (tea.Model, tea.Cmd) {
    switch key {
    case keyEscape:
        // Cancel rename
        m.renameMode = false
        m.searchInput.SetValue("")
        return m, nil
    case keyEnter:
        // Finish rename
        newName := strings.TrimSpace(m.searchInput.Value())
        m.renameMode = false
        m.searchInput.SetValue("")

        if newName == "" || newName == m.renameSession.Name {
            return m, nil
        }

        oldName := m.renameSession.Name

        // Check if name already exists
        existing := tmux.RunTmux("list-sessions", "-F", "#{session_name}")
        for _, line := range strings.Split(existing, "\n") {
            if strings.TrimSpace(line) == newName {
                log.Info("rename failed: %s already exists", newName)
                return m, nil
            }
        }

        // Count sessions with same name (multi-agent check)
        count := 0
        for _, s := range m.sessions {
            if s.Name == oldName {
                count++
            }
        }

        if count > 1 {
            // Rename just the window
            target := fmt.Sprintf("%s:%d", oldName, m.renameSession.WindowIndex)
            tmux.RunTmux("rename-window", "-t", target, newName)
            log.Info("renamed window %s -> %s", target, newName)
        } else {
            // Rename the tmux session
            tmux.RunTmux("rename-session", "-t", oldName, newName)
            log.Info("renamed session %s -> %s", oldName, newName)

            // Update registry
            if existing := m.registry.Find(oldName); existing != nil {
                path := existing.Path
                agent := existing.Agent
                m.registry.Remove(oldName)
                m.registry.Register(newName, path, agent)
            }
        }

        return m, doScan(m.statesDir)
    default:
        // Forward to search input for text editing
        var cmd tea.Cmd
        m.searchInput, cmd = m.searchInput.Update(/* need the original msg */)
        return m, cmd
    }
}
```

**Problem:** `handleRenameKey` receives `key` (string), but the textinput needs the original `tea.KeyMsg`. Refactor: pass the `tea.KeyMsg` instead of `key` to the rename handler.

**Better approach — change handleKey signature to pass msg through:**

At the top of `handleKey`:
```go
if m.renameMode {
    return m.handleRenameKey(msg)
}
```

And `handleRenameKey` takes `tea.KeyMsg`:
```go
func (m Model) handleRenameKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    key := msg.String()
    switch key {
    case keyEscape:
        m.renameMode = false
        m.searchInput.SetValue("")
        return m, nil
    case keyEnter:
        // ... rename logic as above ...
    default:
        var cmd tea.Cmd
        m.searchInput, cmd = m.searchInput.Update(msg)
        return m, cmd
    }
}
```

**F2 trigger in handleKey switch:**
```go
case keyRename:
    if s, ok := m.selectedSession(); ok {
        m.renameMode = true
        m.renameSession = s
        m.searchInput.SetValue(s.Name)
        // Select all text so typing replaces it
        m.searchInput.CursorEnd()
    }
    return m, nil
```

**Visual indicator:** When `m.renameMode` is true, change the search prompt. In `viewLeft`, before rendering the search input:
```go
if m.renameMode {
    m.searchInput.Prompt = " Rename: "
} else {
    m.searchInput.Prompt = " > "
}
```

**Tmux commands:**
- Single session: `tmux rename-session -t {old_name} {new_name}`
- Multi-agent window: `tmux rename-window -t {session}:{window} {new_name}`

---

## Task 7: Update Help Bar

After adding all new keybindings, update `internal/picker/help.go`:

**In `helpBar()`**, add the new keys to the pairs list:
```go
pairs := []struct{ k, v string }{
    {"Enter", "Jump"},
    {"↑/↓", "Navigate"},
    {"Tab", "View"},
    {"Ctrl+y", "Allow"},
    {"Ctrl+a", "Always"},
    {"Ctrl+f", "Star"},
    {"Ctrl+o", "Sort"},
    {"Ctrl+w", "Unload"},
    {"Ctrl+x", "Kill"},
    {"F2", "Rename"},
    {"Ctrl+t", "Theme"},
    {"F1", "Help"},
    {"Esc", "Quit"},
}
```

**In `helpOverlay()`**, add new entries to the Actions section:
```go
section("Actions"),
line("Ctrl+y", "Approve permission (waiting sessions)"),
line("Ctrl+a", "Approve always (waiting sessions)"),
line("Ctrl+f", "Toggle star/favorite"),
line("Ctrl+o", "Cycle sort mode (status/name/agent)"),
line("Ctrl+w", "Unload agent (kill pane)"),
line("Ctrl+x", "Kill entire tmux session"),
line("F2", "Rename session"),
```

---

## Summary of Files Changed

| File | Changes |
|------|---------|
| `internal/picker/keys.go` | Add 5 new key constants |
| `internal/picker/picker.go` | Add `selectedSession()` helper, `registry` field, `renameMode`/`renameSession` fields, 7 new case branches in handleKey, `handleRenameKey` method, `isStarred` method, update `sortFiltered` for starred-first, visual rename indicator in viewLeft |
| `internal/state/registry.go` | Change `ToggleStar` return type to `bool` |
| `internal/picker/help.go` | Update helpBar pairs + helpOverlay actions |

---

## Testing

After implementation, verify each action:

1. **Ctrl+y**: Select a session in WAITING_INPUT, press Ctrl+y → permission approved
2. **Ctrl+a**: Select a session in WAITING_INPUT, press Ctrl+a → always approved
3. **Ctrl+w**: Select a session, press Ctrl+w → pane killed, session list refreshes
4. **Ctrl+x**: Select a session, press Ctrl+x → entire tmux session killed
5. **Ctrl+f**: Select a session, press Ctrl+f → star toggles (check sessions.json)
6. **Ctrl+o**: Press Ctrl+o multiple times → sort cycles through status/name/agent
7. **F2**: Select a session, press F2 → search shows "Rename: {name}", type new name, Enter → renamed in tmux
8. **Edge cases**: Try all actions with 0 sessions, with dead sessions, with sessions that don't exist
