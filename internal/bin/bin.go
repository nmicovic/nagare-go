package bin

import (
	"os"
	"os/exec"
)

// FindSelf locates the nagare-go binary path.
func FindSelf() string {
	if path, err := exec.LookPath("nagare-go"); err == nil {
		return path
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "nagare-go"
}
