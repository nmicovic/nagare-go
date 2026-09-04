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

// RunStrict runs a state-changing tmux command and reports failures to the
// caller. Provisioning code must use this instead of the best-effort scanner
// wrapper above.
func RunStrict(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// PaneExists reports whether paneID still identifies a live tmux pane.
func PaneExists(paneID string) bool {
	if strings.TrimSpace(paneID) == "" {
		return false
	}
	out, err := RunStrict("display-message", "-p", "-t", paneID, "#{pane_id}")
	return err == nil && strings.TrimSpace(out) == paneID
}

// PaneTarget formats a tmux pane target string (e.g., "session:0.1").
func PaneTarget(sessionName string, windowIndex, paneIndex int) string {
	return fmt.Sprintf("%s:%d.%d", sessionName, windowIndex, paneIndex)
}
