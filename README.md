<h1 align="center">nagare-go 流れ</h1>
<p align="center">A tmux-integrated session manager for AI coding agents.<br>Monitor, switch, and control multiple Claude Code, OpenCode, and Gemini CLI sessions from a single interface.</p>

<p align="center">
  <img src="images/nagare-logo-glowing.jpg" alt="nagare-go" width="550">
</p>

Go rewrite of [nagare](https://github.com/nmicovic/nagare) — single binary, 3ms startup, no runtime dependencies.

## Features

- **Session Picker** — fuzzy search, list/grid views, live tmux preview
- **Real-time Status** — hooks detect agent state (idle/working/waiting/dead)
- **Notifications** — toast, bell, OS notifications, popup when agents need attention
- **Session Creation** — create new tmux sessions with agents (Ctrl+n, Ctrl+r, CLI)
- **Inline Prompting** — send prompts to agents without leaving the picker (Ctrl+l, Ctrl+g)
- **Inter-Agent Messaging** — MCP server lets agents discover, message, and coordinate with each other
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

This does two things:
1. **Installs hooks** into `~/.claude/settings.json` — Claude Code will notify nagare on every event (prompt, stop, permission, session start/end)
2. **Registers MCP server** in `~/.claude.json` — enables inter-agent messaging (list_agents, send_message, check_messages, reply)

Then add a tmux keybinding to open the picker:

```bash
# Add to ~/.tmux.conf (prefix + g to open picker)
bind g display-popup -w100% -h100% -B -E "/path/to/nagare-go"
```

Reload tmux config: `tmux source-file ~/.tmux.conf`

## Usage

```bash
nagare-go              # open session picker (default)
nagare-go new ~/proj   # create new session with Claude
nagare-go new myproto  # quick prototype (creates in ~/Prototypes/)
nagare-go notifs       # notification center + settings
nagare-go setup        # install hooks + MCP server
nagare-go mcp          # run MCP server (stdio, used by Claude Code)
```

## Picker Keybindings

| Key | Action |
|-----|--------|
| Type | Fuzzy search |
| Enter | Jump to session |
| Esc | Quit |
| Tab | Toggle list/grid |
| Ctrl+y/a | Approve permission |
| Ctrl+f | Star session |
| Ctrl+o | Cycle sort |
| Ctrl+w | Unload agent |
| Ctrl+x | Kill session |
| F2 | Rename |
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
