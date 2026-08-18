package picker

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/nemke/nagare-go/internal/theme"
)

// TestSlideIsInertWhenSettled — outside a crossfade the tint has to be exactly the
// full-or-nothing it always was, or every frame in the app changes.
func TestSlideIsInertWhenSettled(t *testing.T) {
	var s selectionSlide
	for i := 0; i < 5; i++ {
		want := 0.0
		if i == 2 {
			want = 1
		}
		if got := s.tintFor(i, 2); got != want {
			t.Errorf("settled tintFor(%d, cursor 2) = %v, want %v", i, got, want)
		}
	}
	if s.step() {
		t.Error("a settled slide reported itself as still running")
	}
}

// TestSlideCrossfadeConserves — the two rows' tints must add to a whole at every
// step, or the highlight visibly dims or doubles mid-move.
func TestSlideCrossfadeConserves(t *testing.T) {
	var s selectionSlide
	s.start(3)
	frames := 0
	for {
		from, to := s.tintFor(3, 4), s.tintFor(4, 4)
		if sum := from + to; math.Abs(sum-1) > 1e-9 {
			t.Errorf("frame %d: tints sum to %v, want 1", frames, sum)
		}
		if from < 0 || from > 1 || to < 0 || to > 1 {
			t.Errorf("frame %d: tints out of range (%v, %v)", frames, from, to)
		}
		// Untouched rows stay clear.
		if got := s.tintFor(0, 4); got != 0 {
			t.Errorf("frame %d: an unrelated row carries %v of the tint", frames, got)
		}
		frames++
		if !s.step() {
			break
		}
		if frames > 100 {
			t.Fatal("slide never finished")
		}
	}
	want := int(math.Round(float64(slideDuration) / (1e9 / animFPS)))
	if frames < want-1 || frames > want+1 {
		t.Errorf("crossfade took %d frames, want about %d", frames, want)
	}
	// And it ends fully on the destination.
	if got := s.tintFor(4, 4); got != 1 {
		t.Errorf("after settling the cursor row carries %v, want 1", got)
	}
}

// TestSlideMovesToward — the destination has to gain as the origin loses, monotone,
// so the motion has a direction.
func TestSlideMovesToward(t *testing.T) {
	var s selectionSlide
	s.start(1)
	prevTo := s.tintFor(2, 2)
	for s.step() {
		to := s.tintFor(2, 2)
		if to < prevTo {
			t.Fatalf("destination tint went backwards: %v then %v", prevTo, to)
		}
		prevTo = to
	}
}

// TestStartSlideGuards covers every case where a crossfade would be wrong rather
// than merely absent.
func TestStartSlideGuards(t *testing.T) {
	base := func(t *testing.T) Model {
		return driveModel(t, NewForTest(),
			tea.WindowSizeMsg{Width: 120, Height: 30},
			SessionsUpdatedMsg(waitingSet("iiwii")),
		)
	}

	t.Run("cursor moved", func(t *testing.T) {
		m := base(t)
		if !m.startSlide(m.cursor+1, len(m.filtered)) {
			t.Error("a real cursor move did not start a slide")
		}
	})
	t.Run("cursor did not move", func(t *testing.T) {
		m := base(t)
		if m.startSlide(m.cursor, len(m.filtered)) {
			t.Error("slide started without the cursor moving")
		}
	})
	t.Run("list was refiltered", func(t *testing.T) {
		m := base(t)
		// Narrowing a search renumbers rows, so the previous index no longer refers
		// to the same session and a crossfade would light up something unrelated.
		if m.startSlide(m.cursor+1, len(m.filtered)+3) {
			t.Error("slide started across a list length change")
		}
	})
	t.Run("grid view leaves a trail", func(t *testing.T) {
		m := base(t)
		m.viewMode = GridView
		if !m.startSlide(m.cursor+1, len(m.filtered)) {
			t.Error("grid view should slide too, as a dimming border trail")
		}
	})
	t.Run("overlay open", func(t *testing.T) {
		m := base(t)
		m.showHelp = true
		if m.startSlide(m.cursor+1, len(m.filtered)) {
			t.Error("slide started while an overlay had the screen")
		}
	})
	t.Run("animations disabled", func(t *testing.T) {
		m := base(t)
		m.animEnabled = false
		if m.startSlide(m.cursor+1, len(m.filtered)) {
			t.Error("slide started with animations turned off")
		}
	})
}

