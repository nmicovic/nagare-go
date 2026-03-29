package tmux

import (
	"fmt"
	"os/exec"
	"strings"
)

// RunTmux runs a tmux command and returns stdout. Returns empty string on error.
func RunTmux(args ...string) string {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

// PaneTarget formats a tmux pane target string (e.g., "session:0.1").
func PaneTarget(sessionName string, windowIndex, paneIndex int) string {
	return fmt.Sprintf("%s:%d.%d", sessionName, windowIndex, paneIndex)
}
