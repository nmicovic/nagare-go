package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nemke/nagare-go/internal/models"
)

func writeState(t *testing.T, dir string, filename string, s models.SessionState) {
	t.Helper()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadAllStates_Empty(t *testing.T) {
	dir := t.TempDir()
	states := LoadAllStates(dir)
	if len(states) != 0 {
		t.Errorf("expected 0 states, got %d", len(states))
	}
}

func TestLoadAllStates_SingleFile(t *testing.T) {
	dir := t.TempDir()
	writeState(t, dir, "abc.json", models.SessionState{
		State:     "idle",
		SessionID: "abc",
		Cwd:       "/home/user/project",
		Event:     "Stop",
		Timestamp: "2026-03-29T10:00:00Z",
	})

	states := LoadAllStates(dir)
	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}
	s, ok := states["/home/user/project"]
	if !ok {
		t.Fatal("expected state keyed by cwd")
	}
	if s.State != "idle" {
		t.Errorf("state = %q, want %q", s.State, "idle")
	}
}

func TestLoadAllStates_ConflictLiveOverDead(t *testing.T) {
	dir := t.TempDir()
	writeState(t, dir, "dead.json", models.SessionState{
		State:     "dead",
		SessionID: "dead-id",
		Cwd:       "/home/user/project",
		Event:     "SessionEnd",
		Timestamp: "2026-03-29T12:00:00Z",
	})
	writeState(t, dir, "live.json", models.SessionState{
		State:     "working",
		SessionID: "live-id",
		Cwd:       "/home/user/project",
		Event:     "UserPromptSubmit",
		Timestamp: "2026-03-29T10:00:00Z",
	})

	states := LoadAllStates(dir)
	s := states["/home/user/project"]
	if s.State != "working" {
		t.Errorf("live should beat dead: got %q", s.State)
	}
}

func TestLoadAllStates_ConflictNewerWins(t *testing.T) {
	dir := t.TempDir()
	writeState(t, dir, "old.json", models.SessionState{
		State:     "idle",
		SessionID: "old-id",
		Cwd:       "/home/user/project",
		Event:     "Stop",
		Timestamp: "2026-03-29T10:00:00Z",
	})
	writeState(t, dir, "new.json", models.SessionState{
		State:     "working",
		SessionID: "new-id",
		Cwd:       "/home/user/project",
		Event:     "UserPromptSubmit",
		Timestamp: "2026-03-29T12:00:00Z",
	})

	states := LoadAllStates(dir)
	s := states["/home/user/project"]
	if s.State != "working" {
		t.Errorf("newer should win: got %q", s.State)
	}
}

func TestWriteState(t *testing.T) {
	dir := t.TempDir()
	s := models.SessionState{
		State:     "idle",
		SessionID: "test-id",
		Cwd:       "/home/user/project",
		Event:     "Stop",
		Timestamp: "2026-03-29T10:00:00Z",
	}

	err := WriteState(dir, s)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "test-id.json"))
	if err != nil {
		t.Fatal(err)
	}

	var loaded models.SessionState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.State != "idle" {
		t.Errorf("state = %q, want %q", loaded.State, "idle")
	}
}
