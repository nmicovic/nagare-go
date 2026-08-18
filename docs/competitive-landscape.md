# Competitive Landscape — agent session managers

Research snapshot: **2026-08-18**. Star counts and activity are as of that date and will rot;
the UX conclusions should outlive them.

Scope: tools whose job is running, watching, or orchestrating **multiple AI coding agents at
once**. Plain multiplexers and tmux utilities are included only where they set expectations
nagare is measured against.

## The one-sentence map

The niche split into two camps in the first half of 2026:

- **Camp 1 — replace the terminal or the multiplexer.** Own the PTYs, ship your own
  multiplexer or your own terminal emulator. herdr, cmux, Warp.
- **Camp 2 — sit on top of tmux.** Thin, keyboard-driven tools that read tmux state and jump
  you around it. nagare, tmux.expose, tmuxwatch, workmux, sesh.

The sharpest framing found in the research, from a
[herdr-vs-cmux comparison](https://blog.debedb.com/2026/07/26/herdr-and-cmux-two-shapes-of-the-same-agent-multiplexer/):

> herdr optimizes for **agent-to-agent orchestration**; cmux optimizes for
> **human-in-the-loop**.

That is the axis worth choosing a side on, and it matters more than the camp split.

## Camp 1 — the heavyweights

### herdr

Rust, ratatui 0.30, crossterm 0.29, tokio. ~14k stars since June 2026, front-paged HN.
Single ~10 MB binary, macOS/Linux/Windows. [Repo](https://github.com/ogulcancelik/herdr) ·
[compare page](https://herdr.dev/compare/) ·
[HN thread](https://news.ycombinator.com/item?id=48714802)

Architecture: **headless server owns the PTYs; every UI is a client** — TUI, CLI, and SSH all
attach to the same runtime. The TUI can crash without touching a running agent. Explicitly
positioned as "a runtime with multiple client interfaces, not an application."

Feature depth, from the changelog:

- **Agent awareness** — blocked / working / done / idle, detected from process names plus
  terminal-output heuristics, with hook-reported states layered on top. Agents detected:
  Claude Code, Codex, Amp, Devin, Kimi, Droid, Kilo, Cursor Agent, Copilot CLI, Qoder,
  MastraCode, Grok Build, Pi. Detection rules ship as a **remotely auto-updated manifest**
  with local override precedence.
- **Session restore** — integrations report session IDs so agents can resume conversations
  after a *server* restart.
- **Custom status labels** — hooks can surface short visual states like `indexing` without
  disturbing the semantic status.
- **UI** — sidebar + terminal pane. Collapsible sections, configurable row layouts with
  per-token styling and per-agent overrides, git-worktree groups in the sidebar, resizable
  dividers, drag-to-reorder tabs, scrollable tab overflow, zoom indicators.
- **Session navigator** (`prefix+g`) — searchable workspace/tab/pane tree with agent-state
  filtering and auto-selection of the first result.
- **Copy mode** (`prefix+[`) — visual selection, clipboard yank, smart-case `/` and `?`
  search with `n`/`N` and match highlighting, tmux-style cross-line word motions.
- **Mobile layout** — narrow-terminal single-column mode, explicitly so SSH-from-a-phone is
  practical.
- **Popup panes** — session-modal floating terminals for custom command keybindings.
- **Notifications** — toasts, optionally bridged to the *outer* terminal as OS desktop
  notifications (`notify-send` / `terminal-notifier` / `osascript`).
- **Worktrees** — native create / open-existing / safe-cleanup, CLI and socket API
  (`herdr worktree list|create|open|remove`), atomic worktree-group reordering.
- **Socket API** — `pane.move`, `pane.zoom`, `layout.export`/`apply`, `notification.show`,
  `agent.view.set`/`clear`, per-client navigation state, read-only live ANSI stream via
  `herdr terminal session observe`.
- **Named agent commands** — atomic `prompt`, logical `send-keys`, and a server-owned
  **`wait`** verb for blocking on agent state.
- **Plugins** — v1 local plugins with manifest-declared actions, event hooks, managed panes,
  link handlers, keybinding integration; marketplace planned.
- **Keybindings v2** — explicit `prefix+…` syntax, array bindings per action, `[keys.indexed]`
  families for 1–9 jumps, direct modified chords, navigate mode, `last_pane`.
- **Remote** — `herdr --remote <ssh-target>` streams an efficient terminal view with
  automatic bootstrap.

**What HN praised:** mouse support and native-feeling scroll ("a killer feature over tmux"),
SSH/Tailscale access from a phone via Termius, the API's extensibility, and simply having one
place instead of a dozen terminal windows.

**What HN criticized** — this list is the more useful half:

- Keybinding conflicts "all over the place."
- Complexity relative to what it replaces; several commenters could not articulate what it
  gave them over tmux.
- **"Running outside tmux creates workflow friction for existing terminal users."**
- Unclear visibility into what an individual agent is actually *doing* — state labels are not
  the same as insight.
- Mobile touch via remote clients is poor on Android.
- Skeptics questioned whether many-parallel-agents is desirable at all; some prefer 1–2
  sequential sessions with hands-on review.

**Requested:** fixed/declarative layout config (tmuxp-style), better mobile, clearer progress
indicators, per-agent SSH sessions.

The comparison page lists no weaknesses of its own — worth remembering when reading their
positioning.

### cmux

Native macOS, Swift/AppKit on libghostty, GPU-rendered. AGPL-3.0, ~5k stars since January
2026. macOS only — no Linux, no Windows. [Site](https://cmux.com) ·
[notifications docs](https://cmux.com/docs/notifications)

It *is* the terminal rather than a program inside one, which sidesteps multiplexer key
conflicts entirely.

- Vertical sidebar tabs per agent with state; **a blue ring when an agent is waiting for
  input**.
- `Cmd+Shift+U` — **jump to the most recent unread pane**.
- `Cmd+Shift+I` — **approval feed / notification panel spanning every workspace**, with
  pending requests from all of them in one list.
- Approval banners flow through notification hooks; disabling desktop delivery keeps the feed
  item.
- Worktree-per-agent dispatch: run Claude Code, Codex and Gemini on the same task side by
  side, then **review the diffs, merge the wins, discard the rest**.
- Non-PTY surfaces: built-in scriptable browser, markdown viewer, file previews.
- Native session persistence across 14+ agents via a hook system.

## Camp 2 — the tmux layer

- **[tmux.expose](https://github.com/cesarferreira/tmux.expose)** — Rust TUI, Mission
  Control for tmux. Every session is a **live text thumbnail with tmux ANSI colors
  preserved**, in a responsive balanced grid, refreshed on a 500 ms loop. Fuzzy filter by
  typing, arrows to navigate, Enter to switch, click-to-switch, optional `--vim` mode.
  `--thumbnail-width` / `--columns` to override layout. Runs standalone or in a tmux popup;
  TPM plugin binds `Alt+e`. **The closest thing to what nagare's grid view could be.**
- **[tmuxwatch](https://github.com/steipete/tmuxwatch)** — Charm-based dashboard; polls
  `list-sessions`/`list-windows`/`list-panes`, stitches the hierarchy, shows the latest
  `capture-pane` output per session. Same technique nagare already has the scan loop for.
- **[workmux](https://github.com/raine/workmux)** — git worktrees + tmux windows for parallel
  dev, plus a TUI dashboard of all active agents across all tmux sessions.
- **[sesh](https://github.com/joshmedeski/sesh)** (~2.8k★) — session manager whose good idea
  is **frecency-ranked recent directories** as a session source, so "new session" works before
  any pane exists.
- **[mprocs](https://github.com/pvolok/mprocs)** (~2.7k★) — grid of live panes, conceptually
  nagare's grid view but for arbitrary processes rather than agents.

## Direct neighbours — agent-specific managers

| Tool | Stars / activity | The one thing worth stealing |
|---|---|---|
| [Claude Squad](https://github.com/smtg-ai/claude-squad) | 8.3k, active | Checkout/commit keys acting on the session's worktree branch **from the picker** |
| ccmux | 137, very active | **Three-channel status detection** (JSONL + output pattern + hook PID) instead of pane scraping; pipe a diff to an external reviewer and route line comments **back into the agent**; session handoff between worktrees with provenance in the header |
| Bosun | 40, very active (Rust/ratatui) | **Embedded live PTY preview, not snapshots**; explicit merge/discard/keep at worktree-kill; edit-and-restart a session's stored spec to recover a forgotten `--resume` |
| Agent Deck | 739, very active | **Fork-with-context-inheritance** — branch a running session's whole transcript into a parallel exploration; status bar of clickable "waiting" chips |
| Uzi | 581, stale | **Fan N agents at one prompt** across N worktrees for later comparison |
| NTM | 428, very active | Durable **audit/checkpoint trail** of every agent action, not just live state |
| Crystal → Nimbalyst | 3.1k, successor closed-source | **Run the test script before merge** as a baked-in worktree lifecycle gate |
| Conductor | closed-source Mac app | Treats **PR review comments as an input channel** back into the same session |
| Vibe Kanban | 27.8k, inactive | **todo → running → in-review → done** as a task abstraction above sessions |
| Emdash / Superset / Solo / Pane / AgentsRoom / agentastic.dev | — | Worktree + review-queue manager apps; Solo is a process dashboard for dev stacks |

The pattern across every actively-developed one: **they have all moved past "a picker" into the
review loop.** Vibe Kanban's `in-review` is the state our four-state model
(idle/running/waiting/dead) does not have.

## Workflow insights from the wild

- **The bottleneck is latency, not capability.** A 5+ minute wait is what justifies
  context-switching to another project — that, not agent count, is the thing being managed.
- People run **5–8 concurrent agents**; skeptics deliberately run 1–2 and review by hand.
  Both are legitimate and the tool should not assume the first.
- **Worktrees are the standard race-avoidance mechanism.** Everyone converged on this.
- Phone-over-SSH babysitting is real enough that herdr shipped a dedicated narrow layout for
  it.

## Where nagare actually stands

Moats worth defending:

1. **It does not replace tmux.** No prefix conflicts, works inside an existing config, remote
   by default because tmux already is. herdr's most-cited friction is our free win.
2. **Inter-agent messaging already exists** — MCP tools, `send_message_and_wait`, a mailbox
   UI, and the `nagare-go tool` bridge for pi. herdr is building toward `agent wait --until
   done`; we shipped the human-legible version. **Nobody else has a mailbox UI.**
3. **Structural worktree detection** — the git-common-dir test means hand-made worktrees work
   identically to Claude's. workmux and herdr both special-case their own layout.
4. **Breadth of setup automation** — hooks + MCP + slash commands installed for five agents in
   one command is more complete than anyone else's.
5. **Single Go binary**, 13 themes, cross-platform where cmux is macOS-only.

Gaps, ranked by leverage:

1. **Live pane previews.** `capture-pane -ep` on the scan tick we already run. Turns grid view
   into a Mission Control wall. tmux.expose proves 500 ms is enough and that preserved ANSI is
   what makes it feel alive. Bosun proves live PTY beats snapshots if we want to go further.
2. **"Jump to next thing that needs me."** One key, urgency-ordered. We already compute
   urgency for group ordering and never expose it as navigation. cmux's `Cmd+Shift+U`.
3. **Diff / work review.** We show `git.WorkStatus` counts; the next step is files changed,
   `diff --stat`, and a scrollable diff. This is where "is this agent's work any good" gets
   answered, and it is what every active competitor built.
4. **Narrow/mobile breakpoint.** A `<60`-column single-column layout is roughly a day and a
   headline feature.
5. **Approval feed as a first-class view** — a chronological stream of permission requests,
   answerable inline with the Ctrl+y / Ctrl+a we already have.
6. **Keybinding discipline.** 18 bindings, `Ctrl+`-heavy, F1–F3 mixed in. This is precisely
   what herdr got criticized for. Restructure into layers before adding more.

## Strategic read

Do not chase herdr into being a multiplexer. Nagare's position is **the beautiful
human-in-the-loop cockpit for agents you are already running in tmux**, with the
agent-to-agent messaging substrate nobody else has as the second act.

"Prettiest in the niche" is genuinely unclaimed: herdr is ratatui-functional, cmux is
native-Mac-slick and macOS-only, and nothing in the tmux-wrapper camp has attempted beauty at
all.

## Sources

- [herdr repo](https://github.com/ogulcancelik/herdr) ·
  [changelog](https://github.com/ogulcancelik/herdr/blob/master/CHANGELOG.md) ·
  [compare](https://herdr.dev/compare/) ·
  [HN](https://news.ycombinator.com/item?id=48714802)
- [herdr vs cmux](https://blog.debedb.com/2026/07/26/herdr-and-cmux-two-shapes-of-the-same-agent-multiplexer/)
- [cmux](https://cmux.com/) · [cmux notifications](https://cmux.com/docs/notifications)
- [tmux.expose](https://github.com/cesarferreira/tmux.expose) ·
  [tmuxwatch](https://github.com/steipete/tmuxwatch) ·
  [workmux](https://github.com/raine/workmux)
- [The Terminal Renaissance](https://hyperbliss.tech/blog/2026.04.04_terminal-renaissance/) —
  TUI design principles
- [awesome-tuis](https://github.com/rothgar/awesome-tuis) ·
  [awesome-ratatui](https://github.com/ratatui/awesome-ratatui)
