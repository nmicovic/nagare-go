package picker

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"charm.land/lipgloss/v2"
)

// TestGridRiseTableSettles — the shared spring table is computed once at init, so a
// table that never reached zero would leave every card permanently offset.
func TestGridRiseTableSettles(t *testing.T) {
	if len(gridRiseOffsets) == 0 {
		t.Fatal("rise table is empty")
	}
	if got := gridRiseOffsets[0]; got != int(gridRise) {
		t.Errorf("table starts at %d, want %v", got, gridRise)
	}
	if got := gridRiseOffsets[len(gridRiseOffsets)-1]; got != 0 {
		t.Errorf("table ends at %d, want 0", got)
	}
	prev := gridRiseOffsets[0]
	for i, v := range gridRiseOffsets {
		if v > prev {
			t.Errorf("offset rose at frame %d: %d then %d", i, prev, v)
		}
		if v < 0 {
			t.Errorf("negative offset %d at frame %d", v, i)
		}
		prev = v
	}
}

// TestGridEntryStaggers — the point is that cards arrive in order, so a later card
// must not start before an earlier one has.
func TestGridEntryStaggers(t *testing.T) {
	const cards = 6
	var g gridEntry
	g.start()

	started := make([]int, cards)
	for i := range started {
		started[i] = -1
	}
	for frame := 0; ; frame++ {
		for i := 0; i < cards; i++ {
			if started[i] < 0 && g.offsetFor(i) < int(gridRise) {
				started[i] = frame
			}
		}
		if !g.step(cards) {
			break
		}
		if frame > 200 {
			t.Fatal("grid entry never settled")
		}
	}

	for i, at := range started {
		if at < 0 {
			t.Errorf("card %d never started moving", i)
			continue
		}
		if i > 0 && at <= started[i-1] {
			t.Errorf("card %d started at frame %d, not after card %d at %d",
				i, at, i-1, started[i-1])
		}
	}
	// And everything is at rest at the end.
	for i := 0; i < cards; i++ {
		if got := g.offsetFor(i); got != 0 {
			t.Errorf("card %d settled at offset %d", i, got)
		}
	}
}

// TestGridEntryInertWhenSettled — outside an entry every card must be at zero, or
// every grid frame in the app is shifted.
func TestGridEntryInertWhenSettled(t *testing.T) {
	var g gridEntry
	for i := 0; i < 5; i++ {
		if got := g.offsetFor(i); got != 0 {
			t.Errorf("settled entry offsets card %d by %d", i, got)
		}
	}
	if g.step(5) {
		t.Error("a settled entry reported itself as still running")
	}
}

// TestRiseCardKeepsItsCell is the invariant that matters: the cell does not move
// while the card slides inside it. A card that grew or shrank would push the whole
// grid around and break the hit rectangles.
func TestRiseCardKeepsItsCell(t *testing.T) {
	const width, height = 20, 8
	fill := lipgloss.NewStyle().Background(canvasBg())

	var lines []string
	for i := 0; i < height; i++ {
		lines = append(lines, lipgloss.NewStyle().Width(width).Render(fmt.Sprintf("row%d", i)))
	}
	card := strings.Join(lines, "\n")

	for offset := 0; offset <= height+2; offset++ {
		got := riseCard(card, offset, width, height, fill)
		if n := lipgloss.Height(got); n != height {
			t.Errorf("offset %d: card is %d rows, want %d", offset, n, height)
		}
		for i, row := range strings.Split(got, "\n") {
			if w := ansi.StringWidth(row); w != width {
				t.Errorf("offset %d row %d: %d cells, want %d", offset, i, w, width)
			}
		}
	}

	if got := riseCard(card, 0, width, height, fill); got != card {
		t.Error("offset 0 altered the card")
	}
	// A shifted card shows its first row lower down.
	shifted := ansi.Strip(riseCard(card, 2, width, height, fill))
	rows := strings.Split(shifted, "\n")
	if !strings.Contains(rows[2], "row0") {
		t.Errorf("offset 2 did not move row0 to line 2:\n%s", shifted)
	}
	if strings.Contains(rows[0], "row0") {
		t.Error("offset 2 left row0 at the top")
	}
}

// TestTabStartsGridEntry — through the real key path, since that is the only way it
// is ever triggered.
func TestTabStartsGridEntry(t *testing.T) {
	m := gridModel(t, 160, 40, longSessions(6))
	m.viewMode = ListView
	m.animEnabled = true

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	grid := next.(Model)

	if grid.viewMode != GridView {
		t.Fatal("Tab did not switch to grid view")
	}
	if !grid.gridEnter.active {
		t.Error("Tab did not start the staggered entry")
	}
	if cmd == nil {
		t.Error("no command returned; the animation clock never starts")
	}

	// Switching back and forth must not leave it running.
	next, _ = grid.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if back := next.(Model); back.viewMode != ListView {
		t.Error("Tab did not switch back to list view")
	}
}

// TestFrameStaysSizedThroughGridEntry — the rise manipulates card lines directly,
// which is exactly the sort of thing that smears the alt screen if it is off by a
// row or a cell. Every frame of the animation has to hold the frame invariant.
func TestFrameStaysSizedThroughGridEntry(t *testing.T) {
	for _, sz := range [][2]int{{200, 50}, {160, 40}, {120, 30}, {90, 24}} {
		w, h := sz[0], sz[1]
		m := gridModel(t, w, h, longSessions(9))
		m.animEnabled = true
		m.gridEnter.start()

		for frame := 0; ; frame++ {
			content := m.View().Content
			rows := strings.Split(content, "\n")
			if len(rows) > h {
				t.Fatalf("%dx%d frame %d: %d rows, more than %d", w, h, frame, len(rows), h)
			}
			for i, row := range rows {
				if got := ansi.StringWidth(row); got != w {
					t.Fatalf("%dx%d frame %d row %d: %d cells, want %d",
						w, h, frame, i, got, w)
				}
			}
			if !m.gridEnter.step(len(m.filtered)) {
				break
			}
			if frame > 200 {
				t.Fatal("grid entry never settled")
			}
		}
	}
}
