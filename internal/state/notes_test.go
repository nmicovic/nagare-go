package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNotesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")

	n := NewNotes(path)
	if err := n.Set("alpha", "waiting on API keys"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := n.Get("alpha"); got != "waiting on API keys" {
		t.Errorf("Get = %q, want %q", got, "waiting on API keys")
	}

	reloaded := NewNotes(path)
	if got := reloaded.Get("alpha"); got != "waiting on API keys" {
		t.Errorf("reloaded Get = %q, want %q", got, "waiting on API keys")
	}
}

func TestNotesEmptyClears(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")
	n := NewNotes(path)
	if err := n.Set("alpha", "scratch"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := n.Set("alpha", "  "); err != nil {
		t.Fatalf("Set empty: %v", err)
	}
	if got := n.Get("alpha"); got != "" {
		t.Errorf("Get after clear = %q, want empty", got)
	}

	reloaded := NewNotes(path)
	if got := reloaded.Get("alpha"); got != "" {
		t.Errorf("reloaded Get after clear = %q, want empty", got)
	}
}

func TestNotesMove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")
	n := NewNotes(path)
	if err := n.Set("old", "keep me"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := n.Move("old", "new"); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if got := n.Get("old"); got != "" {
		t.Errorf("Get(old) after move = %q, want empty", got)
	}
	if got := n.Get("new"); got != "keep me" {
		t.Errorf("Get(new) after move = %q, want %q", got, "keep me")
	}

	reloaded := NewNotes(path)
	if got := reloaded.Get("new"); got != "keep me" {
		t.Errorf("reloaded Get(new) = %q, want %q", got, "keep me")
	}
}

func TestNotesMissingFileIsEmpty(t *testing.T) {
	n := NewNotes(filepath.Join(t.TempDir(), "nope.json"))
	if got := n.Get("anything"); got != "" {
		t.Errorf("Get on missing file = %q, want empty", got)
	}
}

func TestNotesNilSafe(t *testing.T) {
	var n *Notes
	if got := n.Get("x"); got != "" {
		t.Errorf("nil Get = %q, want empty", got)
	}
	if err := n.Set("x", "y"); err != nil {
		t.Errorf("nil Set: %v", err)
	}
	if err := n.Move("x", "z"); err != nil {
		t.Errorf("nil Move: %v", err)
	}
}

func TestNotesTrims(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")
	n := NewNotes(path)
	if err := n.Set("alpha", "  padded  "); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := n.Get("alpha"); got != "padded" {
		t.Errorf("Get = %q, want trimmed", got)
	}
}

func TestNotesUnknownSessionIsEmpty(t *testing.T) {
	n := NewNotes(filepath.Join(t.TempDir(), "notes.json"))
	if got := n.Get("ghost"); got != "" {
		t.Errorf("Get unknown = %q, want empty", got)
	}
}

func TestNotesCorruptFileIsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	n := NewNotes(path)
	if got := n.Get("alpha"); got != "" {
		t.Errorf("Get on corrupt file = %q, want empty", got)
	}
}
