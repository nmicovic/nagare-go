package picker

import (
	"testing"

	"github.com/nemke/nagare-go/internal/models"
)

// wt builds a session that sits in a worktree of sessName.
func wt(sessName, worktree string, status models.SessionStatus) models.Session {
	return models.Session{
		Name:        sessName + "/" + worktree,
		SessionName: sessName,
		Status:      status,
		AgentType:   models.AgentClaude,
		Details:     models.SessionDetails{Worktree: worktree, RepoName: sessName},
	}
}

// solo builds a single-pane session with no worktree.
func solo(name string, status models.SessionStatus) models.Session {
	return models.Session{
		Name:        name,
		SessionName: name,
		Status:      status,
		AgentType:   models.AgentClaude,
	}
}

func TestBuildRowsUngrouped(t *testing.T) {
	sessions := []models.Session{
		solo("nagare", models.StatusIdle),
		solo("propco", models.StatusIdle),
	}
	rows := buildRows(sessions)

	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (no headers for singletons)", len(rows))
	}
	for i, r := range rows {
		if r.SessionIdx != i {
			t.Errorf("row %d SessionIdx = %d, want %d", i, r.SessionIdx, i)
		}
		if r.Glyph != "" {
			t.Errorf("row %d has glyph %q, want none", i, r.Glyph)
		}
	}
	if rows[0].Label != "nagare" {
		t.Errorf("label = %q, want the full name", rows[0].Label)
	}
}

func TestBuildRowsOneGroup(t *testing.T) {
	sessions := []models.Session{
		wt("cosmic-platform-frontend", "shipping", models.StatusWaitingInput),
		wt("cosmic-platform-frontend", "the-site", models.StatusRunning),
		wt("cosmic-platform-frontend", "fluttering-watching-gadget", models.StatusIdle),
	}
	rows := buildRows(sessions)

	// header + 3 children
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}
	if rows[0].SessionIdx != -1 {
		t.Errorf("row 0 should be a header, got SessionIdx %d", rows[0].SessionIdx)
	}
	if rows[0].Group != "cosmic-platform-frontend" || rows[0].Count != 3 {
		t.Errorf("header = %+v, want group cosmic-platform-frontend count 3", rows[0])
	}
	wantLabels := []string{"shipping", "the-site", "fluttering-watching-gadget"}
	wantGlyphs := []string{glyphMid, glyphMid, glyphEnd}
	for i := range wantLabels {
		r := rows[i+1]
		if r.Label != wantLabels[i] {
			t.Errorf("child %d label = %q, want %q", i, r.Label, wantLabels[i])
		}
		if r.Glyph != wantGlyphs[i] {
			t.Errorf("child %d glyph = %q, want %q", i, r.Glyph, wantGlyphs[i])
		}
		if r.SessionIdx != i {
			t.Errorf("child %d SessionIdx = %d, want %d", i, r.SessionIdx, i)
		}
	}
}

func TestBuildRowsTwoGroupsAndSingleton(t *testing.T) {
	sessions := []models.Session{
		wt("frontend", "a", models.StatusIdle),
		wt("frontend", "b", models.StatusIdle),
		solo("nagare", models.StatusIdle),
		wt("backend", "x", models.StatusIdle),
		wt("backend", "y", models.StatusIdle),
	}
	rows := buildRows(sessions)

	// 2 headers + 4 children + 1 singleton
	if len(rows) != 7 {
		t.Fatalf("got %d rows, want 7", len(rows))
	}
	var headers []string
	for _, r := range rows {
		if r.SessionIdx == -1 {
			headers = append(headers, r.Group)
		}
	}
	if len(headers) != 2 || headers[0] != "frontend" || headers[1] != "backend" {
		t.Errorf("headers = %v, want [frontend backend]", headers)
	}
	// The singleton keeps its full name and gets no header.
	for _, r := range rows {
		if r.SessionIdx == 2 && (r.Label != "nagare" || r.Glyph != "") {
			t.Errorf("singleton row = %+v, want plain nagare", r)
		}
	}
}

