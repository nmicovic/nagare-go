Do not mention any AI agent (Claude, Gemini, Crush, OpenCode, etc.) in commit messages.
Do not commit without explicit user permission.

# nagare-go

Go rewrite of [nagare](../nagare) — tmux session manager for AI coding agents.

## Build & Test

```bash
./compile.bash             # build (stripped binary)
go build -o nagare-go .    # build (debug)
go test ./... -v           # run all tests
go vet ./...               # lint
```

## Commands

```bash
nagare-go                  # launch picker (default)
nagare-go pick             # launch picker
nagare-go hook-state       # handle agent hook/plugin/extension events (stdin JSON)
nagare-go setup            # install status reporting + MCP server + slash commands
nagare-go notifs           # notification center TUI
nagare-go popup-notif      # popup notification (called by hooks)
nagare-go new [path]       # create new agent session
nagare-go new <repo> -w <name>  # create a named git worktree and start an agent in it
nagare-go mcp              # run MCP server (stdio, for agent CLIs)
nagare-go tool <name> [json]  # invoke a messaging tool directly (hidden; for pi)
```

## Architecture

Single binary with cobra subcommands. All code in `internal/` packages.

- `internal/models` — Session, SessionStatus, AgentType (claude, opencode, gemini, crush, pi)
- `internal/config` — TOML config loading + saving
- `internal/tmux` — scanner (list-panes + /proc descendant walk), per-pane paths and worktree resolution, status detection (pane scraping)
- `internal/git` — resolves a directory into branch, repo name, and worktree name (one `rev-parse` per path)
- `internal/state` — state files + session registry
- `internal/hooks` — hook handler (stdin JSON → state files → notifications)
- `internal/notifications` — delivery (toast/bell/os/popup) + persistent store
- `internal/picker` — Bubble Tea TUI (list/grid views, overlays, keybindings)
- `internal/notifs` — notification center TUI
- `internal/popup` — popup notification TUI
- `internal/session` — session creation + path resolution
- `internal/newsession` — new session + quick prototype forms
- `internal/theme` — 6 themes with AdaptiveColor, self-registering via init()
- `internal/setup` — status reporting + MCP + slash command installation for every agent
- `internal/mcp` — MCP server for inter-agent messaging, plus the CLI tool bridge
- `internal/bin` — shared binary finder
- `internal/fsutil` — atomic file writes
- `internal/log` — file logger (~/.local/share/nagare/nagare-go.log)

### Worktrees

Each agent pane resolves its own directory from `pane_current_path`, not the tmux
session path, so panes in different worktrees of one repo get their own path, branch,
and name (`{session}/{worktree}`). Worktree detection is structural — the git common
dir's parent differs from the toplevel — so hand-made worktrees work the same as
Claude Code's `.claude/worktrees/`.

Display-name precedence is: a window name the user set, then the worktree, then
`{agent}_NN`.

Worktree sessions are created by `session.CreateWorktree`. Claude Code has its own
`-w <name>` flag, so it is handed the flag and creates the worktree under
`.claude/worktrees/`; every other agent has no equivalent, so nagare runs
`git worktree add` into `.worktrees/` and opens the pane there. `--tmux` is never
passed to Claude — nagare already made the window.

A worktree pane always joins its repo's existing tmux session as a new window, which is
what lets the picker group them. Sessions are matched to a repo through
`git.MainRoot`, not by literal path, because a session's own directory is often a
worktree.

Because worktrees are windows in one session, Ctrl+x kills the *window* whenever a
session holds more than one agent pane — killing the session would take a repo's other
worktrees with it. On a worktree pane it then offers removal; `git.RemoveWorktree` never
passes `--force`, so git refuses while there is uncommitted work, and the branch is left
intact.

Worktree creation runs as a `tea.Cmd`, never inline: doing it in the update loop froze
the TUI for seconds. A spinner shows until the new agent pane appears in a scan
(`pendingWorktree.satisfiedBy`) rather than until nagare's own work returns, because
`claude -w` needs seconds more to build the worktree and start. Failures surface in the
status line instead of only reaching the log.

