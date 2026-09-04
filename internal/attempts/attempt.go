// Package attempts persists ticket execution attempts independently from tickets.
package attempts

import (
	"fmt"
	"strings"
	"time"
)

// State is the infrastructure lifecycle of one ticket execution.
type State string

const (
	StateProvisioning State = "provisioning"
	StateRunning      State = "running"
	StateSubmitted    State = "submitted"
	StateFailed       State = "failed"
	StateArchived     State = "archived"
)

// Attempt records the immutable Git base and mutable execution resources for one run.
type Attempt struct {
	ID                string     `json:"id"`
	TicketID          string     `json:"ticket_id"`
	State             State      `json:"state"`
	Agent             string     `json:"agent"`
	ProjectPath       string     `json:"project_path"`
	TargetBranch      string     `json:"target_branch"`
	BaseCommit        string     `json:"base_commit,omitempty"`
	Branch            string     `json:"branch,omitempty"`
	WorktreePath      string     `json:"worktree_path,omitempty"`
	SessionName       string     `json:"session_name,omitempty"`
	PaneID            string     `json:"pane_id,omitempty"`
	LastError         string     `json:"last_error,omitempty"`
	PullRequestURL    string     `json:"pull_request_url,omitempty"`
	PullRequestNumber int        `json:"pull_request_number,omitempty"`
	PullRequestState  string     `json:"pull_request_state,omitempty"`
	PullRequestAt     *time.Time `json:"pull_request_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	SubmittedAt       *time.Time `json:"submitted_at,omitempty"`
	ArchivedAt        *time.Time `json:"archived_at,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// CreateInput contains the identity known before provisioning begins.
type CreateInput struct {
	TicketID     string
	Agent        string
	ProjectPath  string
	TargetBranch string
}

// Valid reports whether state is recognized.
func (s State) Valid() bool {
	switch s {
	case StateProvisioning, StateRunning, StateSubmitted, StateFailed, StateArchived:
		return true
	default:
		return false
	}
}

// Validate checks the persistent attempt invariants.
func (a Attempt) Validate() error {
	if strings.TrimSpace(a.ID) == "" {
		return fmt.Errorf("attempt ID is empty")
	}
	if strings.TrimSpace(a.TicketID) == "" {
		return fmt.Errorf("attempt ticket ID is empty")
	}
	if !a.State.Valid() {
		return fmt.Errorf("invalid attempt state %q", a.State)
	}
	if strings.TrimSpace(a.Agent) == "" {
		return fmt.Errorf("attempt agent is empty")
	}
	if strings.TrimSpace(a.ProjectPath) == "" {
		return fmt.Errorf("attempt project path is empty")
	}
	if strings.TrimSpace(a.TargetBranch) == "" {
		return fmt.Errorf("attempt target branch is empty")
	}
	return nil
}
