# WP2: Notification Center + Popup Notifications — Implementation Plan

**Goal:** Add the notification center TUI (`nagare-go notifs`) and popup notifications that appear when agents need input or finish tasks.

**Codebase context:**
- Notification store: `internal/notifications/store.go` — `Store` with `Add`, `ListAll`, `MarkRead`, `Dismiss`, `DismissAll`, `UnreadCount`
- Notification delivery: `internal/notifications/deliver.go` — `SendToast`, `SendBell`, `SendOsNotify`, `Deliver`, `BuildToastMessage`
- Config: `internal/config/config.go` — `NotificationConfig`, `NotificationEventConfig` with Toast/Bell/OsNotify/Popup/PopupTimeout/MinWorkingSeconds
- Theme: `internal/theme/` — `theme.Current().Colors` for styling
- Hooks: `internal/hooks/hooks.go` — `Handle()` processes hook events, calls `Deliver()`
- CLI: `main.go` — cobra commands, `notifs` is a stub
- Tmux: `internal/tmux/tmux.go` — `RunTmux(args...)`, `PaneTarget(name, win, pane)`
- Picker overlay: `internal/picker/overlay.go` — `placeOverlay()` for reference on ANSI-aware overlay rendering
- Picker styles: `internal/picker/styles.go` — `dialogStyle()`, `panelStyle()`, `mutedStyle()`, `titleStyle()` as patterns to follow
- Logger: `internal/log/log.go` — `log.Info()`, `log.Debug()`

---

## Part A: Notification Center TUI (`nagare-go notifs`)

A Bubble Tea TUI launched via `nagare-go notifs`. Two tabs: notification list + settings.

### Task A1: Create the notifs TUI model

**Create:** `internal/notifs/notifs.go`

**Model struct:**
```go
type Model struct {
    store       *notifications.Store
    items       []notifications.Notification
    cursor      int
    tab         int // 0 = notifications, 1 = settings
    width       int
    height      int
    cfg         config.NagareConfig
    settingsCur int // cursor in settings tab
}
```

**Init:** Load store, load config, populate items from `store.ListAll()`.

**View:** Two-tab layout:
- Tab bar at top: `[1] Notifications  [2] Settings` — highlight active tab with primary color
- Tab 1: notification list
- Tab 2: settings list
- Hint bar at bottom: `Enter Jump/Toggle | d Dismiss | D Dismiss All | 1/2 Tab | Esc Quit`

**Key handling:**
```
"1"     → switch to notifications tab
"2"     → switch to settings tab
"esc"   → quit
"up"    → cursor up
"down"  → cursor down
"d"     → dismiss selected notification (tab 1 only)
"D"     → dismiss all (tab 1 only, capital D = shift+d)
"enter" → tab 1: mark read + jump to session (tmux switch-client) + quit
          tab 2: toggle bool setting or focus int input
```

### Task A2: Notification list rendering

**Each notification item renders as 2 lines:**
```
Line 1: {read_dot} {icon} {session_name}  {message}
Line 2:    {timestamp}
```

Where:
- `read_dot`: `●` (unread, primary color) or ` ` (read)
- `icon`: `✅` if "finished" in message, else `⏳`
- `session_name`: bold
- `message`: normal foreground
- `timestamp`: muted, format `YYYY-MM-DD HH:MM:SS` (first 19 chars of ISO timestamp)

Selected item: highlighted with primary background (same pattern as picker).

### Task A3: Settings tab rendering

**Settings structure — 12 items in order:**

```
Section: Master
  [x] Notifications enabled          → cfg.Notifications.Enabled

Section: Needs Input
  [x] Toast notification              → cfg.Notifications.NeedsInput.Toast
  [x] Bell                            → cfg.Notifications.NeedsInput.Bell
  [x] OS notification                 → cfg.Notifications.NeedsInput.OsNotify
  [ ] Popup notification              → cfg.Notifications.NeedsInput.Popup
  Popup timeout: 10                   → cfg.Notifications.NeedsInput.PopupTimeout

Section: Task Complete
  [x] Toast notification              → cfg.Notifications.TaskComplete.Toast
  [ ] Bell                            → cfg.Notifications.TaskComplete.Bell
  [ ] OS notification                 → cfg.Notifications.TaskComplete.OsNotify
  [ ] Popup notification              → cfg.Notifications.TaskComplete.Popup
  Popup timeout: 10                   → cfg.Notifications.TaskComplete.PopupTimeout
  Min working seconds: 30             → cfg.Notifications.TaskComplete.MinWorkingSeconds
```

**Bool items:** Display as `[x] Label` or `[ ] Label`. Enter toggles.

**Int items:** Display as `Label: {value}`. Enter opens inline edit (reuse textinput from bubbles). Typing replaces value. Enter confirms.

