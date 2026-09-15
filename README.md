<h1 align="center">nagare-go 流れ</h1>
<p align="center">A tmux-integrated session manager for AI coding agents.<br>Monitor, switch, and control multiple Claude Code, Codex, OpenCode, Gemini CLI, Crush, pi, and OhMyPi sessions from a single interface.</p>

<p align="center">
  <img src="images/nagare-logo-glowing.jpg" alt="nagare-go" width="550">
</p>

Go rewrite of [nagare](https://github.com/nmicovic/nagare) — single binary, 3ms startup, no runtime dependencies.

## Features

- **Session Picker** — fuzzy search, list/grid views, live tmux preview
- **Real-time Status** — hooks, plugins, and extensions detect agent state (idle/working/waiting/dead)
- **Worktree Aware** — panes in different git worktrees of one repo show their own path, branch, and name
- **Worktree Launch** — `-w my-feature` or F3 starts an agent in a fresh named worktree, grouped under its repo
- **Notifications** — toast, bell, OS notifications, popup when agents need attention
- **Session Creation** — create new tmux sessions with agents (Ctrl+n, Ctrl+r, CLI)
- **Inline Prompting** — send prompts to agents without leaving the picker (Ctrl+l, Ctrl+g)
- **Inter-Agent Messaging** — MCP server lets agents discover, message, and coordinate with each other (pi has no MCP client, so it gets the same tools through a CLI bridge; OhMyPi uses native MCP)
- **Ticket Orchestration** — run a ticket with any supported agent in a dedicated branch and Nagare-owned worktree, inspect its submitted diff, and safely create or recover a GitHub pull request with durable attempt provenance
- **6 Themes** — tokyonight, catppuccin, dracula, gruvbox, monokai, nord
- **3ms Startup** — compiled Go binary, no runtime dependencies

## Install

```bash
git clone https://github.com/nemke/nagare-go
cd nagare-go
./compile.bash
```

## Setup

```bash
# One command installs everything:
./nagare-go setup
```

This does three things:

1. **Installs status reporting** so each agent notifies nagare on every event (prompt, stop, permission, session start/end). Each agent gets whatever mechanism it supports:

   | Agent | Mechanism |
   |-------|-----------|
   | Claude Code | hooks in `~/.claude/settings.json` |
   | Codex | hooks in `~/.codex/hooks.json` |
   | Gemini CLI | hooks in `~/.gemini/settings.json` |
   | OpenCode | plugin at `~/.config/opencode/plugins/nagare.js` |
   | pi | extension at `~/.pi/agent/extensions/nagare.ts` |
   | OhMyPi (`omp`) | extension at `~/.omp/agent/extensions/nagare.ts` |
2. **Registers the MCP server** in `~/.claude.json`, `~/.codex/config.toml`, `~/.gemini/settings.json`, `~/.config/opencode/opencode.json`, `~/.config/crush/crush.json`, and `~/.omp/agent/mcp.json` — enabling inter-agent messaging and ticket handoff. pi has no MCP client by design, so its extension registers the same tools and routes them through `nagare-go tool`.

3. **Installs messaging workflows** as slash commands (`/nagare-ls`, `/nagare-send`, `/nagare-send-wait`, `/nagare-inbox`) for Claude Code, Gemini CLI, OpenCode, pi, and OhMyPi — and as Agent Skills for Codex and Crush.

Re-running `setup` is safe: it rewrites generated plugin and extension files in place and refreshes every registration.

Codex asks you to review newly installed command hooks once. Open `/hooks` in Codex
and trust the Nagare hooks after setup.

Then add a tmux keybinding to open the picker:

```bash
# Add to ~/.tmux.conf (prefix + g to open picker)
bind g display-popup -w100% -h100% -B -E "/path/to/nagare-go"
```

Reload tmux config: `tmux source-file ~/.tmux.conf`

## Usage

```bash
nagare-go              # open session picker (default)
nagare-go new ~/proj   # create new session with Claude (-a codex|opencode|gemini|crush|pi|omp)
nagare-go new ~/proj -w my-feature   # start an agent in a new named git worktree
nagare-go new myproto  # quick prototype (creates in ~/Prototypes/)
nagare-go board        # cross-project tickets and isolated agent attempts
nagare-go notifs       # notification center + settings
nagare-go setup        # install status reporting + MCP server + slash commands
nagare-go mcp          # run MCP server (stdio, used by agent CLIs)
```

## Board Keybindings

| Key | Action |
|-----|--------|
| `?` | Open the four-page board field guide |
| `h/l` or arrows | Move between columns |
| `1`–`5` | Jump directly to Backlog, Ready, Running, Review, or Done |
| `j/k` or arrows | Select a ticket |
| `[` / `]` | Move ticket left / right |
| `n` | Create ticket |
| `e` | Edit ticket |
| `d` | Run a ticket with a selected agent and optional model in an isolated worktree |
| `v` | Inspect the submitted attempt's stats and scrollable diff |
| `p` | Confirm a non-force push of the recorded branch and create or recover its GitHub pull request |
| `c` | Archive a done ticket's clean worktree after its agent pane closes; retain the branch |
| `a` | Show available agents |
| Enter | Jump to the assigned agent |
| `t` | Toggle Today / All |
| Tab / Shift+Tab | Cycle list / board / grid forward or backward |
| `q` / Esc | Quit |

## Ticket Review and Pull Requests
Press `?` from the board to open the built-in field guide. Use `h/l`, the left
and right arrows, or `1`–`4` to move between Plan, Isolate, Review, and Finish.
The guide explains the complete workflow without leaving Nagare.

Each ticket records its repository and target branch. Press `d` on a Backlog or
Ready ticket to select an agent, then enter a model or leave the model empty to
use that agent's default. Agents with a per-session model option receive it when
their process starts; Crush currently uses its configured default because its
CLI has no per-session model flag. Nagare resolves the target to an immutable
base commit, creates a dedicated branch and managed worktree, launches the agent
there, and keeps the source checkout unchanged.
Managed ticket worktrees live at
`~/.local/share/nagare/workspaces/<attempt-id>/<repository>/`, on branches named
`nagare/<ticket>-<attempt>`. Archiving removes only the clean managed worktree;
the original checkout, branch, commits, pull request, and durable attempt record
remain intact.

When the agent submits the ticket through Nagare, the attempt and ticket move
to Review. The board then supports:

1. Press `v` to inspect the attempt's commit count, dirty-file count, diff
   statistics, and scrollable unified diff from its recorded base.
2. Commit any remaining work. Pull-request creation deliberately refuses dirty
   worktrees and attempts with no commits beyond their base.
3. Press `p` and confirm to push only the recorded branch to the matching
   `origin` branch with a non-force refspec. Nagare creates a GitHub pull request
   against the recorded target, or finds the existing pull request after a
   retry or interrupted creation.
4. Move the reviewed ticket to Done, close its agent pane, then press `c` to
   remove the clean managed worktree. The branch and commits remain intact.

Pull-request creation requires an authenticated
[GitHub CLI](https://cli.github.com/) (`gh`) in `PATH` and an `origin` remote.
Nagare persists the pull-request URL, number, state, and creation time on the
attempt, and shows the pull-request number on the ticket card.

## Picker Keybindings

| Key | Action |
|-----|--------|
| Type | Fuzzy search |
| Enter | Jump to session |
| Esc | Quit |
| Tab / Shift+Tab | Cycle list / board / grid forward or backward |
| Ctrl+y/a | Approve permission |
| Ctrl+f | Star session |
| Ctrl+o | Cycle sort |
| Ctrl+w | Unload agent |
| Ctrl+x | Kill session |
| F2 | Name the selected task |
| F5 | Session note |
| Ctrl+n | New session |
| Ctrl+r | Quick prototype |
| Ctrl+l | Inline prompt |
| Ctrl+g | Editor prompt |
| Ctrl+e | Edit config |
| Ctrl+t | Theme picker |
| F1 | Help |

## Configuration

`~/.config/nagare/config.toml`

```toml
[notifications]
enabled = true

[notifications.needs_input]
toast = true
bell = true
os_notify = true
popup = false

[notifications.task_complete]
toast = true
min_working_seconds = 30

[picker]
show_help_bar = true

[appearance]
theme = "tokyonight"
```

## Architecture

Single Go binary with cobra subcommands. All code in `internal/` packages.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) (TUI), [Lip Gloss](https://github.com/charmbracelet/lipgloss) (styling), and [Cobra](https://github.com/spf13/cobra) (CLI).

Compatible with the Python nagare version — same state files, same JSON schemas, same hook format.

## License

MIT
