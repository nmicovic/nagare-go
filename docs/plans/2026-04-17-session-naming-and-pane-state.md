# Session Naming & Per-Pane Hook State Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the picker, hook state, and MCP messaging all correctly handle multiple agents inside the same tmux session (e.g., two `claude` panes in a `cosmo-ai` session).

**Architecture:**
- Each agent pane is identified by its tmux `pane_id` (e.g. `%23`), which is stable for the pane's lifetime.
- Display names in the picker stay as the bare tmux session name when a session has only one agent pane, and get a `{sessionName}/{agent}_NN` suffix (or `{sessionName}/{customWindowName}`) when multiple panes share a tmux session.
- Hook state files are keyed by pane_id (not cwd), so two agents in the same directory keep distinct status.
- MCP session resolution uses `$TMUX_PANE` to disambiguate "my session" when multiple panes share a cwd; the inbox filesystem layout sanitizes `/` in names.

**Tech Stack:** Go 1.22+, standard library (no new deps).

---

## Design Summary

### 1. Display names (scanner.go)
For each tmux session:
- **1 agent pane** → display = `sess.Name` (unchanged behavior).
- **>1 agent panes** → for each pane (sorted by `(window_index, pane_index)`):
  - If window name is custom (passes `isCustomWindowName`) → `sess.Name + "/" + windowName`.
  - Else → `sess.Name + "/" + agent + "_NN"` where `NN` is a 1-based counter per agent type within the session (e.g. `claude_01`, `claude_02`).

This is a behavior change for existing users who rely on custom window names — they'll now see `sess.Name/windowName` rather than just `windowName`. Acceptable since it removes ambiguity.

### 2. Hook state keyed by pane_id
- `SessionState` gains a `PaneID string` field (JSON `pane_id`).
- Hook handler reads `os.Getenv("TMUX_PANE")` and writes it into the state file.
- Scanner includes `#{pane_id}` in its `list-panes` format string and populates `Session.PaneID`.
- `state.LoadStatesByPaneID(dir)` returns `map[paneID]SessionState` (liveness/timestamp conflict rules unchanged).
- Scanner uses pane_id for the hook-state lookup; if a pane's state has no pane_id (pre-existing state files from before this change), falls back to cwd-match.

### 3. MCP messaging
- Sanitize session names before using them as directory components: replace `/` with `__`. Apply in `InboxDir` so writes, reads, and the `os.ReadDir(baseDir)` walk in `CheckMessagesHandler` all agree.
- `findSession` (tools.go:244): exact match first, then prefix match on `{target}/...`. One match → use it. Zero → not found. Multiple → ambiguity error listing candidates.
- `resolveMySession` (tools.go:263): try `TMUX_PANE` pane_id match first, fall back to cwd match, fall back to registry.

---

## Task 1: Add PaneID to models

**Files:**
- Modify: `internal/models/models.go`
- Test: `internal/models/models_test.go` (no change — existing tests should still pass)

**Step 1: Add field to SessionState**

In `internal/models/models.go`, add a `PaneID` field (JSON `pane_id`) to `SessionState`:

```go
type SessionState struct {
    State            string `json:"state"`
    SessionID        string `json:"session_id"`
    Cwd              string `json:"cwd"`
    PaneID           string `json:"pane_id,omitempty"`
    Event            string `json:"event"`
    NotificationType string `json:"notification_type,omitempty"`
    LastMessage      string `json:"last_message,omitempty"`
    Timestamp        string `json:"timestamp"`
}
```

**Step 2: Add field to Session**

Add a `PaneID string` field to `Session`:

```go
type Session struct {
    Name        string
    SessionID   string
    SessionName string
    Path        string
    WindowIndex int
    PaneIndex   int
    PaneID      string // tmux pane id, e.g. "%23"
    Status      SessionStatus
    AgentType   AgentType
    Details     SessionDetails
    LastMessage string
}
```

**Step 3: Verify build**

Run: `go build ./...`
Expected: PASS (no behavioural change yet; fields are additive).

**Step 4: Commit**

```bash
git add internal/models/models.go
git commit -m "models: add PaneID to Session and SessionState"
```

---

## Task 2: Scanner collects pane_id

**Files:**
- Modify: `internal/tmux/scanner.go`
- Test: `internal/tmux/scanner_test.go`

