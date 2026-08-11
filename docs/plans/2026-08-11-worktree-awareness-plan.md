# Worktree Awareness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make nagare show each agent pane's real worktree — its own path, branch, and a name derived from the worktree directory — instead of collapsing every pane onto the tmux session's directory.

**Architecture:** A new `internal/git` package resolves a directory into a `Repo` value (branch, repo name, worktree name) with one `git rev-parse` call. The tmux scanner starts reading `pane_current_path` so each pane carries its own directory, feeds that directory to `internal/git`, and uses the resulting worktree name when building display names.

**Tech Stack:** Go, tmux format strings, `git rev-parse`, standard `testing` package.

## Global Constraints

- Spec: `docs/plans/2026-08-11-worktree-awareness-design.md`
- Follow Effective Go; `gofmt` everything; no underscores in names (MixedCaps / mixedCaps)
- Tests colocated: `foo_test.go` next to `foo.go`
- Always check errors
- `Describe` must never fail a scan — non-repo, missing git, and detached HEAD all return a usable zero value
- Worktree detection must be structural (`dir(git-common-dir) != toplevel`), never a `.claude/worktrees` path match
- Subprocess count per scan must not grow: the new per-path `rev-parse` replaces the existing per-session `git branch --show-current`
- Verify with: `gofmt -l .`, `go vet ./...`, `go test ./...`

---

### Task 1: `internal/git` package — parse `rev-parse` output

**Files:**
- Create: `internal/git/git.go`
- Test: `internal/git/git_test.go`

**Interfaces:**
- Consumes: nothing
- Produces: `git.Repo{Branch, RepoName, Worktree string; IsWorktree bool}` and `git.parseRevParse(out, dir string) Repo`

`git rev-parse --git-common-dir --show-toplevel --abbrev-ref HEAD` prints exactly three
lines. Verified real output:

```
main checkout:     ".git\n/home/u/Projects/app\ndev\n"
linked worktree:   "/home/u/Projects/app/.git\n/home/u/Projects/app/.claude/worktrees/the-site\nworktree-the-site\n"
non-repo:          exit 128, nothing on stdout
```

- [ ] **Step 1: Write the failing test**

```go
package git

import "testing"

func TestParseRevParse(t *testing.T) {
	tests := []struct {
		name string
		out  string
		dir  string
		want Repo
	}{
		{
			name: "main checkout has a relative common dir",
			out:  ".git\n/home/u/Projects/app\ndev\n",
			dir:  "/home/u/Projects/app",
			want: Repo{Branch: "dev", RepoName: "app"},
		},
		{
			name: "linked worktree",
			out:  "/home/u/Projects/app/.git\n/home/u/Projects/app/.claude/worktrees/the-site\nworktree-the-site\n",
			dir:  "/home/u/Projects/app/.claude/worktrees/the-site",
			want: Repo{Branch: "worktree-the-site", RepoName: "app", Worktree: "the-site", IsWorktree: true},
		},
		{
			name: "detached HEAD reports no branch",
			out:  ".git\n/home/u/Projects/app\nHEAD\n",
			dir:  "/home/u/Projects/app",
			want: Repo{RepoName: "app"},
		},
		{
			name: "empty output yields the zero value",
			out:  "",
			dir:  "/tmp",
			want: Repo{},
		},
		{
			name: "truncated output yields the zero value",
			out:  ".git\n",
			dir:  "/tmp",
			want: Repo{},
		},
	}
	for _, tt := range tests {
		got := parseRevParse(tt.out, tt.dir)
		if got != tt.want {
			t.Errorf("%s: parseRevParse() = %+v, want %+v", tt.name, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/git/ -run TestParseRevParse -v`
Expected: FAIL — build error, `undefined: Repo` and `undefined: parseRevParse`

- [ ] **Step 3: Write minimal implementation**

