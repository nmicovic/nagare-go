# Picker list grouping

Date: 2026-08-11

Spec and implementation plan in one document, since it is implemented in the same
session rather than handed to a fresh agent.

## Problem

Worktree awareness gave every pane its real name, and immediately ate the list pane.
At the observed width the name column is about 33 columns, so
`cosmic-platform-frontend/fluttering-watching-gadget` (51 characters) renders as
`cosmic-platform-frontend/flutter…` — truncated at precisely the part that identifies
it. Three sessions of one repo spend 25 columns each repeating the repo name.

Dropping the prefix alone is not a fix: a row reading `shipping` no longer says which
repo it belongs to. The prefix and the hierarchy have to be solved together.

## Decisions

**Group header rows with indented children.** A non-selectable header names the repo
once; children show only their own name, prefixed with a tree glyph.

**A group sorts as if it were its most urgent member.** The alternative — fixed
alphabetical group positions — buries a waiting session behind quiet repos, and
"is anything waiting on me?" is the picker's main job.

**The cursor keeps indexing sessions, not rows.** Every keybinding (jump, kill, star,
rename, approve, prompt) reads `m.filtered[m.cursor]`, and grid view shares
`m.filtered`. Moving the cursor into row space would touch all of it.

## Design

### Display rows

`buildRows` maps the sorted session list to render lines:

```go
type listRow struct {
	SessionIdx int    // index into the session slice; -1 for a header
	Label      string // worktree name for a child, full name otherwise
	Group      string // header text (headers only)
	Count      int    // members (headers only)
	Glyph      string // "├" / "└" for children, "" otherwise
}
```

Rows are derived per frame rather than stored, so they cannot drift out of sync with
`m.filtered`. Sessions are already in visual order when `buildRows` runs, so it only
inserts headers and derives labels. It relies on group members being contiguous, which
`sortFiltered` guarantees.

A group forms only at two or more members, so this also tidies plain multi-pane
sessions (`claude_01` / `claude_02`), not just worktrees. A lone session renders exactly
as today: no header, no glyph, full name.

Group key is the tmux session name. Child labels come from `Details.Worktree`, else the
part of the name after `{sessionName}/`, else the full name.

### Sorting

`sortFiltered` groups, sorts within each group by the active mode, picks each group's
most urgent member as its representative, orders groups by comparing representatives
with the same starred-then-active-mode comparator used today, then flattens.

"Most urgent" is starred first, then lowest `statusOrder`. It is deliberately
independent of the active sort mode: a waiting child must lift its group even when
sorting by name.

Because a singleton is a group of one and is its own representative, today's flat
behaviour is a special case of the new rule rather than a break from it.

### Rendering

Child rows become ` {dot} {glyph} {label}{gap}{star}{badge}` — two columns of indent
in exchange for dropping 25 of prefix. Headers carry no dot and no badge: the repo name
with a right-aligned `N sessions`. Truncation applies to the label only.

Scrolling moves to row space: find the row whose `SessionIdx` equals the cursor, and
window around it. Sticky headers are out of scope; a group's header can scroll off
above its children.

### Search

No special handling, by construction. Filtering already matches the full session name,
so typing `cosmic` keeps the children, and headers are derived from surviving children —
a group appears exactly when it still has members. Match highlighting moves to the
label, so a query matching only the repo portion highlights nothing on the child; the
header above carries that text.

### Out of scope

- Collapsible / foldable groups
- Grouping in grid view (stays flat)
- Sticky headers while scrolling

## Implementation plan

**Task 1 — `internal/picker/grouping.go` + test.** `listRow`, `groupKeyOf`,
`childLabel`, `buildRows`. Tests: ungrouped list, one group of three (header, glyphs,
labels), two groups, group of one, worktree label preferred over name suffix, custom
window name suffix, contiguity assumption. TDD: tests first.

**Task 2 — grouped sort.** Rewrite `sortFiltered` per above. Tests: waiting child lifts
its group above a quiet repo; starred child lifts its group; within-group order follows
the active mode; a singleton list sorts exactly as before (regression); group members
come out contiguous.

**Task 3 — render rows.** `renderListView` walks rows, renders headers and children,
windows in row space. Test: a 26-character worktree label does not truncate at the
observed pane width, where the old 51-character name did.

**Task 4 — verify live and document.** Run against the real three-worktree session,
update `CLAUDE.md` (picker section) and this document's status.