**Step 1: Update PaneInfo and parser (write failing test)**

Add to `internal/tmux/scanner_test.go` a test asserting that `ParseAllPanes` picks up a trailing `pane_id` column:

```go
func TestParseAllPanesCapturesPaneID(t *testing.T) {
    raw := "work:0:0:claude:123:? claude:%7\n"
    got := ParseAllPanes(raw)
    panes := got["work"]
    if len(panes) != 1 {
        t.Fatalf("expected 1 pane, got %d", len(panes))
    }
    if panes[0].PaneID != "%7" {
        t.Errorf("PaneID = %q, want %q", panes[0].PaneID, "%7")
    }
}
```

Run: `go test ./internal/tmux/ -run TestParseAllPanesCapturesPaneID -v`
Expected: FAIL (PaneID field doesn't exist).

**Step 2: Add PaneID to PaneInfo and extend parser**

In `internal/tmux/scanner.go`:

```go
type PaneInfo struct {
    WindowIndex int
    PaneIndex   int
    AgentType   models.AgentType
    WindowName  string
    PaneID      string
}
```

Update `ParseAllPanes` to accept a 7-field format (add pane_id after window_name) — use `SplitN(line, ":", 7)` and read `parts[6]` when present.

**Step 3: Update the tmux format string in ScanSessions**

Change the `list-panes` format in `ScanSessions`:

```go
rawPanes := RunTmux("list-panes", "-a", "-F",
    "#{session_name}:#{window_index}:#{pane_index}:#{pane_current_command}:#{pane_pid}:#{window_name}:#{pane_id}")
```

Populate `Session.PaneID = pane.PaneID` in the result construction.

**Step 4: Run tests**

Run: `go test ./internal/tmux/ -v`
Expected: PASS (new test plus existing tests still work because the parser handles 6- and 7-field inputs).

**Step 5: Commit**

```bash
git add internal/tmux/scanner.go internal/tmux/scanner_test.go
git commit -m "tmux: capture pane_id in scanner"
```

---

## Task 3: Collision-aware display names

**Files:**
- Modify: `internal/tmux/scanner.go`
- Test: `internal/tmux/scanner_test.go`

**Step 1: Write failing test**

Add to `internal/tmux/scanner_test.go`:

```go
func TestComputeDisplayNames(t *testing.T) {
    sess := "cosmo-ai"
    panes := []PaneInfo{
        {WindowIndex: 1, PaneIndex: 0, AgentType: models.AgentClaude, WindowName: "terminal", PaneID: "%2"},
        {WindowIndex: 0, PaneIndex: 0, AgentType: models.AgentClaude, WindowName: "? claude", PaneID: "%1"},
    }
    got := ComputeDisplayNames(sess, panes)
    if got["%1"] != "cosmo-ai/claude_01" {
        t.Errorf("pane %%1 = %q, want cosmo-ai/claude_01", got["%1"])
    }
    if got["%2"] != "cosmo-ai/claude_02" {
        t.Errorf("pane %%2 = %q, want cosmo-ai/claude_02", got["%2"])
    }
}

func TestComputeDisplayNamesSinglePane(t *testing.T) {
    got := ComputeDisplayNames("work", []PaneInfo{
        {WindowIndex: 0, PaneIndex: 0, AgentType: models.AgentClaude, WindowName: "zsh", PaneID: "%3"},
    })
    if got["%3"] != "work" {
        t.Errorf("single pane name = %q, want work", got["%3"])
    }
}

func TestComputeDisplayNamesCustomWindowName(t *testing.T) {
    panes := []PaneInfo{
        {WindowIndex: 0, PaneIndex: 0, AgentType: models.AgentClaude, WindowName: "? claude", PaneID: "%1"},
        {WindowIndex: 1, PaneIndex: 0, AgentType: models.AgentClaude, WindowName: "planning", PaneID: "%2"},
    }
    got := ComputeDisplayNames("cosmo-ai", panes)
    if got["%1"] != "cosmo-ai/claude_01" {
        t.Errorf("pane %%1 = %q", got["%1"])
    }
    if got["%2"] != "cosmo-ai/planning" {
        t.Errorf("pane %%2 = %q", got["%2"])
    }
}

func TestComputeDisplayNamesMixedAgents(t *testing.T) {
    panes := []PaneInfo{
        {WindowIndex: 0, PaneIndex: 0, AgentType: models.AgentClaude, WindowName: "zsh", PaneID: "%1"},
        {WindowIndex: 1, PaneIndex: 0, AgentType: models.AgentGemini, WindowName: "zsh", PaneID: "%2"},
    }
    got := ComputeDisplayNames("proj", panes)
    if got["%1"] != "proj/claude_01" {
        t.Errorf("pane %%1 = %q", got["%1"])
    }
    if got["%2"] != "proj/gemini_01" {
        t.Errorf("pane %%2 = %q", got["%2"])
    }
}
```

Run: `go test ./internal/tmux/ -run TestComputeDisplayNames -v`
Expected: FAIL (ComputeDisplayNames not defined).

**Step 2: Implement ComputeDisplayNames**

Add to `internal/tmux/scanner.go`:

```go
import "sort"

// ComputeDisplayNames returns a map from pane_id to display name for a set of
// agent panes sharing a tmux session. When there's only one pane, the bare
// session name is used. When multiple panes share the session, panes with a
// custom window name use "{sessName}/{windowName}" and the rest get
// "{sessName}/{agent}_NN" (1-based, per agent type, ordered by window/pane).
func ComputeDisplayNames(sessName string, panes []PaneInfo) map[string]string {
    result := make(map[string]string, len(panes))
    if len(panes) == 1 {
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

    counts := make(map[models.AgentType]int)
    for _, p := range sorted {
        if isCustomWindowName(p.WindowName, sessName) {
            result[p.PaneID] = sessName + "/" + p.WindowName
            continue
        }
        counts[p.AgentType]++
        result[p.PaneID] = fmt.Sprintf("%s/%s_%02d", sessName, p.AgentType, counts[p.AgentType])
    }
    return result
}
```

**Step 3: Use it in ScanSessions**

Replace the current display-name logic in `ScanSessions`:

```go
for _, sess := range sessions {
    panes, ok := allPanes[sess.Name]
    if !ok {
        continue
    }
    displayNames := ComputeDisplayNames(sess.Name, panes)
    for _, pane := range panes {
        // ... existing hook-state/details logic ...
        result = append(result, models.Session{
            Name:        displayNames[pane.PaneID],
            // ... other fields ...
        })
    }
}
```

Remove the old `displayName := sess.Name; if len(panes) > 1 && isCustomWindowName(...) { displayName = pane.WindowName }` block.

**Step 4: Run tests**

Run: `go test ./internal/tmux/ -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tmux/scanner.go internal/tmux/scanner_test.go
git commit -m "tmux: collision-aware display names for multi-pane sessions"
```

---

## Task 4: Hook writes pane_id into state file

**Files:**
- Modify: `internal/hooks/hooks.go`
- Test: `internal/hooks/hooks_test.go`

**Step 1: Write failing test**

Hook tests likely don't cover pane_id yet. Add to `internal/hooks/hooks_test.go` a test that invokes hook handling (refactor if `Handle()` reads from stdin directly — you may need to extract a `processEvent(event HookEvent) models.SessionState` helper). If such extraction is infeasible without large refactor, skip and verify in Task 5's integration testing.

Concretely: if a helper `buildState(event HookEvent, paneID string) models.SessionState` can be extracted, test it. Otherwise note this and move on; coverage is via Task 6.

**Step 2: Thread TMUX_PANE through Handle**

In `internal/hooks/hooks.go`, after parsing the event and before writing the state file, capture `os.Getenv("TMUX_PANE")` and include it:

```go
newSessionState := models.SessionState{
    State:            newState,
    SessionID:        event.SessionID,
    Cwd:              event.Cwd,
    PaneID:           os.Getenv("TMUX_PANE"),
    Event:            event.HookEventName,
    NotificationType: event.NotificationType,
    LastMessage:      event.LastAssistantMessage,
    Timestamp:        now,
}
```

**Step 3: Run tests**

Run: `go test ./internal/hooks/ -v`
Expected: PASS (existing tests unaffected).

**Step 4: Commit**

```bash
git add internal/hooks/hooks.go
git commit -m "hooks: record TMUX_PANE in session state"
```

---

## Task 5: LoadStatesByPaneID

**Files:**
- Modify: `internal/state/state.go`
- Test: `internal/state/state_test.go`

**Step 1: Write failing test**

Add to `internal/state/state_test.go`:

```go
func TestLoadStatesByPaneID(t *testing.T) {
    dir := t.TempDir()
    mustWrite := func(s models.SessionState) {
        if err := WriteState(dir, s); err != nil {
            t.Fatal(err)
        }
    }
    mustWrite(models.SessionState{SessionID: "a", Cwd: "/x", PaneID: "%1", State: "working", Timestamp: "2026-04-17T12:00:00Z"})
    mustWrite(models.SessionState{SessionID: "b", Cwd: "/x", PaneID: "%2", State: "idle", Timestamp: "2026-04-17T12:00:01Z"})

    got := LoadStatesByPaneID(dir)
    if got["%1"].State != "working" {
        t.Errorf("%%1 state = %q", got["%1"].State)
    }
    if got["%2"].State != "idle" {
        t.Errorf("%%2 state = %q", got["%2"].State)
    }
}

func TestLoadStatesByPaneIDSkipsEmpty(t *testing.T) {
    dir := t.TempDir()
    if err := WriteState(dir, models.SessionState{SessionID: "a", Cwd: "/x", Timestamp: "2026-04-17T12:00:00Z"}); err != nil {
        t.Fatal(err)
    }
    got := LoadStatesByPaneID(dir)
    if len(got) != 0 {
        t.Errorf("expected empty map, got %v", got)
    }
}
```

Run: `go test ./internal/state/ -run TestLoadStatesByPaneID -v`
Expected: FAIL (function not defined).

**Step 2: Implement LoadStatesByPaneID**

Add to `internal/state/state.go` (mirrors `LoadAllStates` but keyed by PaneID; skips entries with empty PaneID):

```go
func LoadStatesByPaneID(dir string) map[string]models.SessionState {
    states := make(map[string]models.SessionState)
    entries, err := os.ReadDir(dir)
    if err != nil {
        return states
    }
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
            continue
        }
        data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
        if err != nil {
            continue
        }
        var s models.SessionState
        if err := json.Unmarshal(data, &s); err != nil {
            continue
        }
        if s.PaneID == "" {
            continue
        }
        existing, exists := states[s.PaneID]
        if !exists {
            states[s.PaneID] = s
            continue
        }
        if existing.State == "dead" && s.State != "dead" {
            states[s.PaneID] = s
        } else if existing.State != "dead" && s.State == "dead" {
            // keep existing
        } else if s.Timestamp > existing.Timestamp {
            states[s.PaneID] = s
        }
    }
    return states
}
```

**Step 3: Run tests**

Run: `go test ./internal/state/ -v`
Expected: PASS.

**Step 4: Commit**

```bash
git add internal/state/state.go internal/state/state_test.go
git commit -m "state: add LoadStatesByPaneID"
```

---

## Task 6: Scanner uses pane_id for state lookup

**Files:**
- Modify: `internal/tmux/scanner.go`
- Test: `internal/tmux/scanner_test.go`

**Step 1: Change ScanSessions signature**

Change `ScanSessions` to accept both maps (caller computes pane_id map; keeps cwd map as fallback). Or — simpler — move state loading into scanner via a new constructor. Simpler: change the signature:

```go
func ScanSessions(paneStates map[string]models.SessionState, cwdStates map[string]models.SessionState) []models.Session
```

Update callers in:
- `cmd/picker.go` (or wherever ScanSessions is invoked)
- `internal/mcp/tools.go` scanAll()
- any test helpers

Each caller loads both:

```go
paneStates := state.LoadStatesByPaneID(state.DefaultStatesDir())
cwdStates := state.LoadAllStates(state.DefaultStatesDir())
sessions := tmux.ScanSessions(paneStates, cwdStates)
```

**Step 2: Lookup logic in ScanSessions**

Replace `hookState, hasHook := hookStates[sess.Path]` with:

```go
var hookState models.SessionState
var hasHook bool
if pane.PaneID != "" {
    hookState, hasHook = paneStates[pane.PaneID]
}
if !hasHook {
    hookState, hasHook = cwdStates[sess.Path]
}
```

**Step 3: Find & update all ScanSessions callers**

Run: `grep -rn "ScanSessions(" --include="*.go"` (through the Grep tool) and update each.

**Step 4: Run all tests**

Run: `go test ./... -v`
Expected: PASS.

Run: `go vet ./...`
Expected: no warnings.

**Step 5: Commit**

```bash
git add -A
git commit -m "tmux: correlate hook state per pane with cwd fallback"
```

---

## Task 7: Sanitize session names for MCP inbox paths

**Files:**
- Modify: `internal/mcp/mcp.go`
- Test: `internal/mcp/mcp_test.go` (create if missing)

**Step 1: Write failing test**

Add:

```go
func TestInboxDirSanitizesSlashes(t *testing.T) {
    got := InboxDir("cosmo-ai/claude_01")
    if strings.Contains(filepath.Base(got), "/") {
        t.Errorf("InboxDir leaked slash into directory component: %q", got)
    }
    if filepath.Base(got) != "cosmo-ai__claude_01" {
        t.Errorf("got %q, want ...cosmo-ai__claude_01", got)
    }
}
```

Run: `go test ./internal/mcp/ -run TestInboxDirSanitizesSlashes -v`
Expected: FAIL.

**Step 2: Implement sanitization**

In `internal/mcp/mcp.go`:

```go
// sanitizeName replaces filesystem-unsafe characters in session names so they
// can be used as directory components under MessagesDir.
func sanitizeName(name string) string {
    return strings.ReplaceAll(name, "/", "__")
}

func InboxDir(sessionName string) string {
    return filepath.Join(MessagesDir(), sanitizeName(sessionName))
}
```

Audit call sites of session-name-as-path: `MessagePath`, `InboxDir`, and `CheckMessagesHandler`'s `d.Name() == mySession` check (tools.go:195). That comparison is `d.Name()` (sanitized) vs `mySession` (display). Fix by comparing against `sanitizeName(mySession)`:

```go
if !d.IsDir() || d.Name() == sanitizeName(mySession) {
    continue
}
msgs, _ := ListInbox(d.Name()) // d.Name() is already the on-disk name
```

But note: `ListInbox(d.Name())` then calls `InboxDir(d.Name())` which *re-sanitizes*. Since `d.Name()` is already sanitized and has no `/`, re-sanitization is a no-op. Safe.

Export `sanitizeName` if needed for tests (lowercase is fine since test is in same package).

**Step 3: Run tests**

Run: `go test ./internal/mcp/ -v`
Expected: PASS.

**Step 4: Commit**

```bash
git add internal/mcp/mcp.go internal/mcp/mcp_test.go
git commit -m "mcp: sanitize session names for inbox paths"
```

---

## Task 8: findSession prefix-match disambiguation

**Files:**
- Modify: `internal/mcp/tools.go`
- Test: `internal/mcp/tools_test.go` (create if missing)

**Step 1: Refactor findSession to be testable**

Extract a pure function:

```go
func resolveSession(name string, sessions []models.Session) (models.Session, error) {
    // exact match first
    for _, s := range sessions {
        if s.Name == name {
            return s, nil
        }
    }
    // prefix match on "{name}/..."
    prefix := name + "/"
    var matches []models.Session
    for _, s := range sessions {
        if strings.HasPrefix(s.Name, prefix) {
            matches = append(matches, s)
        }
    }
    switch len(matches) {
    case 0:
        return models.Session{}, fmt.Errorf("session '%s' not found", name)
    case 1:
        return matches[0], nil
    default:
        names := make([]string, len(matches))
        for i, m := range matches {
            names[i] = m.Name
        }
        return models.Session{}, fmt.Errorf("'%s' is ambiguous — matches %s. Please specify one.",
            name, strings.Join(names, ", "))
    }
}

func findSession(name string) (models.Session, error) {
    return resolveSession(name, scanAll())
}
```

**Step 2: Write tests**

```go
func TestResolveSessionExact(t *testing.T) {
    sessions := []models.Session{
        {Name: "cosmo-ai"},
        {Name: "cosmo-ai/claude_01"},
    }
    got, err := resolveSession("cosmo-ai", sessions)
    if err != nil {
        t.Fatal(err)
    }
    if got.Name != "cosmo-ai" {
        t.Errorf("got %q", got.Name)
    }
}

func TestResolveSessionPrefix(t *testing.T) {
    sessions := []models.Session{{Name: "cosmo-ai/claude_01"}}
    got, err := resolveSession("cosmo-ai", sessions)
    if err != nil {
        t.Fatal(err)
    }
    if got.Name != "cosmo-ai/claude_01" {
        t.Errorf("got %q", got.Name)
    }
}

func TestResolveSessionAmbiguous(t *testing.T) {
    sessions := []models.Session{
        {Name: "cosmo-ai/claude_01"},
        {Name: "cosmo-ai/claude_02"},
    }
    _, err := resolveSession("cosmo-ai", sessions)
    if err == nil {
        t.Fatal("expected ambiguity error")
    }
    if !strings.Contains(err.Error(), "ambiguous") {
        t.Errorf("error = %v", err)
    }
    if !strings.Contains(err.Error(), "cosmo-ai/claude_01") || !strings.Contains(err.Error(), "cosmo-ai/claude_02") {
        t.Errorf("error should list candidates: %v", err)
    }
}

func TestResolveSessionNotFound(t *testing.T) {
    _, err := resolveSession("nope", []models.Session{{Name: "other"}})
    if err == nil || !strings.Contains(err.Error(), "not found") {
        t.Errorf("expected not-found error, got %v", err)
    }
}
```

Run: `go test ./internal/mcp/ -v`
Expected: PASS.

**Step 3: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "mcp: resolve ambiguous targets with prefix-match error"
```

---

## Task 9: resolveMySession uses TMUX_PANE

**Files:**
- Modify: `internal/mcp/tools.go`

**Step 1: Update resolveMySession**

Add a pane_id check before the cwd loop:

```go
func resolveMySession() string {
    paneID := os.Getenv("TMUX_PANE")
    cwd, err := os.Getwd()
    if err != nil {
        cwd = ""
    }

    sessions := scanAll()

    // 1. Match by pane_id (unambiguous when multiple agents share a cwd)
    if paneID != "" {
        for _, s := range sessions {
            if s.PaneID == paneID {
                return s.Name
            }
        }
    }

    // 2. Fall back to cwd match
    if cwd != "" {
        for _, s := range sessions {
            if s.Path == cwd {
                return s.Name
            }
        }
    }

    // 3. Fall back to registry
    reg := state.NewRegistry(state.DefaultRegistryPath())
    if cwd != "" {
        if s := reg.FindByPath(cwd); s != nil {
            return s.Name
        }
    }
    if cwd == "" {
        return "unknown"
    }
    return filepath.Base(cwd)
}
```

**Step 2: Run tests**

Run: `go test ./... -v`
Run: `go vet ./...`
Expected: PASS.

**Step 3: Commit**

```bash
git add internal/mcp/tools.go
git commit -m "mcp: disambiguate self via TMUX_PANE"
```

---

## Task 10: End-to-end verification

**Files:** none modified.

**Step 1: Build**

Run: `./compile.bash`
Expected: builds cleanly.

**Step 2: Manual smoke test**

In the `cosmo-ai` tmux session with two claude panes:
1. Launch `./nagare-go` (picker) from another shell.
2. Verify both panes appear as `cosmo-ai/claude_01` and `cosmo-ai/claude_02`.
3. Trigger a hook from one pane (run any command in Claude Code that fires a hook, e.g. `UserPromptSubmit`). Confirm only that pane's row updates in the picker.
4. From one agent, call MCP `list_agents` — both `cosmo-ai/claude_*` entries should be listed.
5. From one agent, call `send_message(target="cosmo-ai", message="...")` — expect ambiguity error with both candidate names.
6. `send_message(target="cosmo-ai/claude_02", message="...")` — should route to window 1 and be readable in pane 2.

**Step 3: Report and hand back**

Report what was verified. No commit for this task.

---

## Rollback notes

If anything goes wrong mid-plan: each task is a separate commit, revert individually. Task 6 is the riskiest (signature change for `ScanSessions` touches every caller); if it breaks anything at runtime that tests didn't catch, revert that one commit and ScanSessions falls back to the cwd-only logic from Task 5 — still a big UX improvement since pane_id is being written.
