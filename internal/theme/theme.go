package theme

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"sync"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
	"github.com/lucasb-eyer/go-colorful"
)

// Colors is nagare's design-token layer: every color is named for the *role*
// it plays, never for what it looks like. Nothing outside this package should
// reach for a raw hex value.
//
// The tokens fall into four groups.
//
// Surfaces are the depth ladder, and the order matters — Surface floats above
// the base, Overlay floats above that. Reading a nagare frame should feel like
// looking at three planes, not one flat wall.
//
// Captured pane output deliberately stays on Background rather than getting a
// plane of its own. It is verbatim foreign content: the agent drew it against
// the terminal's background, and its own explicit fills only read correctly
// over that same ground. A preview is a window through the UI to the terminal
// underneath, not a well cut into it.
//
// Text is an emphasis ladder: Foreground for what you came to read, Subtle for
// supporting values, Muted for labels and hints. Three tiers is the minimum
// that lets a dense panel establish hierarchy without resorting to color.
//
// Accents carry identity and focus. Borders separate "this is chrome" from
// "this is where your keystrokes land".
//
// Status colors must survive being the only signal: strip every other color
// from the UI and the four session states still have to be tellable apart.
type Colors struct {
	// Surfaces — the depth ladder, back to front.
	Background color.Color // the canvas, and the plane streamed pane output sits on
	Surface    color.Color // panels lifted off the canvas
	Overlay    color.Color // dialogs floating above panels
	Shadow     color.Color // the shadow an overlay casts

	// Text — the emphasis ladder, loudest first.
	Foreground color.Color // primary content
	Subtle     color.Color // secondary values
	Muted      color.Color // labels, hints, disabled

	// Accents.
	Primary   color.Color
	Secondary color.Color
	Accent    color.Color

	// Gradient stops, for blended borders and rules. Default to Primary →
	// Secondary so every theme gets a coherent gradient without declaring one.
	GradientFrom color.Color
	GradientTo   color.Color

	// Borders.
	Border      color.Color // quiet chrome
	BorderFocus color.Color // the panel that has focus

	SelBg color.Color // background for the selected row/cell

	// Status.
	Error   color.Color
	Warning color.Color
	Success color.Color
}

// Pair is a dark/light color pair.
//
// It implements color.Color by resolving against the terminal background that
// lipgloss detected at startup, exactly like compat.AdaptiveColor. Unlike
// AdaptiveColor it keeps both halves reachable, which is what lets derived
// tokens be computed *per mode*: elevating a surface means something different
// on a #1a1b26 canvas than on a #d5d6db one, and a single resolved color could
// only ever get one of them right.
type Pair struct {
	Dark  color.Color
	Light color.Color
}

// RGBA implements color.Color.
func (p Pair) RGBA() (r, g, b, a uint32) {
	return compat.AdaptiveColor{Dark: p.Dark, Light: p.Light}.RGBA()
}

// adapt builds a dark/light-aware color from two hex strings. Themes use this
// helper to keep palette declarations terse.
func adapt(dark, light string) color.Color {
	return Pair{
		Dark:  lipgloss.Color(dark),
		Light: lipgloss.Color(light),
	}
}

// Theme is a named color palette.
type Theme struct {
	Name   string
	Colors Colors
}

var (
	mu      sync.RWMutex
	current *Theme
	all     = map[string]*Theme{}
)

// Register adds a theme, filling in any token it left unset. Called from
// init() in the theme files.
func Register(name string, t *Theme) {
	mu.Lock()
	defer mu.Unlock()
	t.Colors = normalize(t.Colors)
	all[name] = t
}

// Set switches the active theme by name.
func Set(name string) error {
	mu.Lock()
	defer mu.Unlock()
	t, ok := all[name]
	if !ok {
		return fmt.Errorf("unknown theme: %s", name)
	}
	current = t
	return nil
}

// Current returns the active theme. Falls back to tokyonight.
func Current() *Theme {
	mu.RLock()
	defer mu.RUnlock()
	if current == nil {
		if t, ok := all["tokyonight"]; ok {
			return t
		}
		// Return first available
		for _, t := range all {
			return t
		}
	}
	return current
}

// Names returns sorted theme names.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Get returns a theme by name, or nil if not found.
func Get(name string) *Theme {
	mu.RLock()
	defer mu.RUnlock()
	return all[name]
}

// --- Token derivation ---
//
// A theme declares the palette it actually has a considered opinion about —
// background, foreground, accents, status — and the depth and emphasis tiers
// are derived from it. Deriving rather than hand-picking is deliberate: it
// guarantees all 13 themes get the *same* sense of depth, and it means a new
// theme is still four lines of hex rather than fourteen. Any theme that wants
// a specific value just sets the field and derivation leaves it alone.

// Elevation is expressed as a step in HCL lightness, which moves a color
// toward "lifted" while leaving its hue and chroma intact — the reason a
// lifted tokyonight panel still reads as blue-grey instead of drifting toward
// grey the way blending toward white would.
const (
	surfaceLift = 0.055 // panels, off the canvas
	overlayLift = 0.105 // dialogs, off the panels
	shadowSink  = 0.450 // toward black, under an overlay
	subtleMix   = 0.450 // Foreground → Muted
)

