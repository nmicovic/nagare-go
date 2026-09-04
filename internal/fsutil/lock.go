package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

const (
	lockWait  = 5 * time.Second
	lockStale = time.Minute
)

// WithFileLock serializes a short record update across Nagare processes using
// atomic directory creation. A crashed writer's lock expires after one minute.
func WithFileLock(path string, fn func() error) error {
	deadline := time.Now().Add(lockWait)
	for {
		err := os.Mkdir(path, 0o700)
		if err == nil {
			defer os.Remove(path)
			return fn()
		}
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("create record lock: %w", err)
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > lockStale {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for record lock %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