Removal is confirmed by a centred dialog (`renderConfirmOverlay`) drawn with
`placeOverlay`, like the help and theme overlays; only `y`/`n`/`esc` answer it. Claude
locks the worktrees it creates, so `git.RemoveWorktree` checks cleanliness itself,
unlocks, then removes without `--force` — passing `--force` would override the dirty
guard too.

The detail pane shows outstanding work (`git.WorkStatus`) and warns when two agents
share one directory. Work is cached per path and refreshed per scan: the pane
re-renders every frame for the status-dot pulse, so git must never be called from
render.

In the list view, sessions sharing a tmux session name are grouped under a single
header row naming the repo, and children show only their own name — so the repo
prefix is not repeated on every row. Grouping starts at two members; a lone session
renders as a plain row. A group takes the position of its most urgent member, so a
waiting worktree lifts its whole repo. Rows are derived per frame by
`picker.buildRows`; the cursor keeps indexing sessions, not rows. Grid view stays flat.

## Agent Integrations

Every agent reports status through one interface: `nagare-go hook-state` reading a JSON
object on stdin with `hook_event_name`, `session_id`, `cwd`, and optionally
`last_assistant_message` / `notification_type`, plus `TMUX_PANE` from the environment.
Agent-native event names are mapped to states centrally in `hooks.EventToState` — never
per-agent elsewhere.

| Agent | Status reporting | Messaging tools |
|-------|------------------|-----------------|
| Claude Code | hooks in `~/.claude/settings.json` | MCP (`~/.claude.json`) |
| Gemini CLI | hooks in `~/.gemini/settings.json` | MCP (`~/.gemini/settings.json`) |
| OpenCode | plugin `~/.config/opencode/plugins/nagare.js` | MCP (`~/.config/opencode/opencode.json`) |
| Crush | none | MCP (`~/.config/crush/crush.json`) |
| pi | extension `~/.pi/agent/extensions/nagare.ts` | `nagare-go tool` bridge (pi has no MCP) |

pi has no MCP client by design, so its extension registers the five nagare tools and
shells out to `nagare-go tool <name> <json>`, which calls the same handlers the MCP
server calls. pi also has no permission prompts, so pi sessions never reach
`waiting_input`.

Generated plugin/extension files are rewritten on every `nagare-go setup`, so edits
belong in `internal/setup`, not in the installed files.

## Picker Keybindings

| Key | Action |
|-----|--------|
| Type | Fuzzy search sessions |
| Enter | Jump to selected session |
| Esc | Quit |
| ↑/↓ | Navigate |
| Tab | Toggle list/grid view |
| Ctrl+y | Approve permission |
| Ctrl+a | Approve always |
| Ctrl+f | Toggle star |
| Ctrl+o | Cycle sort mode |
| Ctrl+w | Unload agent pane |
| Ctrl+x | Kill the pane's window (or the session if it is the only agent pane); offers to remove a worktree |
| F2 | Rename session |
| F3 | New git worktree for this repo |
| Ctrl+n | New session form |
| Ctrl+r | Quick prototype |
| Ctrl+l | Inline prompt |
| Ctrl+g | Editor prompt ($EDITOR) |
| Ctrl+e | Edit config |
| Ctrl+t | Theme picker |
| Ctrl+b | Mailbox viewer |
| F1 | Help overlay |

## State Files

Compatible with Python version. Same paths, same JSON schema:
- `~/.local/share/nagare/states/*.json`
- `~/.local/share/nagare/sessions.json`
- `~/.local/share/nagare/notifications.json`
- `~/.local/share/nagare/messages/` (MCP inter-agent)
- `~/.local/share/nagare/nagare-go.log`
- `~/.config/nagare/config.toml`

## Themes

6 themes with light/dark support (AdaptiveColor):
tokyonight (default), catppuccin, dracula, gruvbox, monokai, nord

Styles are functions (not cached) — theme switches take effect immediately.

## Conventions

- Follow Effective Go (go.dev/doc/effective_go)
- Use `gofmt`
- Tests colocated: `foo_test.go` next to `foo.go`
- No underscores in names — MixedCaps for exported, mixedCaps for unexported
- Always check errors
- Atomic writes for shared state files (write-to-temp-then-rename)
- Tokyonight color palette: idle=#00D26A, running=#e0af68, waiting=#db4b4b, dead=#565f89
