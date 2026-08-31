package theme

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"
)

// lightnessOf reports the HCL lightness of a color, which is what the depth
// ladder is ordered by.
func lightnessOf(t *testing.T, c color.Color) float64 {
	t.Helper()
	cf, ok := toColorful(c)
	if !ok {
		t.Fatalf("color %v is not convertible", c)
	}
	_, _, l := cf.Hcl()
	return l
}

// TestRegisteredThemesAreComplete guards the derivation contract: a theme file
// declares a partial palette, and no renderer should ever see a nil token.
func TestRegisteredThemesAreComplete(t *testing.T) {
	names := Names()
	if len(names) == 0 {
		t.Fatal("no themes registered")
	}
	for _, name := range names {
		th := Get(name)
		c := th.Colors
		tokens := map[string]color.Color{
			"Background": c.Background, "Surface": c.Surface,
			"Overlay": c.Overlay, "Shadow": c.Shadow, "Foreground": c.Foreground,
			"Subtle": c.Subtle, "Muted": c.Muted, "Primary": c.Primary,
			"Secondary": c.Secondary, "Accent": c.Accent,
			"GradientFrom": c.GradientFrom, "GradientTo": c.GradientTo,
			"Border": c.Border, "BorderFocus": c.BorderFocus, "SelBg": c.SelBg,
			"Error": c.Error, "Warning": c.Warning, "Success": c.Success,
		}
		for token, v := range tokens {
			if v == nil {
				t.Errorf("theme %q: token %s is nil", name, token)
			}
		}
	}
}

// TestDepthLadderOrdering is the whole point of the surface tokens: if these
// three do not separate, panels and dialogs render as one flat wall.
func TestDepthLadderOrdering(t *testing.T) {
	for _, name := range Names() {
		// Check the dark half directly rather than whatever the terminal
		// happens to have resolved to, so the test is deterministic.
		c := split(Get(name).Colors, true)
		bg := lightnessOf(t, c.Background)
		surface := lightnessOf(t, c.Surface)
		overlay := lightnessOf(t, c.Overlay)

		if surface <= bg {
			t.Errorf("theme %q: Surface (%.3f) must sit above Background (%.3f)", name, surface, bg)
		}
		if overlay <= surface {
			t.Errorf("theme %q: Overlay (%.3f) must sit above Surface (%.3f)", name, overlay, surface)
		}
	}
}

// TestExplicitTokensSurviveDerivation — derivation is a fallback, not an
// override. A theme that has an opinion keeps it.
func TestExplicitTokensSurviveDerivation(t *testing.T) {
	in := Colors{
		Background: adapt("#000000", "#ffffff"),
		Foreground: adapt("#ffffff", "#000000"),
		Muted:      adapt("#888888", "#777777"),
		Primary:    adapt("#ff0000", "#ff0000"),
		Secondary:  adapt("#00ff00", "#00ff00"),
		Accent:     adapt("#0000ff", "#0000ff"),
		Surface:    adapt("#123456", "#654321"),
	}
	out := normalize(in)

	got := split(out, true).Surface
	want := lipgloss.Color("#123456")
	if got != want {
		t.Errorf("explicit Surface was overwritten: got %v, want %v", got, want)
	}
	// And the unset ones did get filled.
	if split(out, true).Overlay == nil {
		t.Error("Overlay should have been derived")
	}
}

// TestDerivationIsPerMode — a light theme's tokens must come from the light
// palette. Deriving once from a resolved color would leak dark-mode surfaces
// into a light terminal.
func TestDerivationIsPerMode(t *testing.T) {
	out := normalize(Colors{
		Background: adapt("#1a1b26", "#d5d6db"),
		Foreground: adapt("#c0caf5", "#343b58"),
		Muted:      adapt("#565f89", "#9699a3"),
		Primary:    adapt("#7aa2f7", "#34548a"),
		Secondary:  adapt("#bb9af7", "#5a4a78"),
		Accent:     adapt("#7dcfff", "#0f4b6e"),
	})

	darkSurface := lightnessOf(t, split(out, true).Surface)
	lightSurface := lightnessOf(t, split(out, false).Surface)

	if darkSurface > 0.5 {
		t.Errorf("dark-mode Surface is too light (%.3f); it was probably derived from the light palette", darkSurface)
	}
	if lightSurface < 0.5 {
		t.Errorf("light-mode Surface is too dark (%.3f); it was probably derived from the dark palette", lightSurface)
	}
}

// TestLiftPreservesHue is why elevation steps HCL lightness instead of blending
// toward white: a lifted tokyonight panel has to stay blue-grey.
func TestLiftPreservesHue(t *testing.T) {
	base := lipgloss.Color("#1a1b26")
	lifted, ok := toColorful(lift(base, surfaceLift))
	if !ok {
		t.Fatal("lifted color is not convertible")
	}
	orig, _ := toColorful(base)

	oh, _, _ := orig.Hcl()
	lh, _, _ := lifted.Hcl()
	if diff := oh - lh; diff > 1 || diff < -1 {
		t.Errorf("hue drifted on lift: %.2f → %.2f", oh, lh)
	}
}

// TestLiftHandlesGreyscale — a pure grey has no defined hue, and letting NaN
// through produces a black surface instead of a lifted one.
func TestLiftHandlesGreyscale(t *testing.T) {
	for _, in := range []string{"#000000", "#ffffff", "#808080"} {
		out := lift(lipgloss.Color(in), surfaceLift)
		if _, ok := toColorful(out); !ok {
			t.Errorf("lift(%s) produced an unusable color %v", in, out)
		}
		cf, _ := toColorful(out)
		if _, _, l := cf.Hcl(); l != l { // NaN check
			t.Errorf("lift(%s) produced NaN lightness", in)
		}
	}
}

// TestPairFollowsReportedBackground verifies that asynchronous terminal
// detection can switch every adaptive token after the first frame.
func TestPairFollowsReportedBackground(t *testing.T) {
	p := Pair{Dark: lipgloss.Color("#1a1b26"), Light: lipgloss.Color("#d5d6db")}
	resolved := func() string {
		r, g, b, a := p.RGBA()
		if a == 0 {
			t.Fatal("Pair resolved to a transparent color")
		}
		got, _ := colorful.MakeColor(color.RGBA64{R: uint16(r), G: uint16(g), B: uint16(b), A: uint16(a)})
		return got.Hex()
	}

	defer SetDarkBackground(true)
	SetDarkBackground(true)
	if got := resolved(); got != "#1a1b26" {
		t.Fatalf("dark Pair resolved to %s", got)
	}
	SetDarkBackground(false)
	if got := resolved(); got != "#d5d6db" {
		t.Fatalf("light Pair resolved to %s", got)
	}
}
