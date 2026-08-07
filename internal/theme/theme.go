package theme

import (
	"fmt"
	"image/color"
	"sort"
	"sync"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

// Colors holds all semantic colors for a theme.
type Colors struct {
	Background color.Color
	Foreground color.Color
	Primary    color.Color
	Secondary  color.Color
	Accent     color.Color
	Muted      color.Color
	Border     color.Color
	SelBg      color.Color // background for the selected row/cell; subtly elevated from Background
	Error      color.Color
	Warning    color.Color
	Success    color.Color
}

// adapt builds a light/dark-aware color from two hex strings. Themes use this
// helper to keep palette declarations terse.
func adapt(dark, light string) color.Color {
	return compat.AdaptiveColor{
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

// Register adds a theme. Called from init() in theme files.
func Register(name string, t *Theme) {
	mu.Lock()
	defer mu.Unlock()
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
