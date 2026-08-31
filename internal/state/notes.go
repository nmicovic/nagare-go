package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nemke/nagare-go/internal/fsutil"
)

// Notes is a persistent map of session display name → reminder text.
//
// Kept out of sessions.json because Python nagare constructs
// RegisteredSession(**row) and treats unknown fields as a TypeError that
// empties the whole registry.
type Notes struct {
	path   string
	byName map[string]string
}

// DefaultNotesPath returns the default notes file path.
func DefaultNotesPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "nagare", "notes.json")
}

// NewNotes loads or creates a notes store at the given path.
func NewNotes(path string) *Notes {
	n := &Notes{path: path, byName: map[string]string{}}
	n.load()
	return n
}

func (n *Notes) load() {
	data, err := os.ReadFile(n.path)
	if err != nil {
		return
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil || m == nil {
		return
	}
	n.byName = m
}

func (n *Notes) save() error {
	if n.byName == nil {
		n.byName = map[string]string{}
	}
	data, err := json.MarshalIndent(n.byName, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(n.path), 0755); err != nil {
		return fmt.Errorf("notes dir: %w", err)
	}
	if err := fsutil.AtomicWrite(n.path, data, 0644); err != nil {
		return fmt.Errorf("write notes: %w", err)
	}
	return nil
}

// Get returns the note for name, or empty if none is stored.
func (n *Notes) Get(name string) string {
	if n == nil || n.byName == nil {
		return ""
	}
	return n.byName[name]
}

// Set stores text as the note for name. Empty or whitespace-only text
// deletes the note.
func (n *Notes) Set(name, text string) error {
	if n == nil {
		return nil
	}
	if n.byName == nil {
		n.byName = map[string]string{}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		delete(n.byName, name)
	} else {
		n.byName[name] = text
	}
	return n.save()
}

// Move rekeys a note from oldName to newName. A missing note is a no-op.
func (n *Notes) Move(oldName, newName string) error {
	if n == nil || oldName == newName {
		return nil
	}
	text := n.Get(oldName)
	if text == "" {
		return nil
	}
	if n.byName == nil {
		n.byName = map[string]string{}
	}
	delete(n.byName, oldName)
	n.byName[newName] = text
	return n.save()
}
