package picker

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/theme"
)

const (
	// rowGutter is the blank column kept at the right edge of a list row or
	// grid cell, so nothing renders flush against the panel border.
	rowGutter = 1
	// minNameWidth is the floor for a name column. Below this a name is all
	// ellipsis and conveys nothing, so narrow panels overflow instead.
	minNameWidth = 6
	// ellipsis marks a shortened name. One cell, unlike "..." — on a 30-column
	// panel those two extra cells are two more characters of the actual name.
	ellipsis = "…"
)

// fitBox pins a panel to exactly w×h cells, border and padding included.
//
// Both halves are needed. Width/Height only pad *up* to the target, so a
// wrapped path or an over-long preview line still pushes a panel past its
// budget; MaxWidth/MaxHeight clip it back. Getting this wrong is not cosmetic
// — a frame one row too tall scrolls the alt screen and smears the whole UI.
//
// Note these are lipgloss v2 semantics, where Width/Height are the TOTAL
// rendered size. In v1 they excluded the border, so panels were sized as
// Width(outer-2); carrying that over unchanged left every panel two cells
// short in each dimension.
func fitBox(s lipgloss.Style, w, h int) lipgloss.Style {
	return s.Width(w).Height(h).MaxWidth(w).MaxHeight(h)
}

// truncate shortens a display name to at most maxWidth terminal cells,
// appending an ellipsis when it had to cut. Width is measured in cells, not
// runes: CJK and emoji session names occupy two columns each, and counting
// runes would let them overflow the panel.
func truncate(name string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(name) <= maxWidth {
		return name
	}
	return ansi.Truncate(name, maxWidth, ellipsis)
}

// statusDot renders the colored ● marker for a session's status, with a
// low-amplitude breathing Faint toggle on running/waiting sessions driven
// by the 1Hz pulse tick. Idle and dead sessions render flat.
func statusDot(status models.SessionStatus, pulseOn bool) string {
	base := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(status)))
	if pulseOn && (status == models.StatusRunning || status == models.StatusWaitingInput) {
		base = base.Faint(true)
	}
	return base.Render("●")
}

// renderNameWithMatches renders a (possibly truncated) display string for a
// session, highlighting runes that match the current fuzzy query with the
// accent color only (no bold, no background — per television / atuin
// convention). The match runs against the *original* full name so
// highlights stay aligned even when the display text was shortened with
// an ellipsis.
//
// base is the style applied to unmatched runes. When query is empty the
// base style is applied unchanged — zero allocations in the common case.
func renderNameWithMatches(display, full, query string, base lipgloss.Style, accent color.Color) string {
	if query == "" {
		return base.Render(display)
	}
	matches := fuzzy.Find(query, []string{full})
	if len(matches) == 0 || len(matches[0].MatchedIndexes) == 0 {
		return base.Render(display)
	}

	displayRunes := []rune(display)
	// Display is a prefix of full (plus an ellipsis when truncated), so match
	// indexes line up rune-for-rune over the prefix. The ellipsis itself
	// stands in for the cut-off tail and must never be highlighted.
	limit := len(displayRunes)
	if strings.HasSuffix(display, ellipsis) {
		limit--
	}
	hit := make(map[int]struct{}, len(matches[0].MatchedIndexes))
	for _, idx := range matches[0].MatchedIndexes {
		if idx >= 0 && idx < limit {
			hit[idx] = struct{}{}
		}
	}
	if len(hit) == 0 {
		return base.Render(display)
	}

	matched := base.Foreground(accent)
	var b []byte
	for i, r := range displayRunes {
		if _, ok := hit[i]; ok {
			b = append(b, matched.Render(string(r))...)
		} else {
			b = append(b, base.Render(string(r))...)
		}
	}
	return string(b)
}

// sectionHeader renders an inline section title followed by a dim fill
// line (crush / gh-dash convention). No surrounding box — tittle tiles
// look competing inside an already-bordered panel.
//
//	Keyboard Shortcuts ───────────────────────
func sectionHeader(title string, width int) string {
	c := theme.Current().Colors
	titleStyled := lipgloss.NewStyle().Foreground(c.Primary).Bold(true).Render(title)
	fillWidth := width - lipgloss.Width(titleStyled) - 1
	if fillWidth < 1 {
		return titleStyled
	}
	fill := lipgloss.NewStyle().Foreground(c.Muted).Render(strings.Repeat("─", fillWidth))
	return titleStyled + " " + fill
}

// Style functions — always build fresh from theme.Current() so theme
// switches take effect on the next View() call.

func baseStyle() lipgloss.Style {
	c := theme.Current().Colors
	return lipgloss.NewStyle().
		Foreground(c.Foreground).
		Background(c.Background)
}

// panelStyle is the secondary/detail panel border (preview metadata,
// empty states, etc.) — rounded, border-color accent.
func panelStyle() lipgloss.Style {
	c := theme.Current().Colors
	return baseStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(c.Border).
		BorderBackground(c.Background).
		Padding(1)
}

// primaryPanelStyle is the "focus-worthy" panel — the list where keystrokes
// land. Its border is tinted with the accent color so the eye knows where
// focus sits without reading any text.
func primaryPanelStyle() lipgloss.Style {
	c := theme.Current().Colors
	return baseStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(c.Accent).
		BorderBackground(c.Background).
		Padding(1)
}

func previewPanelStyle() lipgloss.Style {
	c := theme.Current().Colors
	return baseStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(c.Muted).
		BorderBackground(c.Background).
		Padding(0, 1)
}

func titleStyle() lipgloss.Style {
	c := theme.Current().Colors
	return lipgloss.NewStyle().
		Foreground(c.Primary).
		Bold(true)
}

func mutedStyle() lipgloss.Style {
	c := theme.Current().Colors
	return lipgloss.NewStyle().
		Foreground(c.Muted)
}

func dialogStyle() lipgloss.Style {
	c := theme.Current().Colors
	return lipgloss.NewStyle().
		Background(c.Background).
		Foreground(c.Foreground).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(c.Primary).
		BorderBackground(c.Background)
}
