# WP4: Inline Prompting + Editor Integration — Implementation Plan

**Goal:** Send prompts to agent sessions without leaving the picker, and open config in an editor.

**Codebase context:**
- Picker: `internal/picker/picker.go` — handleKey, overlay system, `selectedSession()` helper
- Picker overlay: `internal/picker/overlay.go` — `placeOverlay()` for rendering dialogs on top of content
- Picker styles: `internal/picker/styles.go` — `dialogStyle()`, `mutedStyle()`, `titleStyle()`
- Tmux: `internal/tmux/tmux.go` — `RunTmux()`, `PaneTarget()`
- Config: `internal/config/config.go` — `DefaultPath()`, `Save()`
- Keys: `internal/picker/keys.go`
- Help: `internal/picker/help.go`
- Logger: `internal/log/log.go`

---

## Task 1: Inline Prompt (Ctrl+l)

**What it does:** Opens a text input overlay. User types a prompt, presses Enter, and it's sent to the selected session's tmux pane via `send-keys`.

### Model changes

Add to picker Model struct:
```go
promptMode    bool
promptTarget  models.Session
promptInput   textinput.Model
```

Initialize `promptInput` in `New()`:
```go
pi := textinput.New()
pi.Placeholder = "type prompt to send..."
pi.CharLimit = 500
pi.Width = 60
```

### Key handling

**Add to `internal/picker/keys.go`:**
```go
keyInlinePrompt = "ctrl+l"
keyEditPrompt   = "ctrl+g"
keyEditConfig   = "ctrl+e"
```

**In `handleKey`, before the main switch (after renameMode and themePickMode):**
```go
if m.promptMode {
    return m.handlePromptKey(msg)
}
```

**Ctrl+l trigger in the main switch:**
```go
case keyInlinePrompt:
    if s, ok := m.selectedSession(); ok {
        m.promptMode = true
        m.promptTarget = s
        m.promptInput.SetValue("")
        m.promptInput.Focus()
    }
    return m, nil
```

### handlePromptKey method

```go
func (m Model) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    key := msg.String()
    switch key {
    case "esc":
        m.promptMode = false
        return m, nil
    case "enter":
        text := strings.TrimSpace(m.promptInput.Value())
        if text != "" {
            sendPromptToPane(m.promptTarget, text)
            log.Info("prompt sent to %s: %s", m.promptTarget.Name, text)
        }
        m.promptMode = false
        return m, nil
    default:
        var cmd tea.Cmd
        m.promptInput, cmd = m.promptInput.Update(msg)
        return m, cmd
    }
}
```

### sendPromptToPane function

```go
func sendPromptToPane(s models.Session, text string) {
    target := tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex)
    lines := strings.Split(text, "\n")
    for i, line := range lines {
        if i < len(lines)-1 {
            // Intermediate lines: send text then Enter separately
            tmux.RunTmux("send-keys", "-t", target, line, "")
            tmux.RunTmux("send-keys", "-t", target, "Enter", "")
        } else {
            // Last line: send with Enter to execute
            tmux.RunTmux("send-keys", "-t", target, line, "Enter")
        }
    }
}
```

### Rendering

In `View()`, after the help/theme overlay checks, add:
```go
if m.promptMode {
    overlay := m.renderPromptOverlay()
    return placeOverlay(m.width, m.height, overlay, base)
}
```

The prompt overlay is a simple centered dialog:
```go
func (m Model) renderPromptOverlay() string {
    c := theme.Current().Colors
    title := lipgloss.NewStyle().Foreground(c.Primary).Bold(true).
        Render("Send to: " + m.promptTarget.Name)
    hint := lipgloss.NewStyle().Foreground(c.Muted).
        Render("Enter send  Esc cancel")
    content := title + "\n\n" + m.promptInput.View() + "\n\n" + hint
    return dialogStyle().Padding(1, 2).Render(content)
}
```

---

## Task 2: Editor Prompt (Ctrl+g)

**What it does:** Opens `$EDITOR` with a temp file. After the editor closes, the content is sent to the selected session's pane.

### Implementation

Bubble Tea supports suspending the program to run an external command via `tea.ExecProcess`.

**Ctrl+g trigger in handleKey:**
```go
case keyEditPrompt:
    if s, ok := m.selectedSession(); ok {
        m.promptTarget = s
        return m, m.openEditorPrompt()
    }
    return m, nil
```

