# WP5: MCP Server (Inter-Agent Messaging) — Implementation Plan

**Goal:** Expose nagare-go as an MCP tool server so Claude Code sessions can discover each other, send messages, and coordinate work.

**Dependency:** `github.com/modelcontextprotocol/go-sdk` (official Go MCP SDK, published March 2026)

**Codebase context:**
- MCP stubs: `internal/mcp/mcp.go` — existing interface + Message/AgentInfo types (need updating)
- Scanner: `internal/tmux/scanner.go` — `ScanSessions()` returns `[]models.Session`
- State: `internal/state/` — `LoadAllStates()`, `DefaultStatesDir()`, `Registry`
- Models: `internal/models/models.go` — `Session`, `SessionStatus`, `AgentType`
- Tmux: `internal/tmux/tmux.go` — `RunTmux()`, `PaneTarget()`
- Setup: `internal/setup/setup.go` — hook installation (needs MCP registration)
- CLI: `main.go` — cobra commands
- Bin: `internal/bin/bin.go` — `FindSelf()`

---

## Architecture

```
Claude Code Session A                    Claude Code Session B
        │                                        │
        ▼                                        ▼
  nagare-go mcp (stdio)                    nagare-go mcp (stdio)
        │                                        │
        ├── list_agents() ──► tmux scanner        │
        ├── send_message() ──► write JSON ──► nudge via tmux send-keys
        ├── send_message_and_wait() ──► write JSON ──► nudge ──► poll file
        ├── check_messages() ──► read inbox dir   │
        └── reply() ──► update JSON file          │
                                                  │
                    ~/.local/share/nagare/messages/
                    ├── session-A/
                    │   └── msg_abc123.json
                    └── session-B/
                        └── msg_def456.json
```

Each Claude Code session runs its own `nagare-go mcp` process (stdio). Messages are exchanged via JSON files on disk, with tmux send-keys used to nudge the target agent.

---

## Task 1: Install Go MCP SDK

```bash
go get github.com/modelcontextprotocol/go-sdk@latest
```

Check the API — the official SDK uses this pattern:
```go
server := mcp.NewServer(&mcp.Implementation{Name: "nagare", Version: "1.0.0"}, nil)
mcp.AddTool(server, &mcp.Tool{Name: "tool_name", Description: "..."}, handlerFunc)
server.Run(context.Background(), &mcp.StdioTransport{})
```

