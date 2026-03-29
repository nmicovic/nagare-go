Do not mention CLAUDE in commit messages.

# nagare-go

Go rewrite of [nagare](../nagare) — tmux session manager for AI coding agents.

## Build & Test

```bash
go build -o nagare-go .    # build
go test ./... -v           # run all tests
go vet ./...               # lint
```

## Architecture

Single binary with cobra subcommands. All code in `internal/` packages.

- `internal/models` — Session, SessionStatus, AgentType
- `internal/config` — TOML config loading
- `internal/tmux` — scanner (list-panes), status detection (pane scraping)
- `internal/state` — state files + session registry
- `internal/hooks` — hook handler (stdin JSON)
- `internal/notifications` — delivery (toast/bell/os) + persistent store
- `internal/picker` — Bubble Tea TUI
- `internal/mcp` — interface stubs (v2)

## State Files

Compatible with Python version. Same paths, same JSON schema:
- `~/.local/share/nagare/states/*.json`
- `~/.local/share/nagare/sessions.json`
- `~/.local/share/nagare/notifications.json`
- `~/.config/nagare/config.toml`

## Conventions

- Follow Effective Go (go.dev/doc/effective_go)
- Use `gofmt`
- Tests colocated: `foo_test.go` next to `foo.go`
- No underscores in names — MixedCaps for exported, mixedCaps for unexported
- Always check errors
- Tokyonight color palette: idle=#00D26A, running=#e0af68, waiting=#db4b4b, dead=#565f89
