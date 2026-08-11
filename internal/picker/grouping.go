package picker

import (
	"strings"

	"github.com/nemke/nagare-go/internal/models"
)

// Tree glyphs prefixing a grouped session's label.
const (
	glyphMid = "├"
	glyphEnd = "└"
)

// listRow is one rendered line in the list view: either a group header or a
// session sitting under one. Only session rows are selectable, so headers carry
// SessionIdx -1.
//
// Rows are derived per frame rather than stored, which keeps them from drifting
// out of sync with the session slice they describe.
type listRow struct {
	SessionIdx int    // index into the session slice; -1 for a header
	Label      string // worktree name for a child, full name otherwise
	Group      string // header text (headers only)
	Count      int    // members in the group (headers only)
	Glyph      string // glyphMid / glyphEnd for children, empty otherwise
}

// groupKeyOf returns the key sessions are grouped by: the tmux session name,
// which every pane of one repo shares.
func groupKeyOf(s models.Session) string {
	if s.SessionName != "" {
		return s.SessionName
	}
	return s.Name
}

// childLabel returns the text a grouped session shows. The repo name lives on
// the header, so a child only needs to say which worktree or pane it is.
func childLabel(s models.Session) string {
	if s.Details.Worktree != "" {
		return s.Details.Worktree
	}
	if s.SessionName != "" {
		if rest, ok := strings.CutPrefix(s.Name, s.SessionName+"/"); ok && rest != "" {
			return rest
		}
	}
	return s.Name
}

// buildRows turns a session list into display rows, inserting a header above
// each group of two or more sessions sharing a tmux session name. A lone
// session renders as it always has: no header, no glyph, full name.
//
// Sessions must already be in visual order, with group members contiguous —
// sortFiltered guarantees this. Non-contiguous members would produce a header
// per run rather than corrupt the mapping.
func buildRows(sessions []models.Session) []listRow {
	counts := make(map[string]int, len(sessions))
	for _, s := range sessions {
		counts[groupKeyOf(s)]++
	}

	rows := make([]listRow, 0, len(sessions))
	for i, s := range sessions {
		key := groupKeyOf(s)
		if counts[key] < 2 {
			rows = append(rows, listRow{SessionIdx: i, Label: s.Name})
			continue
		}

		if i == 0 || groupKeyOf(sessions[i-1]) != key {
			rows = append(rows, listRow{SessionIdx: -1, Group: key, Count: counts[key]})
		}

		glyph := glyphMid
		if i == len(sessions)-1 || groupKeyOf(sessions[i+1]) != key {
			glyph = glyphEnd
		}
		rows = append(rows, listRow{SessionIdx: i, Label: childLabel(s), Glyph: glyph})
	}
	return rows
}
