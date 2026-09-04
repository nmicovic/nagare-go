package fsutil

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWithFileLockSerializesWriters(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "record.lock")
	var mu sync.Mutex
	active := 0
	maxActive := 0
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := WithFileLock(lockPath, func() error {
				mu.Lock()
				active++
				if active > maxActive {
					maxActive = active
				}
				mu.Unlock()
				time.Sleep(25 * time.Millisecond)
				mu.Lock()
				active--
				mu.Unlock()
				return nil
			}); err != nil {
				t.Errorf("WithFileLock: %v", err)
			}
		}()
	}
	wait.Wait()
	if maxActive != 1 {
		t.Fatalf("concurrent lock holders = %d", maxActive)
	}
}
