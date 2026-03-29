package picker

// Overlay placement based on the approach from
// github.com/opencode-ai/opencode and lipgloss PR #102.

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// placeOverlay renders fg centered on top of bg, preserving bg content around it.
func placeOverlay(width, height int, fg, bg string) string {
	fgLines, fgWidth := getLines(fg)
	bgLines, _ := getLines(bg)

	for len(bgLines) < height {
		bgLines = append(bgLines, "")
	}

	fgHeight := len(fgLines)

	x := (width - fgWidth) / 2
	y := (height - fgHeight) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	var b strings.Builder
	for i, bgLine := range bgLines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i < y || i >= y+fgHeight {
			b.WriteString(bgLine)
			continue
		}

		// Left portion of bg
		pos := 0
		if x > 0 {
			left := ansi.Truncate(bgLine, x, "")
			leftW := ansi.StringWidth(left)
			b.WriteString(left)
			pos = leftW
			if pos < x {
				b.WriteString(strings.Repeat(" ", x-pos))
				pos = x
			}
		}

		// Overlay content
		fgLine := fgLines[i-y]
		b.WriteString(fgLine)
		pos += ansi.StringWidth(fgLine)

		// Right portion of bg — use ansi.Cut to skip past the overlay region
		bgW := lipgloss.Width(bgLine)
		right := ansi.Cut(bgLine, pos, bgW)
		rightW := ansi.StringWidth(right)

		// Fill gap between fg end and right bg start
		if gap := bgW - rightW - pos; gap > 0 {
			b.WriteString(strings.Repeat(" ", gap))
		}
		b.WriteString(right)
	}

	return b.String()
}

func getLines(s string) ([]string, int) {
	lines := strings.Split(s, "\n")
	widest := 0
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > widest {
			widest = w
		}
	}
	return lines, widest
}
