package picker

import (
	"fmt"

	"github.com/nemke/nagare-go/internal/tmux"
)

// CapturePreview captures the current pane content for a session.
func CapturePreview(sessionName string, windowIndex, paneIndex int) string {
	target := fmt.Sprintf("%s:%d.%d", sessionName, windowIndex, paneIndex)
	return tmux.RunTmux("capture-pane", "-t", target, "-p")
}