// TestArrowKeyStartsSlide — the whole point is that this fires on ordinary use, so
// check it through the real key path rather than only the helper.
func TestArrowKeyStartsSlide(t *testing.T) {
	m := driveModel(t, NewForTest(),
		tea.WindowSizeMsg{Width: 120, Height: 30},
		SessionsUpdatedMsg(waitingSet("iiwii")),
	)
	m.animEnabled = true
	before := m.cursor

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	moved := next.(Model)

	if moved.cursor == before {
		t.Fatal("down arrow did not move the cursor")
	}
	if !moved.slide.active {
		t.Error("down arrow did not start a selection slide")
	}
	if moved.slide.from != before {
		t.Errorf("slide starts from row %d, want %d", moved.slide.from, before)
	}
	if cmd == nil {
		t.Error("no command returned; the animation clock never starts")
	}
}

// TestSlideShowsTwoTintedRows — mid-crossfade both rows must carry part of the
// tint in the actual frame, which is what reads as movement.
func TestSlideShowsTwoTintedRows(t *testing.T) {
	m := driveModel(t, NewForTest(),
		tea.WindowSizeMsg{Width: 120, Height: 30},
		SessionsUpdatedMsg(waitingSet("iiiii")),
	)
	m.animEnabled = true

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)
	// Advance to the middle of the fade.
	next, _ = m.Update(tickAnimMsg{})
	m = next.(Model)

	if !m.slide.active {
		t.Fatal("slide finished before the frame under test")
	}

	frame, _ := m.view()
	c := theme.Current().Colors

	// Compute the exact tints from the live slide rather than approximating them:
	// the per-frame step is 1/3.9, so rounded guesses blend to different colours
	// and match nothing.
	fromTint := m.slide.tintFor(m.slide.from, m.cursor)
	toTint := m.slide.tintFor(m.cursor, m.cursor)
	if fromTint <= 0 || fromTint >= 1 || toTint <= 0 || toTint >= 1 {
		t.Fatalf("not mid-fade: from %v, to %v", fromTint, toTint)
	}

	for label, tint := range map[string]float64{"origin": fromTint, "destination": toTint} {
		want := bgSGR(theme.Mix(c.Surface, c.SelBg, tint))
		if !strings.Contains(frame, want) {
			t.Errorf("%s row is not partially tinted in the frame (looked for %s)", label, want)
		}
	}
}

// TestGridSlideLeavesADimmingTrail — a gradient border has no partial form, so the
// grid's version of the crossfade is the outgoing card dimming from the accent back
// to quiet chrome. Check it actually reaches the frame, and that it fades.
func TestGridSlideLeavesADimmingTrail(t *testing.T) {
	m := gridModel(t, 200, 40, longSessions(6))
	m.animEnabled = true
	m.cursor = 0

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = next.(Model)
	if m.cursor == 0 {
		t.Fatal("right arrow did not move the grid cursor")
	}
	if !m.slide.active {
		t.Fatal("moving in grid view did not start a slide")
	}

	c := theme.Current().Colors
	seen := 0
	prev := 2.0
	for {
		trail := m.slide.tintFor(m.slide.from, m.cursor)
		if trail <= 0 {
			break
		}
		if trail >= prev {
			t.Fatalf("trail brightened: %v then %v", prev, trail)
		}
		prev = trail

		// Only count intermediate values. At trail 1 the blend *is* GradientFrom,
		// which the selected card's gradient border already contains, so matching
		// there passes whether or not the trail exists — the first version of this
		// test did exactly that and survived the feature being deleted.
		if trail < 0.95 && trail > 0.05 {
			frame, _ := m.view()
			// The border's colour is emitted together with its background in one SGR,
			// so match the parameter fragment rather than a whole sequence.
			if strings.Contains(frame, fgParams(theme.Mix(c.Border, c.GradientFrom, trail))) {
				seen++
			}
		}
		if !m.slide.step() {
			break
		}
	}
	if seen == 0 {
		t.Error("the outgoing card's border never carried a partially dimmed accent")
	}

	// Once settled, no card carries a trail colour.
	m.slide = selectionSlide{}
	settled, _ := m.view()
	for _, level := range []float64{0.25, 0.5, 0.75} {
		if strings.Contains(settled, fgParams(theme.Mix(c.Border, c.GradientFrom, level))) {
			t.Errorf("a trail colour (%.2f) survives after the slide settled", level)
		}
	}
}

// fgParams is the foreground SGR parameter fragment for a colour, without the CSI
// or the trailing "m", for matching colours that share a sequence with others.
func fgParams(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
}
