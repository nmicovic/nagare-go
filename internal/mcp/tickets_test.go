package mcp

import (
	"strings"
	"testing"

	"github.com/nemke/nagare-go/internal/tickets"
)

func TestSubmitTicketRequiresAssignedAgentAndMovesToReview(t *testing.T) {
	store := tickets.NewStore(t.TempDir())
	ticket, err := store.Create(tickets.CreateInput{
		Title:    "Implement board",
		Status:   tickets.StatusRunning,
		Priority: tickets.PriorityHigh,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(ticket.ID, func(current *tickets.Ticket) error {
		current.AssigneeSession = "nagare-go"
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
}

func TestSubmitTicketRequiresSummary(t *testing.T) {
	store := tickets.NewStore(t.TempDir())
	result := submitTicket(store, "agent", SubmitTicketInput{TicketID: "missing", Summary: "  "})
	if result != "Error: submission summary is required" {
		t.Fatalf("submitTicket() = %q", result)
	}
}