```go
// Package git resolves a directory into the git repository facts nagare
// displays: the current branch, the repository name, and — when the directory
// is a linked worktree — the worktree name.
package git

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo describes the git checkout a directory belongs to. The zero value is
// what callers get for a directory that is not a repository, and it renders as
// "no git information" rather than as an error.
type Repo struct {
	Branch     string // empty when detached or not a repository
	RepoName   string // basename of the main checkout, identical across its worktrees
	Worktree   string // basename of the worktree directory, empty for the main checkout
	IsWorktree bool
}

// parseRevParse turns the three lines of `git rev-parse --git-common-dir
// --show-toplevel --abbrev-ref HEAD` into a Repo. dir is the directory the
// command ran in, needed because git prints a bare ".git" for a main checkout.
func parseRevParse(out, dir string) Repo {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 {
		return Repo{}
	}

	commonDir, toplevel, branch := lines[0], lines[1], lines[2]
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(dir, commonDir)
	}

	// A main checkout's common dir sits directly inside its own toplevel; a
	// linked worktree's points back at the main checkout instead.
	mainRoot := filepath.Dir(filepath.Clean(commonDir))
	repo := Repo{
		RepoName:   filepath.Base(mainRoot),
		IsWorktree: mainRoot != filepath.Clean(toplevel),
	}
	if repo.IsWorktree {
		repo.Worktree = filepath.Base(toplevel)
	}
	// Detached HEAD prints the literal "HEAD"; report no branch, matching what
	// `git branch --show-current` returns.
	if branch != "HEAD" {
		repo.Branch = branch
	}
	return repo
}

// Describe resolves dir into a Repo using a single git invocation. Any failure
// (not a repository, git missing) returns the zero Repo.
func Describe(dir string) Repo {
	cmd := exec.Command("git", "-C", dir, "rev-parse",
		"--git-common-dir", "--show-toplevel", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return Repo{}
	}
	return parseRevParse(string(out), dir)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/git/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go
git commit -m "git: resolve a directory into branch, repo, and worktree facts"
```

---

### Task 2: `Describe` against a real repository and a real worktree

**Files:**
- Modify: `internal/git/git_test.go`

**Interfaces:**
- Consumes: `git.Describe(dir string) Repo` from Task 1
- Produces: nothing

This is the test that would have caught a wrong assumption about git's output
shape. It builds an actual repository and an actual worktree.

- [ ] **Step 1: Write the failing test**

```go
func TestDescribeRealRepoAndWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	repoDir := filepath.Join(root, "app")

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		// Keep the sandbox free of the developer's real git identity/hooks.
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	run(repoDir, "init", "-q", "-b", "dev")
	if err := os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	run(repoDir, "add", ".")
	run(repoDir, "commit", "-qm", "init")

	wtDir := filepath.Join(repoDir, ".claude", "worktrees", "the-site")
	run(repoDir, "worktree", "add", "-q", "-b", "worktree-the-site", wtDir)

	main := Describe(repoDir)
	if main.IsWorktree {
		t.Errorf("main checkout reported as a worktree: %+v", main)
	}
	if main.RepoName != "app" || main.Branch != "dev" || main.Worktree != "" {
		t.Errorf("main checkout = %+v", main)
	}

	wt := Describe(wtDir)
	if !wt.IsWorktree {
		t.Errorf("linked worktree not detected: %+v", wt)
	}
	if wt.RepoName != "app" || wt.Worktree != "the-site" || wt.Branch != "worktree-the-site" {
		t.Errorf("worktree = %+v", wt)
	}

	// A detached worktree HEAD must report no branch, not "HEAD".
	run(wtDir, "checkout", "-q", "--detach")
	if got := Describe(wtDir).Branch; got != "" {
		t.Errorf("detached HEAD branch = %q, want empty", got)
	}

	// A directory outside any repository must not panic or invent facts.
	if got := Describe(root); got != (Repo{}) {
		t.Errorf("non-repo = %+v, want zero value", got)
	}
}
```

- [ ] **Step 2: Add the imports the test needs**

Add `"os"`, `"os/exec"`, and `"path/filepath"` to the import block of
`internal/git/git_test.go`.

- [ ] **Step 3: Run the test**

Run: `go test ./internal/git/ -run TestDescribeRealRepoAndWorktree -v`
Expected: PASS (Task 1's implementation already satisfies it; this test exists to
prove the real-git assumptions, so a failure here means Task 1 is wrong)

- [ ] **Step 4: Commit**

```bash
git add internal/git/git_test.go
git commit -m "git: cover Describe against a real repo and linked worktree"
```

---

### Task 3: Scanner reads `pane_current_path`

**Files:**
- Modify: `internal/tmux/scanner.go` (`PaneInfo` struct, `ParseAllPanes`, the `list-panes` format string in `ScanSessions`)
- Test: `internal/tmux/scanner_test.go`

**Interfaces:**
- Consumes: nothing
- Produces: `PaneInfo.Path string` — the pane's own working directory, empty when tmux reports none

- [ ] **Step 1: Write the failing test**

```go
func TestParseAllPanesCapturesPanePath(t *testing.T) {
	raw := "proj:0:0:claude:111:? claude:%2:/home/u/proj/.claude/worktrees/the-site\n"
	panes := ParseAllPanes(raw)

	p, ok := panes["proj"]
	if !ok || len(p) != 1 {
		t.Fatalf("expected 1 pane, got %v", panes)
	}
	if p[0].Path != "/home/u/proj/.claude/worktrees/the-site" {
		t.Errorf("Path = %q, want the worktree path", p[0].Path)
	}
}

