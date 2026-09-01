package mcp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nemke/nagare-go/internal/tickets"
)

func TestSubmitTicketRequiresAssignedAgentAndMovesToReview(t *testing.T) {
	store := tickets.NewStore(t.TempDir())
	repo := t.TempDir()
	ticket, err := store.Create(tickets.CreateInput{
		Title:       "Implement board",
		ProjectPath: repo,
		Status:      tickets.StatusRunning,
		Priority:    tickets.PriorityHigh,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(ticket.ID, func(current *tickets.Ticket) error {
		current.AssigneeSession = "nagare-go"
		current.AssigneeAgent = "omp"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	wrongAgent := submitTicket(store, "website", SubmitTicketInput{
		TicketID: ticket.ID[:8],
		Summary:  "Finished",
	})
	if !strings.HasPrefix(wrongAgent, "Error:") {
		t.Fatalf("submitTicket(wrong agent) = %q, want error", wrongAgent)
	}
	unchanged, err := store.Get(ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Status != tickets.StatusRunning {
		t.Fatalf("wrong agent changed status to %q", unchanged.Status)
	}

	beforeSubmit := time.Now().UTC()
	result := submitTicket(store, "nagare-go", SubmitTicketInput{
		TicketID: ticket.ID[:8],
		Summary:  "  Added the ticket board and verified its workflow.  ",
	})
	if strings.HasPrefix(result, "Error:") {
		t.Fatalf("submitTicket() = %q", result)
	}
	submitted, err := store.Get(ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if submitted.Status != tickets.StatusReview {
		t.Fatalf("status = %q, want review", submitted.Status)
	}
	if submitted.SubmittedSummary != "Added the ticket board and verified its workflow." {
		t.Fatalf("summary = %q", submitted.SubmittedSummary)
	}
	if submitted.SubmittedBySession != "nagare-go" {
		t.Fatalf("submitted session = %q", submitted.SubmittedBySession)
	}
	if submitted.SubmittedByAgent != "omp" {
		t.Fatalf("submitted agent = %q", submitted.SubmittedByAgent)
	}
	if submitted.SubmittedRepoPath != repo {
		t.Fatalf("submitted repo = %q, want %q", submitted.SubmittedRepoPath, repo)
	}
	if submitted.SubmittedAt == nil || submitted.SubmittedAt.Before(beforeSubmit) || submitted.SubmittedAt.After(time.Now().UTC()) {
		t.Fatalf("submitted at = %v, want current submission time", submitted.SubmittedAt)
	}
	encoded := ticketJSON(submitted)
	for _, field := range []string{"submitted_summary", "submitted_by_session", "submitted_by_agent", "submitted_repo_path", "submitted_at"} {
		if !strings.Contains(encoded, `"`+field+`"`) {
			t.Errorf("submission JSON missing %q: %s", field, encoded)
		}
	}
}

func TestTodayTicketsIncludesWorkSubmittedToday(t *testing.T) {
	store := tickets.NewStore(t.TempDir())
	submitted, err := store.Create(tickets.CreateInput{
		Title:      "Submitted today",
		Status:     tickets.StatusDone,
		Priority:   tickets.PriorityMedium,
		PlannedFor: time.Now().AddDate(0, 0, -1).Format(time.DateOnly),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(submitted.ID, func(current *tickets.Ticket) error {
		current.RecordSubmission("Finished and verified.", time.Now())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(tickets.CreateInput{
		Title:      "Old unrelated work",
		Status:     tickets.StatusDone,
		Priority:   tickets.PriorityMedium,
		PlannedFor: time.Now().AddDate(0, 0, -2).Format(time.DateOnly),
	}); err != nil {
		t.Fatal(err)
	}

	raw := listTickets(store, ListTicketsInput{Today: true})
	var got []tickets.Ticket
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode listTickets(): %v\n%s", err, raw)
	}
	if len(got) != 1 || got[0].ID != submitted.ID {
		t.Fatalf("today tickets = %#v, want submitted ticket", got)
	}
}

func TestSubmitTicketRequiresSummary(t *testing.T) {
	store := tickets.NewStore(t.TempDir())
	result := submitTicket(store, "agent", SubmitTicketInput{TicketID: "missing", Summary: "  "})
	if result != "Error: submission summary is required" {
		t.Fatalf("submitTicket() = %q", result)
	}
}
