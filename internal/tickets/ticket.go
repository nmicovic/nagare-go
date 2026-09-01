// Package tickets provides Nagare's durable unit of work above agent sessions.
package tickets

import (
	"fmt"
	"strings"
	"time"
)

// Status is the human workflow state of a ticket. It is deliberately separate
// from the live status of any agent assigned to the ticket.
type Status string

const (
	StatusBacklog  Status = "backlog"
	StatusReady    Status = "ready"
	StatusRunning  Status = "running"
	StatusReview   Status = "review"
	StatusDone     Status = "done"
	StatusCanceled Status = "canceled"
)

// BoardStatuses is the left-to-right order used by the kanban board.
var BoardStatuses = []Status{StatusBacklog, StatusReady, StatusRunning, StatusReview, StatusDone}

// Priority orders tickets within a column.
type Priority string

const (
	PriorityUrgent Priority = "urgent"
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
)

// Ticket is a durable desired outcome. Assignment fields identify the current
// execution attempt; submission fields snapshot its handoff for later reports.
type Ticket struct {
	ID                 string     `json:"id"`
	Title              string     `json:"title"`
	Description        string     `json:"description,omitempty"`
	ProjectPath        string     `json:"project_path,omitempty"`
	Status             Status     `json:"status"`
	Priority           Priority   `json:"priority"`
	PlannedFor         string     `json:"planned_for,omitempty"`
	AssigneeSession    string     `json:"assignee_session,omitempty"`
	AssigneePaneID     string     `json:"assignee_pane_id,omitempty"`
	AssigneeAgent      string     `json:"assignee_agent,omitempty"`
	SubmittedSummary   string     `json:"submitted_summary,omitempty"`
	SubmittedBySession string     `json:"submitted_by_session,omitempty"`
	SubmittedByAgent   string     `json:"submitted_by_agent,omitempty"`
	SubmittedRepoPath  string     `json:"submitted_repo_path,omitempty"`
	SubmittedAt        *time.Time `json:"submitted_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
}

// RecordSubmission snapshots the completed execution attempt for review and
// reporting. Assignment fields remain available for live board navigation.
func (t *Ticket) RecordSubmission(summary string, submittedAt time.Time) {
	at := submittedAt.UTC()
	t.SubmittedSummary = strings.TrimSpace(summary)
	t.SubmittedBySession = t.AssigneeSession
	t.SubmittedByAgent = t.AssigneeAgent
	t.SubmittedRepoPath = t.ProjectPath
	t.SubmittedAt = &at
}

// ClearSubmission removes a previous execution attempt's report before the
// ticket is assigned again.
func (t *Ticket) ClearSubmission() {
	t.SubmittedSummary = ""
	t.SubmittedBySession = ""
	t.SubmittedByAgent = ""
	t.SubmittedRepoPath = ""
	t.SubmittedAt = nil
}

func (t *Ticket) hydrateSubmission() {
	if t.SubmittedSummary == "" {
		return
	}
	if t.SubmittedBySession == "" {
		t.SubmittedBySession = t.AssigneeSession
	}
	if t.SubmittedByAgent == "" {
		t.SubmittedByAgent = t.AssigneeAgent
	}
	if t.SubmittedRepoPath == "" {
		t.SubmittedRepoPath = t.ProjectPath
	}
	if t.SubmittedAt == nil && !t.UpdatedAt.IsZero() {
		at := t.UpdatedAt.UTC()
		t.SubmittedAt = &at
	}
}

// CreateInput contains user-controlled fields for a new ticket.
type CreateInput struct {
	Title       string
	Description string
	ProjectPath string
	Status      Status
	Priority    Priority
	PlannedFor  string
}

// Valid reports whether s is a recognized workflow state.
func (s Status) Valid() bool {
	switch s {
	case StatusBacklog, StatusReady, StatusRunning, StatusReview, StatusDone, StatusCanceled:
		return true
	default:
		return false
	}
}

// Valid reports whether p is a recognized priority.
func (p Priority) Valid() bool {
	switch p {
	case PriorityUrgent, PriorityHigh, PriorityMedium, PriorityLow:
		return true
	default:
		return false
	}
}

// Validate checks the persistent ticket invariants.
func (t Ticket) Validate() error {
	if strings.TrimSpace(t.ID) == "" {
		return fmt.Errorf("ticket ID is empty")
	}
	if strings.TrimSpace(t.Title) == "" {
		return fmt.Errorf("ticket title is empty")
	}
	if !t.Status.Valid() {
		return fmt.Errorf("invalid ticket status %q", t.Status)
	}
	if !t.Priority.Valid() {
		return fmt.Errorf("invalid ticket priority %q", t.Priority)
	}
	if t.PlannedFor != "" {
		if _, err := time.Parse(time.DateOnly, t.PlannedFor); err != nil {
			return fmt.Errorf("invalid planned date %q: %w", t.PlannedFor, err)
		}
	}
	return nil
}

// StatusLabel returns the compact label used in the board UI and tool output.
func StatusLabel(s Status) string {
	switch s {
	case StatusBacklog:
		return "BACKLOG"
	case StatusReady:
		return "READY"
	case StatusRunning:
		return "RUNNING"
	case StatusReview:
		return "REVIEW"
	case StatusDone:
		return "DONE"
	case StatusCanceled:
		return "CANCELED"
	default:
		return strings.ToUpper(string(s))
	}
}

// PriorityRank returns a stable sort rank, highest priority first.
func PriorityRank(p Priority) int {
	switch p {
	case PriorityUrgent:
		return 0
	case PriorityHigh:
		return 1
	case PriorityMedium:
		return 2
	case PriorityLow:
		return 3
	default:
		return 4
	}
}