// Older field counts must keep parsing, as they already do for pane_id.
func TestParseAllPanesWithoutPanePath(t *testing.T) {
	raw := "proj:0:0:claude:111:? claude:%2\n"
	panes := ParseAllPanes(raw)
	if len(panes["proj"]) != 1 {
		t.Fatalf("expected 1 pane, got %v", panes)
	}
	if got := panes["proj"][0].Path; got != "" {
		t.Errorf("Path = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tmux/ -run TestParseAllPanesCapturesPanePath -v`
Expected: FAIL — `p[0].Path undefined (type PaneInfo has no field or method Path)`

- [ ] **Step 3: Add the field and parse the eighth column**

In `PaneInfo`, after `PaneID string`, add:

```go
	Path        string // pane_current_path — the pane's own cwd, which may be a worktree
```

In `ParseAllPanes`, change the split limit from 7 to 8 and parse the new field
after the existing `paneID` block:

```go
		parts := strings.SplitN(line, ":", 8)
```

```go
		panePath := ""
		if len(parts) >= 8 {
			panePath = strings.TrimSpace(parts[7])
		}
```

Add `Path: panePath` to the `PaneInfo` literal appended to `result[sessionName]`.

Update the doc comment above `ParseAllPanes` to list the new field and to say it
accepts 5- through 8-field input.

- [ ] **Step 4: Request the field from tmux**

In `ScanSessions`, extend the `list-panes` format string with `:#{pane_current_path}`:

```go
	rawPanes := RunTmux("list-panes", "-a", "-F", "#{session_name}:#{window_index}:#{pane_index}:#{pane_current_command}:#{pane_pid}:#{window_name}:#{pane_id}:#{pane_current_path}")
```

- [ ] **Step 5: Run the tmux tests**

Run: `go test ./internal/tmux/ -v`
Expected: PASS, including the pre-existing `TestParseAllPanes*` tests

- [ ] **Step 6: Commit**

```bash
git add internal/tmux/scanner.go internal/tmux/scanner_test.go
git commit -m "tmux: carry each pane's own working directory"
```

---

### Task 4: Worktree-aware display names

**Files:**
- Modify: `internal/tmux/scanner.go` (`ComputeDisplayNames`)
- Modify: `internal/tmux/scanner_test.go` (existing callers gain an argument)

**Interfaces:**
- Consumes: `PaneInfo.Path` from Task 3
- Produces: `ComputeDisplayNames(sessName string, panes []PaneInfo, worktreeOf map[string]string) map[string]string` — `worktreeOf` maps pane ID to worktree basename; a nil map means no pane is in a worktree

- [ ] **Step 1: Write the failing test**

```go
func TestComputeDisplayNamesWorktrees(t *testing.T) {
	panes := []PaneInfo{
		{WindowIndex: 0, PaneIndex: 0, AgentType: models.AgentClaude, WindowName: "? claude", PaneID: "%2"},
		{WindowIndex: 1, PaneIndex: 0, AgentType: models.AgentClaude, WindowName: "terminal", PaneID: "%12"},
	}
	worktreeOf := map[string]string{"%2": "fluttering-watching-gadget", "%12": "the-site"}

	got := ComputeDisplayNames("cosmic-platform-frontend", panes, worktreeOf)

	if got["%2"] != "cosmic-platform-frontend/fluttering-watching-gadget" {
		t.Errorf("%%2 = %q", got["%2"])
	}
	if got["%12"] != "cosmic-platform-frontend/the-site" {
		t.Errorf("%%12 = %q", got["%12"])
	}
}

// A lone pane in a worktree is still worth naming after the worktree.
func TestComputeDisplayNamesSinglePaneInWorktree(t *testing.T) {
	panes := []PaneInfo{{AgentType: models.AgentClaude, PaneID: "%5"}}
	got := ComputeDisplayNames("app", panes, map[string]string{"%5": "the-site"})
	if got["%5"] != "app/the-site" {
		t.Errorf("got %q, want %q", got["%5"], "app/the-site")
	}
}

// An explicitly named window is the user's own labelling and still wins.
func TestComputeDisplayNamesWindowNameBeatsWorktree(t *testing.T) {
	panes := []PaneInfo{
		{WindowIndex: 0, AgentType: models.AgentClaude, WindowName: "review", PaneID: "%1"},
		{WindowIndex: 1, AgentType: models.AgentClaude, WindowName: "terminal", PaneID: "%2"},
	}
	got := ComputeDisplayNames("app", panes, map[string]string{"%1": "wt-a", "%2": "wt-b"})
	if got["%1"] != "app/review" {
		t.Errorf("%%1 = %q, want app/review", got["%1"])
	}
	if got["%2"] != "app/wt-b" {
		t.Errorf("%%2 = %q, want app/wt-b", got["%2"])
	}
}

// Two panes in one worktree must stay distinguishable.
func TestComputeDisplayNamesSameWorktreeTwice(t *testing.T) {
	panes := []PaneInfo{
		{WindowIndex: 0, AgentType: models.AgentClaude, WindowName: "terminal", PaneID: "%1"},
		{WindowIndex: 1, AgentType: models.AgentClaude, WindowName: "terminal", PaneID: "%2"},
	}
	got := ComputeDisplayNames("app", panes, map[string]string{"%1": "the-site", "%2": "the-site"})
	if got["%1"] == got["%2"] {
		t.Errorf("both panes got the same name %q", got["%1"])
	}
}
```

- [ ] **Step 2: Update the existing three callers in the test file**

`TestComputeDisplayNames`, `TestComputeDisplayNamesSinglePane`,
`TestComputeDisplayNamesCustomWindowName`, and
`TestComputeDisplayNamesMixedAgents` each call `ComputeDisplayNames` with two
arguments. Add `, nil` to each call so they assert the no-worktree behaviour.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/tmux/ -run TestComputeDisplayNames -v`
Expected: FAIL — `too many arguments in call to ComputeDisplayNames`

- [ ] **Step 4: Implement the new precedence**

Replace `ComputeDisplayNames` with:

```go
// ComputeDisplayNames returns a map from pane_id to display name for a set of
// agent panes sharing a tmux session. worktreeOf maps pane_id to a worktree
// basename for panes sitting in a linked worktree, and may be nil.
//
// Name precedence, highest first: a window name the user set, the worktree the
// pane sits in, then "{agent}_NN" (1-based, per agent type, ordered by
// window/pane). A single pane outside a worktree keeps the bare session name.
func ComputeDisplayNames(sessName string, panes []PaneInfo, worktreeOf map[string]string) map[string]string {
	result := make(map[string]string, len(panes))
	if len(panes) == 1 && worktreeOf[panes[0].PaneID] == "" {
		result[panes[0].PaneID] = sessName
		return result
	}

	sorted := make([]PaneInfo, len(panes))
	copy(sorted, panes)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].WindowIndex != sorted[j].WindowIndex {
			return sorted[i].WindowIndex < sorted[j].WindowIndex
		}
		return sorted[i].PaneIndex < sorted[j].PaneIndex
	})

	// Reserve names that are unique on their own so the numbered fallback only
	// applies where it is actually needed.
	taken := make(map[string]int)
	for _, p := range sorted {
		if isCustomWindowName(p.WindowName, sessName) {
			taken[p.WindowName]++
		} else if wt := worktreeOf[p.PaneID]; wt != "" {
			taken[wt]++
		}
	}

	counts := make(map[models.AgentType]int)
	for _, p := range sorted {
		label := ""
		if isCustomWindowName(p.WindowName, sessName) {
			label = p.WindowName
		} else if wt := worktreeOf[p.PaneID]; wt != "" {
			label = wt
		}

		if label == "" || taken[label] > 1 {
			counts[p.AgentType]++
			suffix := fmt.Sprintf("%s_%02d", p.AgentType, counts[p.AgentType])
			if label != "" {
				suffix = fmt.Sprintf("%s_%02d", label, counts[p.AgentType])
			}
			result[p.PaneID] = sessName + "/" + suffix
			continue
		}
		result[p.PaneID] = sessName + "/" + label
	}
	return result
}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/tmux/ -v`
Expected: PASS, all `TestComputeDisplayNames*` including the four new ones

- [ ] **Step 6: Commit**

```bash
git add internal/tmux/scanner.go internal/tmux/scanner_test.go
git commit -m "tmux: name panes after the worktree they sit in"
```

---

### Task 5: Wire per-pane paths and worktree facts through `ScanSessions`

**Files:**
- Modify: `internal/models/models.go` (`SessionDetails`)
- Modify: `internal/tmux/scanner.go` (`ScanSessions`, delete `gitBranch`)
- Test: `internal/tmux/scanner_test.go`

**Interfaces:**
- Consumes: `git.Describe` (Task 1), `PaneInfo.Path` (Task 3), `ComputeDisplayNames` with `worktreeOf` (Task 4)
- Produces: `models.SessionDetails.Worktree` and `.RepoName`; `Session.Path` is now the pane's directory

- [ ] **Step 1: Add the detail fields**

In `internal/models/models.go`, add to `SessionDetails` after `GitBranch`:

```go
	Worktree     string // worktree directory basename, empty for a main checkout
	RepoName     string // repository name, shared by a repo's worktrees
