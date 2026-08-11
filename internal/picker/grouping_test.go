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