// normalize fills every unset token in c, deriving each dark/light half from
// that half's own palette.
func normalize(c Colors) Colors {
	dark := derive(split(c, true))
	light := derive(split(c, false))
	return merge(dark, light)
}

// split extracts one mode's half of every token. A theme is free to declare a
// plain (non-Pair) color, in which case both halves are that color.
func split(c Colors, dark bool) Colors {
	h := func(v color.Color) color.Color { return half(v, dark) }
	return Colors{
		Background:   h(c.Background),
		Surface:      h(c.Surface),
		Overlay:      h(c.Overlay),
		Shadow:       h(c.Shadow),
		Foreground:   h(c.Foreground),
		Subtle:       h(c.Subtle),
		Muted:        h(c.Muted),
		Primary:      h(c.Primary),
		Secondary:    h(c.Secondary),
		Accent:       h(c.Accent),
		GradientFrom: h(c.GradientFrom),
		GradientTo:   h(c.GradientTo),
		Border:       h(c.Border),
		BorderFocus:  h(c.BorderFocus),
		SelBg:        h(c.SelBg),
		Error:        h(c.Error),
		Warning:      h(c.Warning),
		Success:      h(c.Success),
	}
}

func half(v color.Color, dark bool) color.Color {
	p, ok := v.(Pair)
	if !ok {
		return v
	}
	if dark {
		return p.Dark
	}
	return p.Light
}

// derive fills the unset tokens of a single-mode palette.
func derive(c Colors) Colors {
	if c.Surface == nil {
		c.Surface = lift(c.Background, surfaceLift)
	}
	if c.Overlay == nil {
		c.Overlay = lift(c.Background, overlayLift)
	}
	if c.Shadow == nil {
		c.Shadow = mix(c.Background, colorful.Color{R: 0, G: 0, B: 0}, shadowSink)
	}
	if c.Subtle == nil {
		c.Subtle = mix(c.Foreground, c.Muted, subtleMix)
	}
	if c.BorderFocus == nil {
		c.BorderFocus = c.Accent
	}
	if c.GradientFrom == nil {
		c.GradientFrom = c.Primary
	}
	if c.GradientTo == nil {
		c.GradientTo = c.Secondary
	}
	return c
}

// merge recombines two single-mode palettes into adaptive Pairs.
func merge(dark, light Colors) Colors {
	p := func(d, l color.Color) color.Color {
		if d == nil && l == nil {
			return nil
		}
		return Pair{Dark: d, Light: l}
	}
	return Colors{
		Background:   p(dark.Background, light.Background),
		Surface:      p(dark.Surface, light.Surface),
		Overlay:      p(dark.Overlay, light.Overlay),
		Shadow:       p(dark.Shadow, light.Shadow),
		Foreground:   p(dark.Foreground, light.Foreground),
		Subtle:       p(dark.Subtle, light.Subtle),
		Muted:        p(dark.Muted, light.Muted),
		Primary:      p(dark.Primary, light.Primary),
		Secondary:    p(dark.Secondary, light.Secondary),
		Accent:       p(dark.Accent, light.Accent),
		GradientFrom: p(dark.GradientFrom, light.GradientFrom),
		GradientTo:   p(dark.GradientTo, light.GradientTo),
		Border:       p(dark.Border, light.Border),
		BorderFocus:  p(dark.BorderFocus, light.BorderFocus),
		SelBg:        p(dark.SelBg, light.SelBg),
		Error:        p(dark.Error, light.Error),
		Warning:      p(dark.Warning, light.Warning),
		Success:      p(dark.Success, light.Success),
	}
}

// lift returns c with its HCL lightness moved by dl, clamped to the display
// gamut. A negative dl sinks it.
func lift(c color.Color, dl float64) color.Color {
	src, ok := toColorful(c)
	if !ok {
		return c
	}
	h, chroma, l := src.Hcl()
	// A fully desaturated color has no defined hue, and NaN would poison the
	// conversion back.
	if math.IsNaN(h) {
		h = 0
	}
	l = clamp01(l + dl)
	return hex(colorful.Hcl(h, chroma, l).Clamped())
}

// mix blends a toward b by t (0 = a, 1 = b) in Lab, so the midpoint looks like
// a midpoint rather than passing through mud.
func mix(a, b color.Color, t float64) color.Color {
	ca, ok := toColorful(a)
	if !ok {
		return a
	}
	cb, ok := toColorful(b)
	if !ok {
		return a
	}
	return hex(ca.BlendLab(cb, clamp01(t)).Clamped())
}

// toColorful converts any color.Color into a colorful.Color. It reports false
// for a nil or fully transparent color, which has no meaningful hue to work
// from.
func toColorful(c color.Color) (colorful.Color, bool) {
	if c == nil {
		return colorful.Color{}, false
	}
	_, _, _, a := c.RGBA()
	if a == 0 {
		return colorful.Color{}, false
	}
	out, ok := colorful.MakeColor(c)
	return out, ok
}

// hex freezes a computed color into a lipgloss.Color. Derived tokens are
// resolved per mode before this point, so there is nothing adaptive left to
// preserve.
func hex(c colorful.Color) color.Color {
	return lipgloss.Color(c.Hex())
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}