```

- [ ] **Step 2: Write the failing test**

```go
func TestSessionDetailsCarryWorktree(t *testing.T) {
	var d models.SessionDetails
	d.Worktree = "the-site"
	d.RepoName = "app"
	if d.Worktree != "the-site" || d.RepoName != "app" {
		t.Errorf("details = %+v", d)
	}
}
```

- [ ] **Step 3: Run it**

Run: `go test ./internal/tmux/ -run TestSessionDetailsCarryWorktree -v`
Expected: PASS once Step 1 is done (a compile guard that the fields exist)

- [ ] **Step 4: Resolve paths per pane in `ScanSessions`**

Replace the body of the per-session loop so it resolves each pane's path first,
describes it once per unique directory, and looks up cwd-keyed hook state by the
pane path. Delete the now-unused `gitBranch` function and the `os/exec` import if
nothing else uses it.

```go
	var result []models.Session
	repos := make(map[string]git.Repo) // one git call per unique directory per scan
	describe := func(dir string) git.Repo {
		if r, ok := repos[dir]; ok {
			return r
		}
		r := git.Describe(dir)
		repos[dir] = r
		return r
	}

	for _, sess := range sessions {
		panes, ok := allPanes[sess.Name]
		if !ok {
			continue
		}

		// A pane may sit in a worktree, so resolve its own directory before
		// naming it or looking up its state.
		panePaths := make(map[string]string, len(panes))
		worktreeOf := make(map[string]string, len(panes))
		for _, pane := range panes {
			path := pane.Path
			if path == "" {
				path = sess.Path
			}
			panePaths[pane.PaneID] = path
			if wt := describe(path).Worktree; wt != "" {
				worktreeOf[pane.PaneID] = wt
			}
		}

		displayNames := ComputeDisplayNames(sess.Name, panes, worktreeOf)
		for _, pane := range panes {
			path := panePaths[pane.PaneID]

			var hookState models.SessionState
			var hasHook bool
			if pane.PaneID != "" {
				hookState, hasHook = paneStates[pane.PaneID]
			}
			if !hasHook {
				hookState, hasHook = cwdStates[path]
			}

			var status models.SessionStatus
			var lastMessage string
			var details models.SessionDetails

			if hasHook {
				s, ok := hookStateMap[hookState.State]
				if ok {
					status = s
				} else {
					status = models.StatusIdle
				}
				lastMessage = hookState.LastMessage
				details.LastActivity = hookState.Timestamp
				details.LastEvent = hookState.Event
			} else {
				status = models.StatusIdle
			}

			repo := describe(path)
			details.GitBranch = repo.Branch
			details.Worktree = repo.Worktree
			details.RepoName = repo.RepoName

			result = append(result, models.Session{
				Name:        displayNames[pane.PaneID],
				SessionID:   sess.SessionID,
				SessionName: sess.Name,
				Path:        path,
				WindowIndex: pane.WindowIndex,
				PaneIndex:   pane.PaneIndex,
				PaneID:      pane.PaneID,
				Status:      status,
				AgentType:   pane.AgentType,
				Details:     details,
				LastMessage: lastMessage,
			})
		}
	}
	return result
