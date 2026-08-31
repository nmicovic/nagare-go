package tickets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Store persists each ticket independently so agent MCP processes can update
// unrelated tickets without contending on or rewriting a global document.
type Store struct {
	dir string
}

// DefaultDir returns Nagare's per-user ticket directory.
func DefaultDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "nagare", "tickets")
}

// NewStore creates a ticket store rooted at dir.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// Create validates and atomically persists a new ticket.
func (s *Store) Create(input CreateInput) (Ticket, error) {
	now := time.Now().UTC()
	status := input.Status
	if status == "" {
		status = StatusBacklog
	}
	priority := input.Priority
	if priority == "" {
		priority = PriorityMedium
	}

	ticket := Ticket{
		ID:          uuid.NewString(),
		Title:       strings.TrimSpace(input.Title),
		Description: strings.TrimSpace(input.Description),
		ProjectPath: cleanProjectPath(input.ProjectPath),
		Status:      status,
		Priority:    priority,
		PlannedFor:  input.PlannedFor,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := ticket.Validate(); err != nil {
		return Ticket{}, err
	}
	if err := s.write(ticket); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

// List returns every ticket, sorted by priority and creation time.
func (s *Store) List() ([]Ticket, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read tickets: %w", err)
	}

	result := make([]Ticket, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read ticket %s: %w", entry.Name(), err)
		}
		var ticket Ticket
		if err := json.Unmarshal(data, &ticket); err != nil {
			return nil, fmt.Errorf("decode ticket %s: %w", entry.Name(), err)
		}
		if err := ticket.Validate(); err != nil {
			return nil, fmt.Errorf("validate ticket %s: %w", entry.Name(), err)
		}
		result = append(result, ticket)
	}

	sort.SliceStable(result, func(i, j int) bool {
		left, right := PriorityRank(result[i].Priority), PriorityRank(result[j].Priority)
		if left != right {
			return left < right
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

// Get resolves either a full ticket ID or an unambiguous ID prefix.
func (s *Store) Get(id string) (Ticket, error) {
	resolved, err := s.resolveID(id)
	if err != nil {
		return Ticket{}, err
	}
	data, err := os.ReadFile(s.path(resolved))
	if err != nil {
		return Ticket{}, fmt.Errorf("read ticket %s: %w", id, err)
	}
	var ticket Ticket
	if err := json.Unmarshal(data, &ticket); err != nil {
		return Ticket{}, fmt.Errorf("decode ticket %s: %w", id, err)
	}
	if err := ticket.Validate(); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

// Update reloads a ticket immediately before applying mutate, then atomically
// replaces that ticket's file. This keeps UI-held copies from overwriting newer
// agent updates.
func (s *Store) Update(id string, mutate func(*Ticket) error) (Ticket, error) {
	ticket, err := s.Get(id)
	if err != nil {
		return Ticket{}, err
	}
	if err := mutate(&ticket); err != nil {
		return Ticket{}, err
	}
	ticket.Title = strings.TrimSpace(ticket.Title)
	ticket.Description = strings.TrimSpace(ticket.Description)
	ticket.ProjectPath = cleanProjectPath(ticket.ProjectPath)
	ticket.UpdatedAt = time.Now().UTC()
	if err := ticket.Validate(); err != nil {
		return Ticket{}, err
	}
	if err := s.write(ticket); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

// Delete removes the ticket resolved by a full ID or unambiguous ID prefix.
func (s *Store) Delete(id string) error {
	resolved, err := s.resolveID(id)
	if err != nil {
		return err
	}
	if err := os.Remove(s.path(resolved)); err != nil {
		return fmt.Errorf("delete ticket %s: %w", id, err)
	}
	return nil
}

// SetStatus transitions a ticket and maintains completion metadata.
func (s *Store) SetStatus(id string, status Status) (Ticket, error) {
	if !status.Valid() {
		return Ticket{}, fmt.Errorf("invalid ticket status %q", status)
	}
	return s.Update(id, func(ticket *Ticket) error {
		ticket.Status = status
		if status == StatusDone {
			now := time.Now().UTC()
			ticket.CompletedAt = &now
		} else {
			ticket.CompletedAt = nil
		}
		return nil
	})
}

func (s *Store) resolveID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" || filepath.Base(id) != id || strings.Contains(id, ".") {
		return "", fmt.Errorf("invalid ticket ID %q", id)
	}
	if _, err := os.Stat(s.path(id)); err == nil {
		return id, nil
	}

	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("ticket %q not found", id)
	}
	if err != nil {
		return "", fmt.Errorf("read tickets: %w", err)
	}
	var match string
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".json")
		if entry.IsDir() || name == entry.Name() || !strings.HasPrefix(name, id) {
			continue
		}
		if match != "" {
			return "", fmt.Errorf("ticket prefix %q is ambiguous", id)
		}
		match = name
	}
	if match == "" {
		return "", fmt.Errorf("ticket %q not found", id)
	}
	return match, nil
}

func (s *Store) write(ticket Ticket) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create ticket directory: %w", err)
	}
	data, err := json.MarshalIndent(ticket, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ticket: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(s.dir, ".ticket-*.tmp")
	if err != nil {
		return fmt.Errorf("create ticket temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write ticket: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync ticket: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close ticket: %w", err)
	}
	if err := os.Rename(tmpName, s.path(ticket.ID)); err != nil {
		return fmt.Errorf("replace ticket: %w", err)
	}
	return nil
}

func (s *Store) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func cleanProjectPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil {
		switch {
		case path == "~":
			path = home
		case strings.HasPrefix(path, "~/"):
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if absolute, err := filepath.Abs(path); err == nil {
		return filepath.Clean(absolute)
	}
	return filepath.Clean(path)
}