**Saving:** After each toggle/change, write the full config back to `~/.config/nagare/config.toml`.

**For saving config, add to `internal/config/config.go`:**
```go
func Save(cfg NagareConfig) error {
    path := DefaultPath()
    if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
        return err
    }
    var buf bytes.Buffer
    enc := toml.NewEncoder(&buf)
    if err := enc.Encode(cfg); err != nil {
        return err
    }
    return fsutil.AtomicWrite(path, buf.Bytes(), 0644)
}
```

**Note:** This requires `github.com/BurntSushi/toml` encoder. Check if it supports `Encode`. If not, use `github.com/pelletier/go-toml/v2` or manually construct the TOML string. The `BurntSushi/toml` package does NOT have an encoder — only a decoder. Options:
1. Add `pelletier/go-toml/v2` dependency which has both encode and decode
2. Write TOML manually via `fmt.Fprintf` (simpler for our known structure)
3. Use the existing TOML file and do targeted field replacement

**Recommended approach:** Write TOML manually since our config structure is known and simple:
```go
func Save(cfg NagareConfig) error {
    var b strings.Builder
    fmt.Fprintf(&b, "[notifications]\nenabled = %t\n\n", cfg.Notifications.Enabled)
    writeEventConfig(&b, "notifications.needs_input", cfg.Notifications.NeedsInput)
    writeEventConfig(&b, "notifications.task_complete", cfg.Notifications.TaskComplete)
    // ... picker, appearance sections
    return fsutil.AtomicWrite(DefaultPath(), []byte(b.String()), 0644)
}
```

### Task A4: Wire `nagare-go notifs` command

**Update `main.go`:**

Import `internal/notifs` and replace the notifs stub:
```go
notifsCmd := &cobra.Command{
    Use:   "notifs",
    Short: "View notification history and settings",
    RunE: func(cmd *cobra.Command, args []string) error {
        p := tea.NewProgram(notifs.New(), tea.WithAltScreen())
        _, err := p.Run()
        return err
    },
}
```

---

## Part B: Popup Notification

A small Bubble Tea TUI shown in a tmux split when an agent needs attention.

### Task B1: Create the popup TUI model

**Create:** `internal/popup/popup.go`

**CLI arguments (parsed in main.go):**
```
nagare-go popup-notif --session NAME --event TYPE --message MSG [--timeout SEC] [--duration SEC]
```

Where:
- `--session`: tmux session name
- `--event`: `needs_input` or `task_complete`
- `--message`: notification text
- `--timeout`: auto-dismiss seconds (default 10)
- `--duration`: working seconds for task_complete display

**Model struct:**
```go
type Model struct {
    sessionName    string
    eventType      string
    message        string
    workingSeconds int
    timeout        int
    countdown      int
    preview        string
    width          int
    height         int
}
```

**Init:** Start 1-second tick for countdown + preview refresh.

**View layout:**
```
┌──────────────────────────────────────────────┐
│ ● NEEDS INPUT                                │
│ 💬 session needs permission                  │
├──────────────────────────────────────────────┤
│ (live pane preview from tmux capture-pane)   │
│ ...                                          │
│ ...                                          │
├──────────────────────────────────────────────┤
│ Enter Jump  Ctrl+y Allow  Ctrl+a Always  Esc │
│                            Auto-closing in 8s│
└──────────────────────────────────────────────┘
```

For `task_complete`, header shows:
```
● TASK COMPLETE (worked 5m 30s)
```

And hint bar omits Ctrl+y and Ctrl+a.

**Key handling:**
```
"esc"    → quit (dismiss)
"enter"  → tmux switch-client -t {session} → quit
"ctrl+y" → tmux send-keys -t {session} Enter → quit (needs_input only)
"ctrl+a" → tmux send-keys -t {session} Down Enter → quit (needs_input only)
```

**Countdown:** Tick every 1s, decrement `countdown`. When 0, quit.

**Preview:** Capture pane via `tmux capture-pane -e -t {session} -p`. Refresh each tick. Truncate lines to width. Show bottom portion (most recent output).

### Task B2: Wire `nagare-go popup-notif` command

**Update `main.go`:**

Add cobra command with flags:
```go
popupCmd := &cobra.Command{
    Use:   "popup-notif",
    Short: "Show popup notification for a session",
    RunE: func(cmd *cobra.Command, args []string) error {
        session, _ := cmd.Flags().GetString("session")
        event, _ := cmd.Flags().GetString("event")
        message, _ := cmd.Flags().GetString("message")
        timeout, _ := cmd.Flags().GetInt("timeout")
        duration, _ := cmd.Flags().GetInt("duration")
        p := tea.NewProgram(popup.New(session, event, message, timeout, duration))
        _, err := p.Run()
        return err
    },
}
popupCmd.Flags().String("session", "", "Session name")
popupCmd.Flags().String("event", "", "Event type (needs_input or task_complete)")
popupCmd.Flags().String("message", "", "Notification message")
popupCmd.Flags().Int("timeout", 10, "Auto-dismiss timeout in seconds")
popupCmd.Flags().Int("duration", 0, "Working seconds (for task_complete)")
```