```

Add `"github.com/nemke/nagare-go/internal/git"` to the imports.

- [ ] **Step 5: Verify the whole package builds and passes**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output, no vet output, all packages PASS

- [ ] **Step 6: Commit**

```bash
git add internal/models/models.go internal/tmux/scanner.go internal/tmux/scanner_test.go
git commit -m "tmux: resolve status, path, and branch per pane instead of per session"
```

---

### Task 6: Show the worktree in the picker

**Files:**
- Modify: `internal/picker/picker.go:1195` (detail pane, next to the Branch row)

**Interfaces:**
- Consumes: `models.SessionDetails.Worktree` from Task 5
- Produces: nothing

- [ ] **Step 1: Add the Worktree row**

The detail pane renders Branch at `internal/picker/picker.go:1195`. Directly after
that block, add:

```go
	if s.Details.Worktree != "" {
		info.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Worktree"), val.Render(s.Details.Worktree)))
	}
```

- [ ] **Step 2: Verify it builds and the picker tests pass**

Run: `go test ./internal/picker/ -v`
Expected: PASS

- [ ] **Step 3: Verify against the live session**

Run: `./compile.bash && ./nagare-go tool list_agents`
Expected: the two `cosmic-platform-frontend` panes now print as
`cosmic-platform-frontend/the-site` and
`cosmic-platform-frontend/fluttering-watching-gadget`, each with its own
`.claude/worktrees/...` path.

- [ ] **Step 4: Commit**

```bash
git add internal/picker/picker.go
git commit -m "picker: show the worktree in session details"
```

---

### Task 7: Documentation

**Files:**
- Modify: `CLAUDE.md` (Architecture package list)
- Modify: `README.md` (Features)

**Interfaces:**
- Consumes: everything above
- Produces: nothing

- [ ] **Step 1: Document the package and the behaviour**

In `CLAUDE.md`, add to the Architecture list after the `internal/tmux` entry:

```markdown
- `internal/git` — resolves a directory into branch, repo name, and worktree name (one `rev-parse` per path)
```

Update the `internal/tmux` entry to mention per-pane paths:

```markdown
- `internal/tmux` — scanner (list-panes + /proc descendant walk), per-pane paths and worktree resolution, status detection (pane scraping)
```

Add a short subsection under Architecture:

```markdown
### Worktrees

