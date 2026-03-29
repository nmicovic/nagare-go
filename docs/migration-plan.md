# nagare → nagare-go Migration Plan

Full feature migration from the Python nagare implementation to Go.

## Current State

The Go rewrite covers ~30% of Python features:
- Core picker TUI (list + grid views, fuzzy search, preview)
- Hook processing + notification delivery (toast/bell/os)
- Config system (TOML with smart merging)
- Theme system (6 themes, live switching via Ctrl+t dialog)
- State management (session states + registry)
- tmux integration (scanner, pane capture, agent detection)
- Setup command (Claude Code hook installation)
- Help bar + F1 help overlay
- Logging

## Work Packages

### WP1: Picker Actions (Quick Wins)
**Priority:** Highest — makes the tool immediately usable for daily work.

Keybindings that need implementing in the picker:
- **Ctrl+y** — approve permission (send "y" + Enter to pane) *(partially done)*
- **Ctrl+a** — approve always (send Down + Enter to pane) *(partially done)*
- **Ctrl+w** — unload agent (kill the agent's pane)
- **Ctrl+x** — kill entire tmux session
- **Ctrl+f** — toggle star/favorite on selected session
- **Ctrl+o** — cycle sort mode (status → name → agent)
- **F2** — rename session inline

**Deps:** None.

---

### WP2: Notification Center + Popup Notifications
**Priority:** High — completes the notification story.

Two new TUI screens:
- `nagare-go notifs` — notification history viewer with settings tab
- Popup notification — auto-shown via tmux when agent needs input or finishes task

**Deps:** Notification store (done).

---

### WP3: Session Creation
**Priority:** High — core workflow for creating new agent sessions.

- `nagare-go new [path]` CLI command with optional interactive form
- **Ctrl+n** in picker — new session form (path autocomplete, name, agent selector)
- **Ctrl+r** in picker — quick prototype (name + agent, creates in ~/Prototypes/)
- Auto-registers created sessions in the registry

**Deps:** Registry (done).

---

### WP4: Inline Prompting + Editor Integration
**Priority:** Medium — power-user features.

- **Ctrl+l** — text input overlay, sends typed text to selected session's pane
- **Ctrl+g** — launch $EDITOR with temp file, send contents on save
- **Ctrl+e** — open config.toml in $EDITOR

**Deps:** WP1 (pane targeting patterns).

---

### WP5: MCP Server (Inter-Agent Messaging)
**Priority:** Medium — differentiating feature, but depends on Go MCP SDK.

- 5 MCP tools: list_agents, send_message, send_message_and_wait, check_messages, reply
- File-based mailbox: `~/.local/share/nagare/messages/{session}/msg_{id}.json`
- **Ctrl+b** in picker — mailbox viewer overlay
- Setup registers MCP server in `~/.claude.json`

**Deps:** Go MCP SDK evaluation.

---

### WP6: Sound Engine
**Priority:** Low — nice-to-have, not core.

- CESP/openpeon sound pack support
- Platform-aware audio player detection
- `nagare-go sounds list|install|test` CLI commands
- Fire-and-forget async playback with debouncing
- Per-category toggles, per-session pack overrides

**Deps:** Hook handler (done).

---

### WP7: Voice Engine
**Priority:** Low — nice-to-have, not core.

- TTS engine auto-detection (say, piper, edge-tts, espeak-ng, WSL SAPI)
- Message templates with `{session}` placeholder
- Per-category toggles, per-session voice overrides
- Async playback with 2s debounce

**Deps:** Hook handler (done).

---

### WP8: Token Tracking + History
**Priority:** Low — informational, not blocking.

- Parse Claude Code transcript JSONL files for token usage
- Display in session details panel
- Read `~/.claude/history.jsonl` for last user message per project

**Deps:** None.

---

## Recommended Execution Order

```
WP1 (actions)  ──→  WP3 (session creation)  ──→  WP4 (prompting)
      │
      └──→  WP2 (notifications UI)

WP6 (sounds)  ──→  WP7 (voice)     [independent track]

WP5 (MCP)                            [independent, can defer]
WP8 (tokens/history)                  [independent, low priority]
```

## Detailed Plans

Each WP has a detailed implementation plan in `docs/plans/`:
- [WP1: Picker Actions](plans/wp1-picker-actions.md)
