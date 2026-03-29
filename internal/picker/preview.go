package picker

import "github.com/nemke/nagare-go/internal/tmux"

// CapturePreview captures the current pane content for a session.
func CapturePreview(sessionName string, windowIndex, paneIndex int) string {
	return tmux.RunTmux("capture-pane", "-t", tmux.PaneTarget(sessionName, windowIndex, paneIndex), "-p")
}
