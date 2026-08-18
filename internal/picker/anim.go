package picker

import (
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"

	"github.com/nemke/nagare-go/internal/models"
)

// Overlay entry animation.
//
// A dialog rises into its resting place instead of appearing instantly. This is
// not decoration for its own sake: an overlay that pops in gives no clue where it
// came from or that it is a separate layer, while one that rises reads as having
// been summoned — the same reason the shadow under it helps.
//
// The clock only runs while something is actually moving. A frame costs 2-5ms to
// assemble at a realistic size (30 sessions, 200x50), so a continuous 30fps clock
// would burn 5-15% of a core for as long as the picker is open. Eight frames of
// settle costs nothing, which is why the status-dot pulse stays on its 1Hz
// toggle rather than being made to breathe smoothly.
const (
	animFPS = 30
	// overlayRise is how far below its resting place a dialog starts, in rows.
	// Six is enough to read as movement; a longer slide reads as sluggish,
	// because a terminal row is a large jump compared to a pixel.
	overlayRise = 6.0
	// animRestVel is the speed below which the spring counts as stopped, once its
	// offset has rounded to zero. Both conditions matter: a terminal cannot show
	// sub-cell movement, so stepping past that point re-renders identical frames
	// — the first tuning of this ran 6 such frames, a fifth of the animation
	// spent invisible.
	animRestVel = 6.0
)

// The breathing clock.
//
// Status dots on running and waiting sessions breathe: their colour walks toward
// the panel behind them and back. A terminal has no opacity, so interpolating the
// colour is what a fade is here.
//
// It is deliberately slow, and that is what makes it affordable. A frame costs a
// few milliseconds to assemble, so a 30fps clock is out of the question — but a
// breath lasting a couple of seconds needs nothing like 30 samples to look smooth,
// because the colour moves so little between frames. At 10fps a 30-session frame
// costs about 3% of a core, and the motion still reads as continuous.
const (
	breathFPS = 10
	// breathPeriod is one full breath. Slow enough to read as alive rather than as
	// a blinking cursor demanding attention.
	breathPeriod = 2200 * time.Millisecond
	// breathDepth is how far the dot fades toward its background at the trough.
	// Past about half it stops reading as the status colour at all.
	breathDepth = 0.5
)

type tickBreathMsg struct{}

func doBreathTick() tea.Cmd {
	return tea.Tick(time.Second/breathFPS, func(time.Time) tea.Msg { return tickBreathMsg{} })
}

// breathStep advances the phase by one frame, wrapping at a full cycle.
func breathStep(phase float64) float64 {
	return math.Mod(phase+float64(time.Second/breathFPS)/float64(breathPeriod), 1)
}

// breathFactor converts a phase into a 0..1 fade amount, easing at both ends so
// the dot settles at its brightest and dimmest rather than snapping through them.
func breathFactor(phase float64) float64 {
	return (1 - math.Cos(2*math.Pi*phase)) / 2
}

// breathes reports whether a status is one that should animate. Idle, dead and
// saved sessions hold still: motion is the signal that something is happening.
func breathes(status models.SessionStatus) bool {
	return status == models.StatusRunning || status == models.StatusWaitingInput
}

// needsBreathing reports whether any visible session is animating, so the clock
// can stop entirely when the whole list is idle.
func needsBreathing(sessions []models.Session) bool {
	for _, s := range sessions {
		if breathes(s.Status) {
			return true
		}
	}
	return false
}

// Selection slide.
//
// The highlight crossfades between the row it left and the row it arrived at,
// rather than teleporting. This is the one animation that fires on ordinary use —
// every press of an arrow key — which is exactly why it is worth having: an
// overlay spring is invisible if you never open an overlay.
//
// It is short. The eye needs only a hint of travel to read the highlight as having
// moved, and anything longer starts to feel like input lag while navigating a list
// quickly.
const (
	// slideDuration is the crossfade length. Four frames at the transient clock's
	// rate.
	slideDuration = 130 * time.Millisecond
)

// selectionSlide crossfades the row tint from one row to another.
type selectionSlide struct {
	// from is the row index the highlight is leaving.
	from int
	// level runs 1 down to 0: the share of the tint still on `from`.
	level float64
	// active distinguishes a settled slide from one that starts on row 0.
	active bool
}

func (s *selectionSlide) start(from int) {
	*s = selectionSlide{from: from, level: 1, active: true}
}

// step advances the crossfade and reports whether it is still running.
func (s *selectionSlide) step() bool {
	if !s.active {
		return false
	}
	step := float64(time.Second/animFPS) / float64(slideDuration)
	if s.level-step <= 0 {
		*s = selectionSlide{}
		return false
	}
	s.level -= step
	return true
}

// tintFor returns how much of the selection tint row i should carry, given where
// the cursor is now. Outside a slide this is the plain 1-or-0 it always was.
func (s selectionSlide) tintFor(i, cursor int) float64 {
	if !s.active {
		if i == cursor {
			return 1
		}
		return 0
	}
	switch i {
	case cursor:
		return 1 - s.level
	case s.from:
		return s.level
	}
	return 0
}

// tickAnimMsg advances any running animation.
type tickAnimMsg struct{}

func doAnimTick() tea.Cmd {
	return tea.Tick(time.Second/animFPS, func(time.Time) tea.Msg { return tickAnimMsg{} })
}

// overlayAnim is a spring settling a vertical offset back to zero.
type overlayAnim struct {
	spring harmonica.Spring
	pos    float64
	vel    float64
	active bool
}

