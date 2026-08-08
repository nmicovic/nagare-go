# pi agent support, and bringing OpenCode + Claude up to date

Date: 2026-08-07

## Goal

Add first-class support for [pi](https://github.com/earendil-works/pi) alongside the
existing four agents, and refresh the OpenCode and Claude Code integrations against
their current upstream contracts.

## Findings

### pi (earendil-works/pi)

| Thing | Value |
|-------|-------|
| Binary | `pi` (npm `@earendil-works/pi-coding-agent`, or install.sh binary) |
| Continue flag | `-c` / `--continue` |
| Global settings | `~/.pi/agent/settings.json` |
| Extensions (auto-discovered) | `~/.pi/agent/extensions/*.ts`, `~/.pi/agent/extensions/*/index.ts` |
| Prompt templates | `~/.pi/agent/prompts/*.md` → `/name`, frontmatter `description`, `$ARGUMENTS` |
| MCP | **None.** Deliberate: pi "intentionally does not include built-in MCP, sub-agents, permission popups, plan mode, to-dos, or background bash" |

pi's extension event system is the integration surface. The docs state directly:
"Use `agent_settled` for status integrations that need to know Pi will not continue
running automatically." Extensions can also register LLM-callable tools
(`pi.registerTool`) and spawn processes (`pi.exec`).

Because pi has no MCP client, nagare's five messaging tools cannot arrive over MCP.
They are registered by the nagare pi extension instead, each one shelling out to a
new `nagare-go tool <name> <json>` bridge that calls the same handlers the MCP server
calls. No protocol logic is duplicated in TypeScript.

pi has no permission prompts, so pi sessions never enter `waiting_input`.

### OpenCode (anomalyco/opencode)

Two things are out of date:

1. **Wrong MCP config path.** nagare writes `~/.config/opencode/config.json`; current
   OpenCode reads `~/.config/opencode/opencode.json`. nagare's MCP registration has
   been silently inert on current OpenCode versions. The `mcp.<name> = {type: "local",
   command: [...], enabled: true}` schema nagare writes is still correct.
2. **Unused plugin system.** OpenCode has `~/.config/opencode/plugins/*.js` with events
   `session.idle`, `session.status`, `session.error`, `permission.asked`,
   `permission.replied`, `tool.execute.before`, `session.created`. nagare uses none of
   them, so OpenCode panes always render as "Idle" — status detection never worked.

`~/.config/opencode/commands/` (plural) is already correct.

### Claude Code

`last_assistant_message` on `Stop` and the `Notification` matcher on notification type
are both still correct. The event list has grown; these are worth adding:

| Event | State | Why |
|-------|-------|-----|
| `PermissionRequest` | `waiting_input` | Fires exactly when a tool call needs a decision — a precise signal, where `Notification`/`permission_prompt` is a side effect of the UI |
| `Elicitation` | `waiting_input` | MCP server requesting user input |
| `ElicitationResult` | `working` | Input supplied, turn resumes |
| `PostToolUse` | `working` | Keeps state fresh through long turns |
| `PreCompact` / `PostCompact` | `working` | Compaction is not idleness |
| `StopFailure` | `idle` | Turn died on an API error; currently falls through to `unknown` |

Adding `PermissionRequest` next to `Notification` means two events can now request the
same `waiting_input` state, so `ShouldNotify` must not fire `needs_input` twice.

## Design

### State plumbing (unchanged contract)

Every agent drives nagare through one interface: `nagare-go hook-state` reading a JSON
object on stdin with `hook_event_name`, `session_id`, `cwd`, `last_assistant_message`,
`notification_type`, plus `TMUX_PANE` from the environment. Agent-native event names are
mapped centrally in `hooks.EventToState` — the same way Gemini's `BeforeTool`/`AfterAgent`
already are. No per-agent state logic anywhere else.

pi events: `agent_start`, `before_agent_start`, `turn_start` → working;
`agent_settled` → idle; `session_start` → idle; `session_shutdown` → dead.

OpenCode events: `session.status`, `tool.execute.before`, `permission.replied` → working;
`session.idle`, `session.error`, `session.created` → idle; `permission.asked` →
waiting_input.

### Tool bridge

`nagare-go tool <name> [json]` dispatches to the existing `internal/mcp` handlers and
prints the text result. Same `resolveMySession()` self-identification as the MCP server,
which works because `pi.exec` inherits pi's `TMUX_PANE` and cwd. The command is hidden —
it is an integration surface, not a user-facing one.

### Setup additions

- `~/.pi/agent/extensions/nagare.ts` — status events + five registered tools
- `~/.pi/agent/prompts/nagare-*.md` — the four existing slash commands, pi format
- `~/.config/opencode/plugins/nagare.js` — status events → `hook-state`
- OpenCode MCP registration moves to `opencode.json`

Both generated files are rewritten on every `setup` run, so they upgrade in place.

### UI / lifecycle

`AgentPi` joins the model enum with a violet identity (`#a78bfa` on `#241b3b`,
gradient `#c4b5fd`→`#7c5cf0`) — distinct from Crush's pink and Gemini's blue. Detection
covers both install shapes: `pi` as `pane_current_command` for the binary, and `pi` in
the `/proc` descendant walk for the npm/node install. Launch command is `pi` / `pi -c`.
pi appears in the new-session form, the quick-prototype form, the `--agent` flag help,
and the picker's block-letter logo set.

## Out of scope

- pi permission gating (pi has no permission prompts; nothing to observe)
- Pushing messages into pi via its RPC mode — the tmux nudge already works
- Migrating stale nagare entries out of the legacy `~/.config/opencode/config.json`
  (harmless; current OpenCode ignores the file)
