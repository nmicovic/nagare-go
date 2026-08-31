package tickets

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCreateListGetAndUpdate(t *testing.T) {
	store := NewStore(t.TempDir())
	low, err := store.Create(CreateInput{
		Title:       "Low priority",
		ProjectPath: "./project",
		Status:      StatusBacklog,
		Priority:    PriorityLow,
	})
	if err != nil {
		t.Fatal(err)
	}
	urgent, err := store.Create(CreateInput{
		Title:      "Urgent outcome",
		Status:     StatusReady,
		Priority:   PriorityUrgent,
		PlannedFor: time.Now().Format(time.DateOnly),
	})
	if err != nil {
		t.Fatal(err)
	}

	all, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ID != urgent.ID || all[1].ID != low.ID {
		t.Fatalf("List() = %#v, want urgent ticket before low ticket", all)
	}
	if !filepath.IsAbs(low.ProjectPath) {
		t.Fatalf("ProjectPath = %q, want absolute path", low.ProjectPath)
	}

	byPrefix, err := store.Get(urgent.ID[:8])
	if err != nil {
		t.Fatal(err)
	}
	if byPrefix.ID != urgent.ID {
		t.Fatalf("Get(prefix).ID = %q, want %q", byPrefix.ID, urgent.ID)
	}

	updated, err := store.Update(urgent.ID, func(ticket *Ticket) error {
		ticket.AssigneeSession = "api"
		ticket.Status = StatusRunning
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != StatusRunning || updated.AssigneeSession != "api" {
		t.Fatalf("Update() = %#v", updated)
	}
}

func TestStoreDeleteByPrefix(t *testing.T) {
	store := NewStore(t.TempDir())
	ticket, err := store.Create(CreateInput{
		Title:    "Remove me",
		Status:   StatusBacklog,
		Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Delete(ticket.ID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ticket.ID); err == nil {
		t.Fatal("Get() found a deleted ticket")
	}
	all, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("List() returned %d tickets after delete", len(all))
	}
}

func TestStoreRejectsInvalidTicketFields(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.Create(CreateInput{Status: StatusReady, Priority: PriorityMedium}); err == nil {
		t.Fatal("Create() accepted an empty title")
	}
	if _, err := store.Create(CreateInput{
		Title:      "Bad date",
		Status:     StatusReady,
		Priority:   PriorityMedium,
		PlannedFor: "tomorrow",
	}); err == nil {
		t.Fatal("Create() accepted a malformed planned date")
	}
}

func TestSetStatusMaintainsCompletionTime(t *testing.T) {
	store := NewStore(t.TempDir())
	ticket, err := store.Create(CreateInput{Title: "Review me", Status: StatusReview, Priority: PriorityMedium})
	if err != nil {
		t.Fatal(err)
	}
	done, err := store.SetStatus(ticket.ID, StatusDone)
	if err != nil {
		t.Fatal(err)
	}
	if done.CompletedAt == nil {
		t.Fatal("SetStatus(done) did not record completion time")
	}
	reopened, err := store.SetStatus(ticket.ID, StatusReview)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.CompletedAt != nil {
		t.Fatal("SetStatus(review) retained completion time")
	}
}