Each agent pane resolves its own directory from `pane_current_path`, not the tmux
session path, so panes in different worktrees of one repo get their own path, branch,
and name (`{session}/{worktree}`). Worktree detection is structural — the git common
dir's parent differs from the toplevel — so hand-made worktrees work the same as
Claude Code's `.claude/worktrees/`.
```

In `README.md`, add to the Features list after the Real-time Status bullet:

```markdown
- **Worktree Aware** — panes in different git worktrees of one repo show their own path, branch, and name
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: describe per-pane worktree resolution"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| Per-pane path from `pane_current_path` | 3, 5 |
| `internal/git` package with `Describe`/`Repo` | 1 |
| Relative common dir, detached HEAD, non-repo | 1, 2 |
| Per-scan memoization owned by `ScanSessions` | 5 |
| Replaces per-session `git branch --show-current` | 5 (deletes `gitBranch`) |
| Naming precedence, single-pane short-circuit, collisions | 4 |
| cwd-keyed hook state looked up by pane path | 5 |
| `SessionDetails.Worktree` / `.RepoName`, detail row | 5, 6 |
| Errors never fail a scan | 1 |
| Tests: parser table, display names, `ParseAllPanes`, real-git integration | 1, 2, 3, 4 |

No spec requirement is unassigned.

**Placeholder scan:** none — every code step carries the actual code.

**Type consistency:** `Repo{Branch, RepoName, Worktree, IsWorktree}` is defined in
Task 1 and used with those exact names in Tasks 2 and 5.
`ComputeDisplayNames(sessName, panes, worktreeOf)` is defined in Task 4 and called
with that arity in Task 5. `SessionDetails.Worktree` / `.RepoName` are added in
Task 5 and consumed in Task 6.
