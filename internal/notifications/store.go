package notifications

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

// Notification is a stored notification entry.
type Notification struct {
	ID          string `json:"id"`
	SessionName string `json:"session_name"`
	Message     string `json:"message"`
	Timestamp   string `json:"timestamp"`
	Read        bool   `json:"read"`
}

// Store manages persistent notifications.
type Store struct {
	path          string
	notifications []Notification
}

// DefaultStorePath returns the default notification store path.
func DefaultStorePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "nagare", "notifications.json")
}

// NewStore loads or creates a notification store.
func NewStore(path string) *Store {
	s := &Store{path: path}
	s.load()
	return s
}

func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		s.notifications = nil
		return
	}
	json.Unmarshal(data, &s.notifications)
}

func (s *Store) save() {
	data, _ := json.MarshalIndent(s.notifications, "", "  ")
	os.MkdirAll(filepath.Dir(s.path), 0755)
	os.WriteFile(s.path, data, 0644)
}

// Add appends a new notification.
func (s *Store) Add(sessionName, message string) {
	s.notifications = append(s.notifications, Notification{
		ID:          uuid.New().String(),
		SessionName: sessionName,
		Message:     message,
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
		Read:        false,
	})
	s.save()
}

// ListAll returns notifications in reverse chronological order (newest first).
func (s *Store) ListAll() []Notification {
	sorted := make([]Notification, len(s.notifications))
	copy(sorted, s.notifications)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp > sorted[j].Timestamp
	})
	return sorted
}

// MarkRead marks a notification as read by ID.
func (s *Store) MarkRead(id string) {
	for i := range s.notifications {
		if s.notifications[i].ID == id {
			s.notifications[i].Read = true
			s.save()
			return
		}
	}
}

// Dismiss removes a notification by ID.
func (s *Store) Dismiss(id string) {
	for i := range s.notifications {
		if s.notifications[i].ID == id {
			s.notifications = append(s.notifications[:i], s.notifications[i+1:]...)
			s.save()
			return
		}
	}
}

// DismissAll removes all notifications.
func (s *Store) DismissAll() {
	s.notifications = nil
	s.save()
}

// UnreadCount returns the number of unread notifications.
func (s *Store) UnreadCount() int {
	count := 0
	for _, n := range s.notifications {
		if !n.Read {
			count++
		}
	}
	return count
}
