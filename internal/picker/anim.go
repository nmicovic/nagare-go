package picker

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/harmonica"
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
