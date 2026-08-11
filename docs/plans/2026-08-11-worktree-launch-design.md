# Launching agents in named worktrees

Date: 2026-08-11

Builds on `2026-08-11-worktree-awareness-design.md` (branch `worktree-awareness`),
which gave every pane its own path, branch, and worktree name, and grouped repos in
the list.

## Goal

Start an agent in a fresh, named git worktree from one command or one keystroke —
`nagare-go new <repo> -w my-worktree-name` — plus the surrounding worktree affordances
that make a multi-worktree workflow safe to live in.

## Findings

Claude Code already ships the flag:

```
-w, --worktree [name]   Create a new git worktree for this session (optionally specify a name)
--tmux                  Create a tmux session for the worktree (requires --worktree)
```

`claude -w the-site` produces `.claude/worktrees/the-site` on branch
`worktree-the-site` — confirmed against the live worktrees. None of opencode, gemini,
crush, or pi has an equivalent.

`--tmux` is deliberately never passed: nagare creates the pane itself, and letting
Claude create a second tmux session would fight it.

## Decisions

**Delegate to the agent where supported.** Claude gets `-w`, its blessed path. For the
other four, nagare runs `git worktree add` itself. Two directory layouts coexist
(`.claude/worktrees/` and `.worktrees/`), which is acceptable because worktree
detection is structural and does not care where a worktree lives.

**A worktree pane joins the repo's existing tmux session as a new window.** This is
what makes the picker group it with its siblings — a separate tmux session per worktree
would defeat the grouping entirely. If the repo has no session yet, one is created
first.

**A name is required.** Naming the worktree is the point of the feature, so nagare
requires `-w NAME` rather than mirroring Claude's optional form. `-w` together with
`-n` is an error: they would mean two different names.

## Design

### Phase 1 — create and launch

`internal/git` gains actions beside its existing facts:

```go
func ValidWorktreeName(name string) error       // reject empty, separators, leading "-", odd runes
func MainRoot(dir string) string                // the main checkout, from any worktree of a repo
func AddWorktree(repoRoot, name string) (string, error)
```

`MainRoot` is a small separate call rather than a new `Repo` field, so `Describe`'s
struct and its table tests stay as they are. It matters because F3 may be pressed while
a *worktree* pane is selected — the new worktree must still be created relative to the
main checkout.

`internal/session` gains:

```go
type worktreeLaunch struct {
    Cmd  string // command sent to the pane
    Cwd  string // directory the window opens in
    Path string // where the worktree will live
}
func planWorktreeLaunch(agent, mainRoot, name string) worktreeLaunch
func CreateWorktree(repoPath, worktreeName, agent string) (string, error)
```

Splitting the plan out from the tmux work keeps the branching testable without a tmux
server: Claude yields `claude -w <name>` with the window opening at the repo root,
every other agent yields its normal command with the window opening in the worktree.

`CreateWorktree` then validates the name, resolves the main root, refuses a non-repo,
creates the worktree (unless Claude will), finds or creates the repo's tmux session,
adds a window named after the worktree, sends the command, and registers the session
under its grouped display name.

CLI: `nagare-go new <repo> -w <name>`. Picker: **F3** on the selected session prompts
for a name and runs the same path. Every Ctrl-letter worth having is already bound
(`^n` new, `^r` proto, `^w` unload), and F-keys are free.

### Phase 2 — awareness

**Shared worktree warning.** Two agents in one working tree will fight over files.
Resolved paths are already known, so any path held by two or more panes earns a
warning-coloured `!` on the row and a `Shared  N agents` line in the detail pane.

**Dirty / ahead state.** The detail pane re-renders every frame for the status-dot
pulse, so git cannot be called from render. `git.WorkStatus(path)` runs for the selected
session only — on selection change and on the refresh tick — cached in the model by
path. The ahead count appears only when the branch has an upstream; without one, only
the uncommitted count shows, rather than inventing a base to diff against.

### Phase 3 — removal, and a pre-existing hazard

Ctrl+x kills the whole tmux session today. Now that worktrees are windows inside one
session, Ctrl+x on `the-site` also kills `shipping` and `fluttering-watching-gadget`.
Grouping did not cause this, but it makes it easy to trigger.

So Ctrl+x becomes: kill only the window when the session holds more than one agent
pane; behave exactly as today on a single-pane session. For a worktree pane it then
offers to remove the worktree — refusing when there is uncommitted work, requiring
explicit confirmation, and leaving the branch alone so commits survive.

## Testing

- `ValidWorktreeName` table: empty, `a/b`, `..`, `-x`, valid names.
- `AddWorktree` against a real repo in `t.TempDir()`, asserting the worktree exists,
  is detected by `Describe`, and that a duplicate name fails.
- `MainRoot` from both a main checkout and a linked worktree.
- `planWorktreeLaunch` per agent: Claude's `-w` form and root cwd, others' worktree cwd.
- `WorkStatus`: clean, dirty, and no-upstream cases against a real repo.
- Shared-path detection from a session list.
- Kill-target selection: multi-pane session yields a window target, single-pane yields
  the session.

The tmux wiring in `CreateWorktree` is verified live rather than unit-tested, matching
how the rest of `internal/session` is covered.

## Out of scope

- Passing `--tmux` to Claude
- Deleting the branch when removing a worktree
- Converting an existing plain session into a worktree
- Listing or pruning worktrees that have no agent pane

## Landing

Two PRs: Phase 1 alone, then Phases 2 and 3. Phase 1 is independently useful, and
Phase 3 changes what a destructive key does, so it deserves separate review.

Branched from `worktree-awareness`, which is not yet merged; rebases onto main once it
lands.
