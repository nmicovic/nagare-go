package attempts

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
	"github.com/nemke/nagare-go/internal/fsutil"
)

// Store persists each attempt in its own atomically replaced file.
type Store struct {
	dir string
}

// DefaultDir returns Nagare's per-user attempt directory.
func DefaultDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "nagare", "attempts")
}

// NewStore creates an attempt store rooted at dir.
func NewStore(dir string) *Store { return &Store{dir: dir} }

// Create persists a provisioning attempt before any external resource is created.
func (s *Store) Create(input CreateInput) (Attempt, error) {
	now := time.Now().UTC()
	attempt := Attempt{
		ID:           uuid.NewString(),
		TicketID:     strings.TrimSpace(input.TicketID),
		State:        StateProvisioning,
		Agent:        strings.TrimSpace(input.Agent),
		Model:        strings.TrimSpace(input.Model),
		ProjectPath:  cleanPath(input.ProjectPath),
		TargetBranch: strings.TrimSpace(input.TargetBranch),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := attempt.Validate(); err != nil {
		return Attempt{}, err
	}
	if err := s.write(attempt); err != nil {
		return Attempt{}, err
	}
	return attempt, nil
}

// Get loads an attempt by full ID.
func (s *Store) Get(id string) (Attempt, error) {
	id = strings.TrimSpace(id)
	if id == "" || filepath.Base(id) != id || strings.Contains(id, ".") {
		return Attempt{}, fmt.Errorf("invalid attempt ID %q", id)
	}
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return Attempt{}, fmt.Errorf("read attempt %s: %w", id, err)
	}
	var attempt Attempt
	if err := json.Unmarshal(data, &attempt); err != nil {
		return Attempt{}, fmt.Errorf("decode attempt %s: %w", id, err)
	}
	if err := attempt.Validate(); err != nil {
		return Attempt{}, fmt.Errorf("validate attempt %s: %w", id, err)
	}
	return attempt, nil
}

// Update reloads and atomically replaces one attempt.
func (s *Store) Update(id string, mutate func(*Attempt) error) (Attempt, error) {
	id = strings.TrimSpace(id)
	if id == "" || filepath.Base(id) != id || strings.Contains(id, ".") {
		return Attempt{}, fmt.Errorf("invalid attempt ID %q", id)
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return Attempt{}, err
	}
	var updated Attempt
	err := fsutil.WithFileLock(s.path(id)+".lock", func() error {
		attempt, err := s.Get(id)
		if err != nil {
			return err
		}
		if err := mutate(&attempt); err != nil {
			return err
		}
		attempt.ProjectPath = cleanPath(attempt.ProjectPath)
		attempt.WorktreePath = cleanPath(attempt.WorktreePath)
		attempt.UpdatedAt = time.Now().UTC()
		if err := attempt.Validate(); err != nil {
			return err
		}
		if err := s.write(attempt); err != nil {
			return err
		}
		updated = attempt
		return nil
	})
	if err != nil {
		return Attempt{}, err
	}
	return updated, nil
}

// List returns every attempt in creation order.
func (s *Store) List() ([]Attempt, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read attempts: %w", err)
	}
	var result []Attempt
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		attempt, err := s.Get(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		result = append(result, attempt)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

// ListForTicket returns attempts in creation order for one ticket.
func (s *Store) ListForTicket(ticketID string) ([]Attempt, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	result := make([]Attempt, 0, len(all))
	for _, attempt := range all {
		if attempt.TicketID == ticketID {
			result = append(result, attempt)
		}
	}
	return result, nil
}

func (s *Store) write(attempt Attempt) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create attempt directory: %w", err)
	}
	data, err := json.MarshalIndent(attempt, "", "  ")
	if err != nil {
		return fmt.Errorf("encode attempt: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(s.dir, ".attempt-*.tmp")
	if err != nil {
		return fmt.Errorf("create attempt temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write attempt: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync attempt: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close attempt: %w", err)
	}
	if err := os.Rename(tmpName, s.path(attempt.ID)); err != nil {
		return fmt.Errorf("replace attempt: %w", err)
	}
	return nil
}

func (s *Store) path(id string) string { return filepath.Join(s.dir, id+".json") }

func cleanPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if absolute, err := filepath.Abs(path); err == nil {
		return filepath.Clean(absolute)
	}
	return filepath.Clean(path)
}
