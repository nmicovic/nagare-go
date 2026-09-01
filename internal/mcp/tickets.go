package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nemke/nagare-go/internal/tickets"
)

// ListTicketsInput filters tickets visible to an agent.
type ListTicketsInput struct {
	Status      string `json:"status,omitempty" jsonschema:"optional workflow status: backlog, ready, running, review, done, canceled"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"optional exact project path"`
	Today       bool   `json:"today,omitempty" jsonschema:"tickets planned, submitted, or completed today plus active work"`
}

// GetTicketInput identifies a ticket by full ID or unique prefix.
type GetTicketInput struct {
	TicketID string `json:"ticket_id" jsonschema:"ticket ID or unique prefix"`
}

// SubmitTicketInput is an agent's handoff to human review.
type SubmitTicketInput struct {
	TicketID string `json:"ticket_id" jsonschema:"assigned ticket ID or unique prefix"`
	Summary  string `json:"summary" jsonschema:"required description of what changed and how the result was verified"`
}

// ListTicketsHandler returns matching tickets as JSON.
func ListTicketsHandler(input ListTicketsInput) string {
	return listTickets(tickets.NewStore(tickets.DefaultDir()), input)
}

func listTickets(store *tickets.Store, input ListTicketsInput) string {
	all, err := store.List()
	if err != nil {
		return "Error: " + err.Error()
	}
	filtered := make([]tickets.Ticket, 0, len(all))
	today := time.Now().Format(time.DateOnly)
	for _, ticket := range all {
		if input.Status != "" && string(ticket.Status) != input.Status {
			continue
		}
		if input.ProjectPath != "" && ticket.ProjectPath != input.ProjectPath {
			continue
		}
		if input.Today && !ticketRelevantOn(ticket, today) {
			continue
		}
		filtered = append(filtered, ticket)
	}
	return ticketJSON(filtered)
}

func ticketRelevantOn(ticket tickets.Ticket, date string) bool {
	if ticket.PlannedFor == date || ticket.Status == tickets.StatusRunning || ticket.Status == tickets.StatusReview {
		return true
	}
	return occurredOn(ticket.SubmittedAt, date) || occurredOn(ticket.CompletedAt, date)
}

func occurredOn(at *time.Time, date string) bool {
	return at != nil && at.Local().Format(time.DateOnly) == date
}

// GetTicketHandler returns one ticket as JSON.
func GetTicketHandler(input GetTicketInput) string {
	store := tickets.NewStore(tickets.DefaultDir())
	ticket, err := store.Get(input.TicketID)
	if err != nil {
		return "Error: " + err.Error()
	}
	return ticketJSON(ticket)
}

// SubmitTicketHandler hands an assigned running ticket back for human review.
func SubmitTicketHandler(mySession string, input SubmitTicketInput) string {
	return submitTicket(tickets.NewStore(tickets.DefaultDir()), mySession, input)
}

func submitTicket(store *tickets.Store, mySession string, input SubmitTicketInput) string {
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		return "Error: submission summary is required"
	}
	ticket, err := store.Get(input.TicketID)
	if err != nil {
		return "Error: " + err.Error()
	}
	if ticket.Status != tickets.StatusRunning {
		return fmt.Sprintf("Error: ticket %s is %s, not running", ticket.ID, ticket.Status)
	}
	if ticket.AssigneeSession == "" || ticket.AssigneeSession != mySession {
		return fmt.Sprintf("Error: ticket %s is assigned to %q, not %q", ticket.ID, ticket.AssigneeSession, mySession)
	}
	updated, err := store.Update(ticket.ID, func(current *tickets.Ticket) error {
		if current.Status != tickets.StatusRunning {
			return fmt.Errorf("ticket is %s, not running", current.Status)
		}
		if current.AssigneeSession != mySession {
			return fmt.Errorf("ticket is assigned to %q", current.AssigneeSession)
		}
		current.Status = tickets.StatusReview
		current.RecordSubmission(summary, time.Now())
		return nil
	})
	if err != nil {
		return "Error: " + err.Error()
	}
	return fmt.Sprintf("Ticket %s moved to review.", updated.ID)
}

func ticketJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "Error: " + err.Error()
	}
	return string(data)
}
