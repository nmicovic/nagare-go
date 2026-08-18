package picker

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestOverlayAnimSettles is the property that matters most: a spring that never
// reaches its rest condition would tick forever, re-rendering the whole frame at
// 30fps for the life of the picker.
func TestOverlayAnimSettles(t *testing.T) {
	a := newOverlayAnim()
	a.start()

	frames := 0
	for a.step() {
		frames++
		if frames > 5*animFPS {
			t.Fatalf("spring still moving after %d frames (%ds); it must settle", frames, 5)
		}
	}

	if a.active {
		t.Error("spring stopped stepping but is still marked active")
	}
	if a.offset() != 0 {
		t.Errorf("settled offset = %d, want 0", a.offset())
	}
	// Long enough to be seen, short enough not to be in the way.
	if frames < 4 {
		t.Errorf("settled in %d frames, too fast to read as motion", frames)
	}
	if frames > animFPS {
		t.Errorf("took %d frames (over a second) to settle", frames)
	}
}

// TestOverlayAnimMovesTowardRest — the offset must shrink monotonically enough to
// read as a rise rather than a wobble. Damping is set just under 1 for exactly
// this, so any regression that introduces bouncing should fail here.
func TestOverlayAnimMovesTowardRest(t *testing.T) {
	a := newOverlayAnim()
	a.start()

	if got := a.offset(); got != int(overlayRise) {
		t.Fatalf("initial offset = %d, want %v", got, overlayRise)
	}

	prev := a.pos
	for a.step() {
		if a.pos > prev+0.01 {
			t.Fatalf("offset grew from %.2f to %.2f; the dialog bounced", prev, a.pos)
		}
		if a.pos < -0.5 {
			t.Fatalf("offset overshot to %.2f; the dialog rose past its resting place", a.pos)
		}
		prev = a.pos
	}
}

// TestOverlayAnimStartsOnOpen — catching the transition in Update is what keeps
// every overlay animated without each open site remembering to ask.
func TestOverlayAnimStartsOnOpen(t *testing.T) {
	m := mouseModel(t, 120, 30)
	m.animEnabled = true

	if m.overlayOpen() {
		t.Fatal("an overlay is open before the test began")
	}

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	opened := next.(Model)

	if !opened.showHelp {
		t.Fatal("F1 did not open the help overlay")
	}
	if !opened.overlayAnim.active {
		t.Error("overlay opened without starting the entry animation")
	}
	if opened.overlayAnim.offset() == 0 {
		t.Error("animation is active but the overlay is already at rest")
	}
	if cmd == nil {
		t.Error("no command returned; the animation clock never starts")
	}
}

// TestOverlayAnimSkippedWhenDisabled — the config switch has to reach the frame,
// not just the model.
func TestOverlayAnimSkippedWhenDisabled(t *testing.T) {
	m := mouseModel(t, 120, 30)
	m.animEnabled = false

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	opened := next.(Model)

	if !opened.showHelp {
		t.Fatal("F1 did not open the help overlay")
	}
	if opened.overlayAnim.active {
		t.Error("animations are disabled but the spring started anyway")
	}
	if got := opened.overlayAnim.offset(); got != 0 {
		t.Errorf("offset with animations disabled = %d, want 0", got)
	}
}

// TestOverlayAnimStopsOnClose — leaving a spring running after its dialog closed
// would keep the clock alive with nothing to move.
func TestOverlayAnimStopsOnClose(t *testing.T) {
	m := mouseModel(t, 120, 30)
	m.animEnabled = true

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	m = next.(Model)
	if !m.overlayAnim.active {
		t.Fatal("expected an active animation to stop")
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyF1}) // toggles help back off
	closed := next.(Model)

	if closed.overlayOpen() {
		t.Fatal("help overlay did not close")
	}
	if closed.overlayAnim.active {
		t.Error("animation still running after the overlay closed")
	}
}

// TestAnimTickStopsWhenSettled — the clock must not reschedule itself forever.
func TestAnimTickStopsWhenSettled(t *testing.T) {
	m := mouseModel(t, 120, 30)
	m.animEnabled = true
	m.showHelp = true
	m.overlayAnim.start()

	var cmd tea.Cmd
	for i := 0; i < 5*animFPS; i++ {
		var next tea.Model
		next, cmd = m.Update(tickAnimMsg{})
		m = next.(Model)
		if cmd == nil {
			break
		}
	}
	if cmd != nil {
		t.Fatal("animation clock never stopped rescheduling")
	}
	if m.overlayAnim.active {
		t.Error("clock stopped while the animation was still active")
	}
}

// TestAnimatedOverlayStaysInFrame — the rise starts the dialog lower than its
// resting place, and a frame one row too tall smears the alt screen.
func TestAnimatedOverlayStaysInFrame(t *testing.T) {
	m := mouseModel(t, 120, 30)
	m.animEnabled = true
	m.showThemePick = true
	m.themeNames = []string{"tokyonight", "dracula", "nord"}
	m.overlayAnim.start()

	for step := 0; ; step++ {
		frame, hits := m.view()
		lines, width := frameSize(frame)
		if lines != 30 || width != 120 {
			t.Fatalf("step %d (offset %d): frame is %dx%d, want 120x30",
				step, m.overlayAnim.offset(), width, lines)
		}
		if hits.dialog.Min.Y < 0 {
			t.Errorf("step %d: dialog bounds start above the frame at y=%d", step, hits.dialog.Min.Y)
		}
		if !m.overlayAnim.step() {
			break
		}
		if step > 5*animFPS {
			t.Fatal("animation did not finish")
		}
	}
}

// TestAnimationOffsetMovesTheDialog — the offset has to actually change where the
// dialog is drawn, not merely be tracked in the model.
func TestAnimationOffsetMovesTheDialog(t *testing.T) {
	m := mouseModel(t, 120, 30)
	m.showHelp = true

	_, resting := m.view()

	m.animEnabled = true
	m.overlayAnim.start()
	_, rising := m.view()

	if rising.dialog.Min.Y <= resting.dialog.Min.Y {
		t.Errorf("rising dialog starts at y=%d, resting at y=%d; it should start lower",
			rising.dialog.Min.Y, resting.dialog.Min.Y)
	}
}
