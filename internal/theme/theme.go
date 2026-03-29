package theme

import (
	"fmt"
	"sort"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// Colors holds all semantic colors for a theme.
type Colors struct {
	Background lipgloss.AdaptiveColor
	Foreground lipgloss.AdaptiveColor
	Primary    lipgloss.AdaptiveColor
	Secondary  lipgloss.AdaptiveColor
	Accent     lipgloss.AdaptiveColor
	Muted      lipgloss.AdaptiveColor
	Border     lipgloss.AdaptiveColor
	Error      lipgloss.AdaptiveColor
	Warning    lipgloss.AdaptiveColor
	Success    lipgloss.AdaptiveColor
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

// CycleNext switches to the next theme in alphabetical order.
func CycleNext() string {
	names := Names()
	cur := Current().Name
	for i, name := range names {
		if name == cur {
			next := names[(i+1)%len(names)]
			Set(next)
			return next
		}
	}
	if len(names) > 0 {
		Set(names[0])
		return names[0]
	}
	return cur
}