### Task B3: Add popup delivery to hooks

**Update `internal/notifications/deliver.go`** — add `SendPopup` function:

```go
func SendPopup(sessionName, eventType, message string, workingSeconds, popupTimeout int) {
    bin := findBinary()
    args := []string{
        "popup-notif",
        "--session", sessionName,
        "--event", eventType,
        "--message", message,
        "--timeout", fmt.Sprintf("%d", popupTimeout),
    }
    if workingSeconds > 0 {
        args = append(args, "--duration", fmt.Sprintf("%d", workingSeconds))
    }

    // Get active pane to split from
    paneID := tmux.RunTmux("display-message", "-p", "#{pane_id}")
    if paneID == "" {
        return
    }
    cmd := strings.Join(append([]string{bin}, args...), " ")
    tmux.RunTmux("split-window", "-t", paneID, "-v", "-l", "30%", cmd)
}
```

Where `findBinary()` finds the nagare-go executable (same logic as `internal/setup/setup.go`'s `findBinary` — extract to a shared location or duplicate).

**Update `internal/hooks/hooks.go`** — in `Handle()`, after the `Deliver()` call:

```go
if eventCfg.Popup {
    notifications.SendPopup(sessionName, eventType, message, workingSeconds, eventCfg.PopupTimeout)
}
```

The current `Deliver()` function signature is:
```go
func Deliver(message string, toast, bell, osNotify bool, durationMs int)
```

The popup delivery needs more context (session name, event type, working seconds, popup timeout). Either:
1. Add `SendPopup` as a separate call in hooks.go (recommended — keeps Deliver simple)
2. Expand Deliver's signature (not recommended — parameter sprawl)

**Go with option 1:** Call `SendPopup` directly from `Handle()` after `Deliver()`.

### Task B4: Find binary helper (shared)

The `findBinary()` function exists in `internal/setup/setup.go`. Extract it to a shared location.

**Create:** `internal/bin/bin.go`
```go
package bin

import (
    "os"
    "os/exec"
)

// FindSelf locates the nagare-go binary path.
func FindSelf() string {
    if path, err := exec.LookPath("nagare-go"); err == nil {
        return path
    }
    if exe, err := os.Executable(); err == nil {
        return exe
    }
    return "nagare-go"
}
```

Update `internal/setup/setup.go` to use `bin.FindSelf()` instead of its local `findBinary()`.

---

## Summary of Files

| File | Action | Description |
|------|--------|-------------|
| `internal/notifs/notifs.go` | Create | Notification center TUI (Model, Init, Update, View) |
| `internal/popup/popup.go` | Create | Popup notification TUI |
| `internal/bin/bin.go` | Create | Shared binary finder |
| `internal/config/config.go` | Modify | Add `Save()` function for writing config |
| `internal/notifications/deliver.go` | Modify | Add `SendPopup()` |
| `internal/hooks/hooks.go` | Modify | Call `SendPopup` when popup enabled |
| `internal/setup/setup.go` | Modify | Use `bin.FindSelf()` |
| `main.go` | Modify | Wire `notifs` and `popup-notif` commands |

---

## Implementation Order

1. **B4** — Extract `findBinary` to shared `bin` package (small, unblocks B3)
2. **A1-A3** — Notification center TUI (self-contained, can test immediately)
3. **A4** — Wire `notifs` command
4. **B1-B2** — Popup TUI + CLI command
5. **B3** — Hook popup delivery (connects everything)

---

## Testing

**Notification Center:**
1. Run `nagare-go notifs` → should show notification list
2. Press `2` → settings tab with toggles
3. Press Enter on a bool setting → toggles, check config.toml
4. Press `1` → back to notifications
5. Press `d` → dismiss one notification
6. Press `D` → dismiss all
7. Select notification + Enter → jumps to session

**Popup:**
1. Run `nagare-go popup-notif --session test --event needs_input --message "needs permission" --timeout 5`
2. Should show header + preview + countdown
3. Ctrl+y → sends Enter to session pane
4. Ctrl+a → sends Down+Enter
5. Wait 5s → auto-dismisses
6. Test via real hook: trigger a permission prompt in Claude Code → popup should appear

**Integration:**
1. Set `[notifications.needs_input] popup = true` in config.toml
2. Run `nagare-go setup` to install hooks
3. Trigger a permission prompt in Claude Code
4. Popup should appear in a tmux split with the session preview and action buttons
