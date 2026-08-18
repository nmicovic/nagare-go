package picker

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
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
