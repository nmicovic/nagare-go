package fsutil

import "os"

// AtomicWrite writes data to a temp file and renames it into place.
// This prevents corruption from concurrent readers/writers.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
