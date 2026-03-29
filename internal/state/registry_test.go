package state

import (
	"path/filepath"
	"testing"
)

func TestRegistryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	reg := NewRegistry(path)

	reg.Register("my-session", "/home/user/project", "claude")

	sessions := reg.ListAll()
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Name != "my-session" {
		t.Errorf("name = %q, want %q", sessions[0].Name, "my-session")
	}
	if sessions[0].Agent != "claude" {
		t.Errorf("agent = %q, want %q", sessions[0].Agent, "claude")
	}

	s := reg.Find("my-session")
	if s == nil {
		t.Fatal("Find returned nil")
	}

	s = reg.FindByPath("/home/user/project")
	if s == nil {
		t.Fatal("FindByPath returned nil")
	}

	reg.ToggleStar("my-session")
	s = reg.Find("my-session")
	if !s.Starred {
		t.Error("session should be starred")
	}

	reg.Remove("my-session")
	if len(reg.ListAll()) != 0 {
		t.Error("session should be removed")
	}
}

func TestRegistryPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	reg1 := NewRegistry(path)
	reg1.Register("test", "/tmp/test", "opencode")

	reg2 := NewRegistry(path)
	sessions := reg2.ListAll()
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session after reload, got %d", len(sessions))
	}
	if sessions[0].Agent != "opencode" {
		t.Errorf("agent = %q, want %q", sessions[0].Agent, "opencode")
	}
}
