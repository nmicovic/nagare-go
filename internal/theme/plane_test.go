package theme

import (
	"image/color"
	"strings"
	"testing"
)

func TestOnPlaneReassertsBackgroundAfterStyleReset(t *testing.T) {
	background := color.RGBA{R: 24, G: 28, B: 40, A: 255}
	set := backgroundSequence(background)
	content := "\x1b[38;2;120;130;140mstyled\x1b[0m plain"
	got := OnPlane(content, background)
	if !strings.HasPrefix(got, set) {
		t.Fatalf("plane does not establish its background: %q", got)
	}
	if !strings.Contains(got, "\x1b[0m"+set+" plain") {
		t.Fatalf("plane leaves terminal background visible after reset: %q", got)
	}
}

func TestOnPlanePreservesExplicitNestedSurface(t *testing.T) {
	background := color.RGBA{R: 24, G: 28, B: 40, A: 255}
	foreign := "\x1b[48;2;70;80;90mcard"
	got := OnPlane(foreign, background)
	if !strings.Contains(got, foreign) {
		t.Fatalf("plane overwrote nested surface: %q", got)
	}
}