// Every session index must appear exactly once, or the cursor can land on a
// session that has no row to highlight.
func TestBuildRowsCoversEverySession(t *testing.T) {
	sessions := []models.Session{
		wt("frontend", "a", models.StatusIdle),
		wt("frontend", "b", models.StatusIdle),
		solo("nagare", models.StatusIdle),
	}
	seen := make(map[int]int)
	for _, r := range buildRows(sessions) {
		if r.SessionIdx >= 0 {
			seen[r.SessionIdx]++
		}
	}
	for i := range sessions {
		if seen[i] != 1 {
			t.Errorf("session %d appears in %d rows, want exactly 1", i, seen[i])
		}
	}
}

func TestChildLabel(t *testing.T) {
	tests := []struct {
		name string
		s    models.Session
		want string
	}{
		{
			name: "worktree wins",
			s: models.Session{
				Name: "repo/custom", SessionName: "repo",
				Details: models.SessionDetails{Worktree: "the-site"},
			},
			want: "the-site",
		},
		{
			name: "falls back to the suffix after the session name",
			s:    models.Session{Name: "repo/claude_02", SessionName: "repo"},
			want: "claude_02",
		},
		{
			name: "custom window name suffix",
			s:    models.Session{Name: "repo/review", SessionName: "repo"},
			want: "review",
		},
		{
			name: "bare name when there is no prefix to strip",
			s:    models.Session{Name: "nagare", SessionName: "nagare"},
			want: "nagare",
		},
	}
	for _, tt := range tests {
		if got := childLabel(tt.s); got != tt.want {
			t.Errorf("%s: childLabel() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// names returns the visual order of a sorted session slice.
func names(sessions []models.Session) []string {
	out := make([]string, len(sessions))
	for i, s := range sessions {
		out[i] = s.Name
	}
	return out
}

// A waiting child must lift its whole repo above a quiet one, otherwise
// grouping buries the one thing the picker exists to surface.
func TestSortFilteredWaitingChildLiftsGroup(t *testing.T) {
	m := Model{sortMode: SortByStatus}
	m.filtered = []models.Session{
		solo("aaa-quiet", models.StatusIdle),
		wt("zzz-busy", "one", models.StatusIdle),
		wt("zzz-busy", "two", models.StatusWaitingInput),
	}
	m.sortFiltered()

	if got := names(m.filtered)[0]; got != "zzz-busy/two" && got != "zzz-busy/one" {
		t.Errorf("first row = %q, want a zzz-busy member (it has a waiting child)", got)
	}
	if last := names(m.filtered)[2]; last != "aaa-quiet" {
		t.Errorf("last row = %q, want aaa-quiet", last)
	}
}

// Group members must stay contiguous or buildRows emits duplicate headers.
func TestSortFilteredKeepsGroupsContiguous(t *testing.T) {
	m := Model{sortMode: SortByStatus}
	m.filtered = []models.Session{
		wt("repo-a", "one", models.StatusIdle),
		wt("repo-b", "one", models.StatusWaitingInput),
		wt("repo-a", "two", models.StatusWaitingInput),
		wt("repo-b", "two", models.StatusIdle),
	}
	m.sortFiltered()

	seen := map[string]bool{}
	prev := ""
	for _, s := range m.filtered {
		key := groupKeyOf(s)
		if key != prev {
			if seen[key] {
				t.Fatalf("group %q is not contiguous: %v", key, names(m.filtered))
			}
			seen[key] = true
			prev = key
		}
	}
}

// Within a group the active sort mode still applies.
func TestSortFilteredWithinGroupFollowsMode(t *testing.T) {
	m := Model{sortMode: SortByName}
	m.filtered = []models.Session{
		wt("repo", "zebra", models.StatusIdle),
		wt("repo", "alpha", models.StatusIdle),
	}
	m.sortFiltered()

	if got := names(m.filtered); got[0] != "repo/alpha" {
		t.Errorf("order = %v, want alpha first under SortByName", got)
	}
}

// With no groups at all, ordering must match the old flat behaviour.
func TestSortFilteredSingletonRegression(t *testing.T) {
	m := Model{sortMode: SortByStatus}
	m.filtered = []models.Session{
		solo("idle-one", models.StatusIdle),
		solo("dead-one", models.StatusDead),
		solo("waiting-one", models.StatusWaitingInput),
		solo("running-one", models.StatusRunning),
	}
	m.sortFiltered()

	want := []string{"waiting-one", "running-one", "idle-one", "dead-one"}
	got := names(m.filtered)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