func newOverlayAnim() overlayAnim {
	// Damping just under 1 decelerates into place without a visible bounce: a
	// bouncing dialog reads as a toy, a decelerating one reads as weight. The
	// frequency gives roughly a third of a second of travel — slower feels
	// sluggish when the smallest step is a whole row, faster stops registering as
	// motion at all.
	return overlayAnim{spring: harmonica.NewSpring(harmonica.FPS(animFPS), 16.0, 0.9)}
}

func (a *overlayAnim) start() {
	a.pos, a.vel, a.active = overlayRise, 0, true
}

func (a *overlayAnim) stop() {
	a.pos, a.vel, a.active = 0, 0, false
}

// step advances the spring one frame and reports whether it is still moving.
func (a *overlayAnim) step() bool {
	if !a.active {
		return false
	}
	a.pos, a.vel = a.spring.Update(a.pos, a.vel, 0)
	if int(math.Round(a.pos)) == 0 && math.Abs(a.vel) < animRestVel {
		a.stop()
		return false
	}
	return true
}

// offset is the whole-row displacement to draw the overlay at. Terminals have no
// sub-cell vertical positioning, so the spring's continuous position is rounded;
// the motion is carried by which row it lands on, frame to frame.
func (a overlayAnim) offset() int {
	if !a.active {
		return 0
	}
	return int(math.Round(a.pos))
}

// overlayOpen reports whether any overlay is currently displayed.
func (m Model) overlayOpen() bool {
	return m.showHelp || m.showThemePick || m.promptMode || m.confirmMode
}

// startSlide begins a selection crossfade if the cursor actually moved within an
// unchanged list, and reports whether the clock needs starting.
//
// The list length has to match: narrowing a search renumbers every row, so the
// index the cursor came from no longer refers to the same session and a crossfade
// there would light up something unrelated.
//
// It applies in both views. A list row crossfades its background; a grid card
// cannot, since a gradient border has no partial form, so the card being left
// behind dims from the accent back to quiet chrome instead — a trail rather than a
// crossfade, but the same signal.
func (m *Model) startSlide(prevCursor, prevLen int) bool {
	if !m.animEnabled || m.overlayOpen() {
		return false
	}
	if m.cursor == prevCursor || len(m.filtered) != prevLen {
		return false
	}
	m.slide.start(prevCursor)
	return true
}

// Grid entry.
//
// Switching to grid view drops a wall of cards on the screen at once, which is the
// one moment in the picker where a lot of content appears simultaneously and there
// is no way to tell what arrived. Staggering the cards gives the eye an order to
// follow, and the order it follows is the order they are laid out in.
//
// The stagger is short. It is a hint that the cards arrived in sequence, not a
// waiting animation — with nine cards and three frames apart, the last one starts a
// quarter of a second after the first.
const (
	// gridRise is how far below its cell a card starts, in rows. Smaller than an
	// overlay's: a card is short, and a long slide inside one looks like the content
	// is scrolling rather than the card arriving.
	gridRise = 3.0
	// gridStagger is how many frames apart consecutive cards start.
	gridStagger = 3
)

// gridRiseOffsets is the whole settle sequence of the card spring, precomputed.
//
// One shared table rather than a spring per card: the motion is identical for every
// card and only its start time differs, so a card's offset at any frame is a lookup
// at that frame minus its own delay. That keeps the animation state a single int
// instead of one spring per visible card.
var gridRiseOffsets = func() []int {
	spring := harmonica.NewSpring(harmonica.FPS(animFPS), 16.0, 0.9)
	pos, vel := gridRise, 0.0
	offsets := []int{int(math.Round(pos))}
	for i := 0; i < 4*animFPS; i++ {
		pos, vel = spring.Update(pos, vel, 0)
		offsets = append(offsets, int(math.Round(pos)))
		if int(math.Round(pos)) == 0 && math.Abs(vel) < animRestVel {
			break
		}
	}
	return offsets
}()

// gridEntry tracks how far through the staggered arrival the grid is.
type gridEntry struct {
	frame  int
	active bool
}

func (g *gridEntry) start() { *g = gridEntry{frame: 0, active: true} }
func (g *gridEntry) stop()  { *g = gridEntry{} }

// step advances the stagger and reports whether any of cards cards is still moving.
func (g *gridEntry) step(cards int) bool {
	if !g.active {
		return false
	}
	g.frame++
	if g.frame > (cards-1)*gridStagger+len(gridRiseOffsets) {
		g.stop()
		return false
	}
	return true
}

// offsetFor is how many rows down card i should currently be drawn.
func (g gridEntry) offsetFor(i int) int {
	if !g.active {
		return 0
	}
	at := g.frame - i*gridStagger
	switch {
	case at < 0:
		// Not started yet: hold at the bottom of the rise rather than appearing
		// early, so the stagger is visible as an order rather than a blur.
		return int(gridRise)
	case at >= len(gridRiseOffsets):
		return 0
	default:
		return gridRiseOffsets[at]
	}
}

// riseCard shifts a rendered card down by offset rows inside its own cell, filling
// above it and clipping what falls off the bottom, so the card appears to rise into
// place without the grid's layout moving around it.
func riseCard(card string, offset, width, height int, fill lipgloss.Style) string {
	if offset <= 0 {
		return card
	}
	lines := strings.Split(card, "\n")
	blank := fill.Width(width).Render("")
	shifted := make([]string, 0, height)
	for i := 0; i < offset && len(shifted) < height; i++ {
		shifted = append(shifted, blank)
	}
	for _, l := range lines {
		if len(shifted) >= height {
			break
		}
		shifted = append(shifted, l)
	}
	return strings.Join(shifted, "\n")
}