If the official SDK API differs from this (it's new), adapt accordingly. The `mark3labs/mcp-go` SDK is the fallback:
```go
s := server.NewMCPServer("nagare", "1.0.0")
s.AddTool(mcp.NewTool("tool_name", mcp.WithDescription("...")), handler)
server.ServeStdio(s)
```

---

## Task 2: Message types and file I/O

**Update `internal/mcp/mcp.go`** — replace the existing stubs with real types:

```go
package mcp

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "time"

    "github.com/google/uuid"
    "github.com/nemke/nagare-go/internal/fsutil"
)

// Message is an inter-agent message stored as a JSON file.
type Message struct {
    ID           string  `json:"id"`
    FromSession  string  `json:"from_session"`
    ToSession    string  `json:"to_session"`
    Content      string  `json:"content"`
    ExpectsReply bool    `json:"expects_reply"`
    Status       string  `json:"status"`       // "pending", "delivered", "completed"
    Response     *string `json:"response"`      // nil until reply
    CreatedAt    string  `json:"created_at"`
    RespondedAt  *string `json:"responded_at"`  // nil until reply
}

// MessagesDir returns the base messages directory.
func MessagesDir() string {
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".local", "share", "nagare", "messages")
}

// InboxDir returns a session's inbox directory.
func InboxDir(sessionName string) string {
    return filepath.Join(MessagesDir(), sessionName)
}

// MessagePath returns the file path for a message.
func MessagePath(toSession, msgID string) string {
    return filepath.Join(InboxDir(toSession), fmt.Sprintf("msg_%s.json", msgID))
}

// WriteMessage writes a message to the target's inbox.
func WriteMessage(msg Message) error {
    dir := InboxDir(msg.ToSession)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }
    data, err := json.MarshalIndent(msg, "", "  ")
    if err != nil {
        return err
    }
    return fsutil.AtomicWrite(MessagePath(msg.ToSession, msg.ID), data, 0644)
}

// ReadMessage reads a message from disk.
func ReadMessage(toSession, msgID string) (Message, error) {
    data, err := os.ReadFile(MessagePath(toSession, msgID))
    if err != nil {
        return Message{}, err
    }
    var msg Message
    if err := json.Unmarshal(data, &msg); err != nil {
        return Message{}, err
    }
    return msg, nil
}

// ListInbox reads all messages in a session's inbox.
func ListInbox(sessionName string) ([]Message, error) {
    dir := InboxDir(sessionName)
    entries, err := os.ReadDir(dir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, nil
        }
        return nil, err
    }
    var msgs []Message
    for _, e := range entries {
        if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
            continue
        }
        data, err := os.ReadFile(filepath.Join(dir, e.Name()))
        if err != nil {
            continue
        }
        var msg Message
        if err := json.Unmarshal(data, &msg); err != nil {
            continue
        }
        msgs = append(msgs, msg)
    }
    return msgs, nil
}

// NewMessageID generates a short unique message ID.
func NewMessageID() string {
    return uuid.New().String()[:12]
}
```

---

## Task 3: MCP tool handlers

**Create `internal/mcp/tools.go`:**

### list_agents

Scans tmux for agent sessions, returns formatted list excluding self.

```go
func ListAgentsHandler(mySession string) string {
    hookStates := state.LoadAllStates(state.DefaultStatesDir())
    sessions := tmux.ScanSessions(hookStates)

    var lines []string
    for _, s := range sessions {
        if s.Name == mySession {
            continue // exclude self
        }
        lines = append(lines, fmt.Sprintf("- %s (%s) [%s] %s",
            s.Name, models.AgentLabel(s.AgentType),
            models.StatusLabel(s.Status), s.Path))
    }
    if len(lines) == 0 {
        return "No other agents found."
    }
    return strings.Join(lines, "\n")
}
```

### send_message

Validates target is IDLE, writes message file, nudges via tmux send-keys.

```go
func SendMessageHandler(mySession, target, message string) string {
    session, err := findSession(target)
    if err != nil {
        return fmt.Sprintf("Error: %v", err)
    }
    if session.Status != models.StatusIdle {
        return fmt.Sprintf("Error: %s is %s, not idle. Wait for it to finish.", target, models.StatusLabel(session.Status))
    }

    msg := Message{
        ID:           NewMessageID(),
        FromSession:  mySession,
        ToSession:    target,
        Content:      message,
        ExpectsReply: false,
        Status:       "pending",
        CreatedAt:    time.Now().UTC().Format(time.RFC3339),
    }
    if err := WriteMessage(msg); err != nil {
        return fmt.Sprintf("Error writing message: %v", err)
    }

    // Nudge target
    nudge := fmt.Sprintf("FYI from '%s': %s -- This is informational only. No reply needed. Continue your current work.", mySession, truncate(message, 200))
    paneTarget := tmux.PaneTarget(session.Name, session.WindowIndex, session.PaneIndex)
    tmux.RunTmux("send-keys", "-t", paneTarget, nudge, "Enter")

    msg.Status = "delivered"
    WriteMessage(msg)

    return fmt.Sprintf("Message sent to %s.", target)
}
```

### send_message_and_wait

Same as send_message but with `expects_reply=true` and a polling loop.

```go
func SendMessageAndWaitHandler(mySession, target, message string, timeout int) string {
    session, err := findSession(target)
    if err != nil {
        return fmt.Sprintf("Error: %v", err)
    }
    if session.Status != models.StatusIdle {
        return fmt.Sprintf("Error: %s is %s, not idle.", target, models.StatusLabel(session.Status))
    }

    msg := Message{
        ID:           NewMessageID(),
        FromSession:  mySession,
        ToSession:    target,
        Content:      message,
        ExpectsReply: true,
        Status:       "pending",
        CreatedAt:    time.Now().UTC().Format(time.RFC3339),
    }
    if err := WriteMessage(msg); err != nil {
        return fmt.Sprintf("Error: %v", err)
    }

    // Nudge target
    nudge := fmt.Sprintf("You have a message from '%s' that requires a reply. Call check_messages() to read it, then reply() to respond.", mySession)
    paneTarget := tmux.PaneTarget(session.Name, session.WindowIndex, session.PaneIndex)
    tmux.RunTmux("send-keys", "-t", paneTarget, nudge, "Enter")

    msg.Status = "delivered"
    WriteMessage(msg)

    // Poll for response
    deadline := time.Now().Add(time.Duration(timeout) * time.Second)
    for time.Now().Before(deadline) {
        time.Sleep(2 * time.Second)
        updated, err := ReadMessage(target, msg.ID)
        if err != nil {
            continue
        }
        if updated.Status == "completed" && updated.Response != nil {
            return *updated.Response
        }
    }

    return fmt.Sprintf("Timeout: %s did not respond within %d seconds.", target, timeout)
}
```

### check_messages

Returns pending incoming messages + completed outgoing responses.

```go
func CheckMessagesHandler(mySession string) string {
    var parts []string

    // Incoming messages (my inbox)
    inbox, _ := ListInbox(mySession)
    var pending []Message
    for _, m := range inbox {
        if m.Status == "pending" || m.Status == "delivered" {
            pending = append(pending, m)
        }
    }
    if len(pending) > 0 {
        parts = append(parts, "=== Incoming Messages ===")
        for _, m := range pending {
            replyNote := ""
            if m.ExpectsReply {
                replyNote = fmt.Sprintf(" [REPLY NEEDED - use reply('%s', 'your response')]", m.ID)
            }
            parts = append(parts, fmt.Sprintf("From: %s%s\n%s", m.FromSession, replyNote, m.Content))
        }
    }

    // Completed outgoing responses (scan all inboxes for messages FROM me)
    baseDir := MessagesDir()
    dirs, _ := os.ReadDir(baseDir)
    var responses []Message
    for _, d := range dirs {
        if !d.IsDir() || d.Name() == mySession {
            continue
        }
        msgs, _ := ListInbox(d.Name())
        for _, m := range msgs {
            if m.FromSession == mySession && m.Status == "completed" && m.Response != nil {
                responses = append(responses, m)
            }
        }
    }
    if len(responses) > 0 {
        parts = append(parts, "=== Responses to Your Messages ===")
        for _, m := range responses {
            parts = append(parts, fmt.Sprintf("Response from %s:\n%s", m.ToSession, *m.Response))
        }
    }

    if len(parts) == 0 {
        return "No pending messages or responses."
    }
    return strings.Join(parts, "\n\n")
}
```

### reply

```go
func ReplyHandler(mySession, messageID, content string) string {
    inbox, _ := ListInbox(mySession)
    for _, m := range inbox {
        if m.ID == messageID {
            now := time.Now().UTC().Format(time.RFC3339)
            m.Status = "completed"
            m.Response = &content
            m.RespondedAt = &now
            if err := WriteMessage(m); err != nil {
                return fmt.Sprintf("Error saving reply: %v", err)
            }
            return fmt.Sprintf("Reply sent to %s.", m.FromSession)
        }
    }
    return fmt.Sprintf("Error: message %s not found in your inbox.", messageID)
}
```

### Helper functions

```go
func findSession(name string) (models.Session, error) {
    hookStates := state.LoadAllStates(state.DefaultStatesDir())
    sessions := tmux.ScanSessions(hookStates)
    for _, s := range sessions {
        if s.Name == name {
            return s, nil
        }
    }
    return models.Session{}, fmt.Errorf("session '%s' not found", name)
}

func truncate(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen] + "..."
}

// resolveMySession determines the current session name from cwd.
func resolveMySession() string {
    cwd, _ := os.Getwd()
    reg := state.NewRegistry(state.DefaultRegistryPath())
    if s := reg.FindByPath(cwd); s != nil {
        return s.Name
    }
    // Fallback: scan tmux sessions for matching path
    hookStates := state.LoadAllStates(state.DefaultStatesDir())
    sessions := tmux.ScanSessions(hookStates)
    for _, s := range sessions {
        if s.Path == cwd {
            return s.Name
        }
    }
    return filepath.Base(cwd)
}
```

---

## Task 4: MCP server entry point

**Create `internal/mcp/server.go`:**

Wire the tools into an MCP server that runs on stdio.

```go
package mcp

import (
    "context"
    "log"

    gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func RunServer() error {
    mySession := resolveMySession()

    server := gomcp.NewServer(&gomcp.Implementation{
        Name:    "nagare",
        Version: "1.0.0",
    }, nil)

    // Register tools — adapt to actual SDK API
    // Each tool handler receives input params and returns a string result

    gomcp.AddTool(server, &gomcp.Tool{
        Name:        "list_agents",
        Description: "List all active AI agent sessions with their status",
    }, func(ctx context.Context, req *gomcp.CallToolRequest, input struct{}) (*gomcp.CallToolResult, string, error) {
        result := ListAgentsHandler(mySession)
        return nil, result, nil
    })

    // ... register send_message, send_message_and_wait, check_messages, reply
    // Each follows the same pattern with appropriate input structs

    return server.Run(context.Background(), &gomcp.StdioTransport{})
}
```

**IMPORTANT:** The exact SDK API may differ. The dev agent should:
1. Run `go get github.com/modelcontextprotocol/go-sdk@latest`
2. Check `go doc github.com/modelcontextprotocol/go-sdk/mcp` for the actual API
3. If the official SDK doesn't work, fall back to `github.com/mark3labs/mcp-go`:
   ```go
   s := server.NewMCPServer("nagare", "1.0.0")
   s.AddTool(mcp.NewTool("list_agents", mcp.WithDescription("...")), handler)
   server.ServeStdio(s)
   ```
4. The key requirement: stdio transport, 5 tools, return strings

**Input structs for tools:**

```go
type SendMessageInput struct {
    Target  string `json:"target" jsonschema:"description=target session name"`
    Message string `json:"message" jsonschema:"description=message to send"`
}

type SendMessageAndWaitInput struct {
    Target  string `json:"target" jsonschema:"description=target session name"`
    Message string `json:"message" jsonschema:"description=message to send"`
    Timeout int    `json:"timeout,omitempty" jsonschema:"description=timeout in seconds,default=120"`
}

type ReplyInput struct {
    MessageID string `json:"message_id" jsonschema:"description=ID of the message to reply to"`
    Content   string `json:"content" jsonschema:"description=reply content"`
}
```

---

## Task 5: Wire `nagare-go mcp` CLI command

**Update `main.go`:**

```go
mcpCmd := &cobra.Command{
    Use:   "mcp",
    Short: "Run MCP server for inter-agent messaging",
    RunE: func(cmd *cobra.Command, args []string) error {
        return mcp.RunServer()
    },
}
rootCmd.AddCommand(..., mcpCmd)
```

---

## Task 6: Register MCP server in setup

**Update `internal/setup/setup.go`** — add MCP registration to `Run()`:

After installing hooks, register the MCP server in `~/.claude.json`:

```go
func installMCPServer(home string) error {
    claudeJSON := filepath.Join(home, ".claude.json")
    data, _ := os.ReadFile(claudeJSON)

    var config map[string]interface{}
    if len(data) > 0 {
        json.Unmarshal(data, &config)
    }
    if config == nil {
        config = make(map[string]interface{})
    }

    servers, _ := config["mcpServers"].(map[string]interface{})
    if servers == nil {
        servers = make(map[string]interface{})
    }

    nagareBin := bin.FindSelf()
    servers["nagare"] = map[string]interface{}{
        "command": nagareBin,
        "args":    []string{"mcp"},
    }
    config["mcpServers"] = servers

    out, _ := json.MarshalIndent(config, "", "  ")
    return os.WriteFile(claudeJSON, out, 0644)
}
```

Call from `Run()`:
```go
if err := installMCPServer(home); err != nil {
    return fmt.Errorf("failed to register MCP server: %w", err)
}
fmt.Printf("  MCP server registered: ~/.claude.json\n")
```

---

## Task 7: Mailbox viewer in picker (Ctrl+b)

**Add to picker:** A simple overlay showing messages for the selected session.

**Add to keys.go:**
```go
keyMailbox = "ctrl+b"
```

**In handleKey:**
```go
case keyMailbox:
    if s, ok := m.selectedSession(); ok {
        m.showMailbox = true
        m.mailboxSession = s.Name
        msgs, _ := mcp.ListInbox(s.Name)
        m.mailboxMessages = msgs
    }
    return m, nil
```

**Render as overlay (similar to help/theme):**
- Show list of messages: from, content preview, status, reply needed indicator
- Esc to close
- Simple read-only view for v1

---

## Summary of Files

| File | Action | Description |
|------|--------|-------------|
| `internal/mcp/mcp.go` | Rewrite | Message type, file I/O (WriteMessage, ReadMessage, ListInbox) |
| `internal/mcp/tools.go` | Create | 5 tool handlers + helper functions |
| `internal/mcp/server.go` | Create | MCP server entry point with stdio transport |
| `internal/setup/setup.go` | Modify | Add MCP server registration in ~/.claude.json |
| `internal/picker/keys.go` | Modify | Add keyMailbox |
| `internal/picker/picker.go` | Modify | Add mailbox overlay (Ctrl+b) |
| `internal/picker/help.go` | Modify | Add Ctrl+b to help |
| `main.go` | Modify | Add `mcp` command |
| `go.mod` | Modify | Add MCP SDK dependency |

---

## Implementation Order

1. **Task 1** — Install SDK dependency
2. **Task 2** — Message types and file I/O
3. **Task 3** — Tool handlers (can test independently)
4. **Task 4** — Server entry point
5. **Task 5** — Wire CLI command
6. **Task 6** — Setup registration
7. **Task 7** — Mailbox viewer (optional, can defer)

---

## Testing

1. **SDK check:** `go get` the SDK, verify it compiles
2. **Message I/O:** Write/read a message, verify JSON format matches Python
3. **list_agents:** Run `echo '{"method":"tools/call","params":{"name":"list_agents"}}' | ./nagare-go mcp` — should list sessions
4. **send_message:** Send from session A to B, verify file created in B's inbox, nudge appears in B's pane
5. **check_messages:** Verify B sees the message
6. **reply:** B replies, verify A can read the response
7. **send_message_and_wait:** Full round-trip with timeout
8. **Setup:** Run `nagare-go setup`, verify `~/.claude.json` has nagare MCP entry
9. **Real test:** Open two Claude Code sessions, use list_agents from one, send_message to the other