**openEditorPrompt method:**
```go
func (m Model) openEditorPrompt() tea.Cmd {
    editor := os.Getenv("EDITOR")
    if editor == "" {
        editor = os.Getenv("VISUAL")
    }
    if editor == "" {
        editor = "vi"
    }

    // Create temp file
    tmpFile, err := os.CreateTemp("", "nagare-prompt-*.md")
    if err != nil {
        log.Error("editor prompt: %v", err)
        return nil
    }
    tmpFile.Close()
    tmpPath := tmpFile.Name()

    c := exec.Command(editor, tmpPath)
    return tea.ExecProcess(c, func(err error) tea.Msg {
        return editorDoneMsg{path: tmpPath, err: err}
    })
}
```

**Message type:**
```go
type editorDoneMsg struct {
    path string
    err  error
}
```

**Handle in Update:**
```go
case editorDoneMsg:
    defer os.Remove(msg.path)
    if msg.err != nil {
        log.Error("editor prompt failed: %v", msg.err)
        return m, nil
    }
    data, err := os.ReadFile(msg.path)
    if err != nil {
        log.Error("editor prompt read: %v", err)
        return m, nil
    }
    text := strings.TrimSpace(string(data))
    if text != "" {
        sendPromptToPane(m.promptTarget, text)
        log.Info("editor prompt sent to %s: %s", m.promptTarget.Name, text[:min(len(text), 80)])
    }
    return m, nil
```

**Required imports in picker.go:** `os`, `os/exec` (add if not present).

---

## Task 3: Config Editor (Ctrl+e)

**What it does:** Opens `~/.config/nagare/config.toml` in `$EDITOR`. Exits the picker first.

### Implementation

**Ctrl+e trigger in handleKey:**
```go
case keyEditConfig:
    return m, m.openConfigEditor()
```

**openConfigEditor method:**
```go
func (m Model) openConfigEditor() tea.Cmd {
    editor := os.Getenv("EDITOR")
    if editor == "" {
        editor = os.Getenv("VISUAL")
    }
    if editor == "" {
        editor = "vi"
    }

    cfgPath := config.DefaultPath()
    // Ensure file exists
    os.MkdirAll(filepath.Dir(cfgPath), 0755)
    if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
        // Write defaults
        cfg := config.Default()
        config.Save(cfg)
    }

    c := exec.Command(editor, cfgPath)
    return tea.ExecProcess(c, func(err error) tea.Msg {
        return configEditDoneMsg{err: err}
    })
}
```

**Message type:**
```go
type configEditDoneMsg struct {
    err error
}
```

**Handle in Update:**
```go
case configEditDoneMsg:
    // Config may have changed — reload theme
    if cfg, err := config.Load(); err == nil {
        theme.Set(cfg.Appearance.Theme)
    }
    return m, nil
```

This keeps the picker running after the editor closes (unlike the Python version which exits). The theme is reloaded in case the user changed it.

---

## Task 4: Update help bar + overlay

**Add to `internal/picker/help.go` helpBar pairs:**
```go
{"Ctrl+l", "Prompt"},
{"Ctrl+g", "Editor"},
{"Ctrl+e", "Config"},
```

**Add to helpOverlay Actions section:**
```go
line("Ctrl+l", "Send inline prompt to session"),
line("Ctrl+g", "Send prompt via $EDITOR"),
line("Ctrl+e", "Edit config file"),
```

---

## Summary of Files

| File | Action | Description |
|------|--------|-------------|
| `internal/picker/keys.go` | Modify | Add keyInlinePrompt, keyEditPrompt, keyEditConfig |
| `internal/picker/picker.go` | Modify | Add promptMode/promptTarget/promptInput fields, handlePromptKey, sendPromptToPane, openEditorPrompt, openConfigEditor, editorDoneMsg, configEditDoneMsg, renderPromptOverlay |
| `internal/picker/help.go` | Modify | Add new keybindings |

---

## Testing

1. **Ctrl+l**: Select a session → type "hello" → Enter → check session pane received "hello"
2. **Ctrl+l + Esc**: Should cancel without sending
3. **Ctrl+l on no sessions**: Should do nothing
4. **Ctrl+g**: Opens $EDITOR with temp .md file → type text → save and quit → text sent to session
5. **Ctrl+g empty file**: Save empty → nothing sent
6. **Ctrl+e**: Opens config.toml in editor → make a change → save → picker resumes with new theme if changed
7. **Multiline Ctrl+l**: Type "line1\nline2" → both lines sent with proper Enter handling
