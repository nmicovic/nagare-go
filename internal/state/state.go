package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/nemke/nagare-go/internal/fsutil"
	"github.com/nemke/nagare-go/internal/models"
)

// DefaultStatesDir returns the default states directory path.
func DefaultStatesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "nagare", "states")
}

// LoadAllStates loads all state files from dir, keyed by cwd.
// Conflict resolution: live beats dead, then newer timestamp wins.
func LoadAllStates(dir string) map[string]models.SessionState {
	states := make(map[string]models.SessionState)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return states
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var s models.SessionState
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}

		if s.Cwd == "" {
			continue
		}

		existing, exists := states[s.Cwd]
		if !exists {
			states[s.Cwd] = s
			continue
		}

		// Live beats dead
		if existing.State == "dead" && s.State != "dead" {
			states[s.Cwd] = s
		} else if existing.State != "dead" && s.State == "dead" {
			// Keep existing live state
		} else if s.Timestamp > existing.Timestamp {
			// Same liveness: newer wins
			states[s.Cwd] = s
		}
	}

	return states
}

// LoadStateByID loads a single state file by session ID. Returns zero value and false if not found.
func LoadStateByID(dir, sessionID string) (models.SessionState, bool) {
	path := filepath.Join(dir, sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return models.SessionState{}, false
	}
	var s models.SessionState
	if err := json.Unmarshal(data, &s); err != nil {
		return models.SessionState{}, false
	}
	return s, true
}

// WriteState writes a session state to dir/{session_id}.json atomically.
func WriteState(dir string, s models.SessionState) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	path := filepath.Join(dir, s.SessionID+".json")
	return fsutil.AtomicWrite(path, data, 0644)
}
