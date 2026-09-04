package attempts

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAttemptLifecyclePersistsImmutableExecutionProvenance(t *testing.T) {
	store := NewStore(t.TempDir())
	attempt, err := store.Create(CreateInput{
		TicketID:     "ticket-1",
		Agent:        "codex",
		ProjectPath:  filepath.Join(t.TempDir(), "repo"),
		TargetBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempt.State != StateProvisioning {
		t.Fatalf("initial state = %q", attempt.State)
	}

	startedAt := time.Now().UTC()
	updated, err := store.Update(attempt.ID, func(current *Attempt) error {
		current.State = StateRunning
		current.BaseCommit = "abc123"
		current.Branch = "nagare/ticket-attempt"
		current.WorktreePath = filepath.Join(t.TempDir(), "workspace")
		current.SessionName = "repo/ticket-attempt"
		current.PaneID = "%12"
		current.StartedAt = &startedAt
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(updated.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != StateRunning || loaded.BaseCommit != "abc123" || loaded.PaneID != "%12" {
		t.Fatalf("loaded attempt = %#v", loaded)
	}
	listed, err := store.ListForTicket("ticket-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != attempt.ID {
		t.Fatalf("ticket attempts = %#v", listed)
	}
}
