# Worktree awareness

Date: 2026-08-11

## Problem

Worktrees are central to how these agents are used — Claude Code creates them under
`.claude/worktrees/<name>` — but nagare treats every pane in a tmux session as if it
sat in the session's directory. Two Claude panes in two worktrees of
`cosmic-platform-frontend` currently report:

```
cosmic-platform-frontend/claude_01 (Claude) [Working] /home/nemke/Projects/cosmic-platform-frontend
cosmic-platform-frontend/claude_02 (Claude) [Idle]    /home/nemke/Projects/cosmic-platform-frontend
```

Three things are wrong:

- **Names carry no information.** `claude_01` and `claude_02` give no way to tell which
  worktree is which, in the picker or as a messaging target.
- **Paths are wrong.** Both show the main checkout. The real paths are
  `.claude/worktrees/fluttering-watching-gadget` and `.claude/worktrees/the-site`.
- **Branches are wrong.** Both render the main checkout's `dev`, not
  `worktree-fluttering-watching-gadget` and `worktree-the-site`.

The root cause is single: `ScanSessions` uses the tmux *session* path for every pane in
the session and never reads `pane_current_path`.

Claude Code's hooks already report the correct worktree path as `cwd`, and nagare
already stores it in the state file — it is simply never used for display.

## Decisions

**Per-pane path comes from tmux `pane_current_path`.** It is available for every pane
without extra processes, works for agents with no status reporting at all (Crush) and
before any hook has fired. Hook-state `cwd` was considered and rejected as the display
source: it does not exist until a hook fires, never exists for Crush, and stale state
files outlive their panes (pane ids are recycled — `%2` appears in old state files
pointing at four different repositories), so using it for display would resurrect dead
paths.

**Worktree identity is the worktree directory basename**, giving
`cosmic-platform-frontend/the-site`. The git branch was considered and rejected: Claude's
generated branches are verbose and the repeated `worktree-` prefix adds no information.
The branch remains visible in the picker, just no longer inside the name.

**Worktrees are detected structurally, not by path matching.** `dir(git-common-dir) !=
toplevel` identifies a linked worktree, so hand-made worktrees in any location behave
exactly like Claude's.

## Design

### 1. Scanner learns per-pane paths

`PaneInfo` gains a `Path` field. `ParseAllPanes` parses an 8th `pane_current_path` field
and still accepts 5-, 6-, and 7-field input, matching the backward compatibility it
already provides. `ScanSessions` resolves each pane's path as `pane.Path`, falling back
to the session path when tmux reports nothing.

The hook-state cwd fallback lookup changes with it: `cwdStates` is keyed off the
resolved pane path rather than the session path. Today `cwdStates[sess.Path]` cannot
match a worktree pane at all, because the agent reports the worktree directory.

### 2. New `internal/git` package

```go
type Repo struct {
    Branch     string // "" when detached or not a repo
    RepoName   string // main checkout basename — identical across a repo's worktrees
    Worktree   string // worktree dir basename; "" for the main checkout
    IsWorktree bool
}

func Describe(dir string) Repo
```

One `git -C <dir> rev-parse --git-common-dir --show-toplevel --abbrev-ref HEAD` per
unique path. Relative common dirs (git prints a bare `.git` for a main checkout) are
resolved against `dir`, so no minimum git version is required. A detached HEAD makes
`--abbrev-ref` print the literal `HEAD`, which is normalized to an empty `Branch` —
matching what `git branch --show-current` returns today.

Memoization is a map owned by a single `ScanSessions` call, not package state, so two
panes sharing a worktree cost one subprocess while a branch switch still shows up on the
next refresh.

This replaces the existing per-session `git branch --show-current`, so the number of
subprocesses per scan stays flat even though resolution moves from per-session to
per-pane.

`RepoName` is the basename of the common dir's parent, which is the main checkout for
both a main checkout and a linked worktree.

### 3. Naming

`ComputeDisplayNames` precedence becomes:

1. explicit (user-set) window name — unchanged, user intent wins
2. worktree basename → `{session}/{worktree}`
3. `{session}/{agent}_NN` — unchanged fallback

A single-pane session sitting in a worktree also takes the suffix, which means the
existing "one pane → bare session name" short-circuit at the top of
`ComputeDisplayNames` has to consult the worktree first rather than returning early.
Existing prefix matching in `resolveSession` means
`send_message("cosmic-platform-frontend", …)` keeps resolving while it is unambiguous.
Two panes in the same worktree fall back to the existing `_NN` counter.

### 4. Picker

The name change does most of the work. Beyond it: `SessionDetails` gains `Worktree` and
`RepoName`, the detail pane gains a "Worktree" row shown only for linked worktrees, and
the per-row branch becomes the pane's real branch. No layout changes.

### 5. Error handling

Non-git directories, a missing `git` binary, and detached HEAD each yield a zero `Repo`
and render exactly as today. `Describe` never fails the scan.

### 6. Testing

- Table tests for the `rev-parse` output parser: main checkout, linked worktree,
  relative common dir, detached HEAD, non-repo, empty output.
- `ComputeDisplayNames` with worktree panes: naming, same-worktree collisions, and
  explicit window names still winning.
- `ParseAllPanes` with 8 fields, plus the existing 5/6/7-field compatibility.
- One integration test building a real repository with a real worktree in `t.TempDir()`,
  asserting `Describe` on both the main checkout and the worktree.

## Out of scope

- Creating or removing worktrees from nagare
- Grouping or nesting worktrees visually under a parent repo row in the picker
- Backfilling worktree paths into existing state files
