package notifications

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/nemke/nagare-go/internal/tmux"
)

// BuildToastMessage creates a human-readable notification message.
func BuildToastMessage(sessionName, eventType, notificationType string) string {
	switch eventType {
	case "needs_input":
		if notificationType == "permission_prompt" {
			return fmt.Sprintf("🔴 %s needs permission", sessionName)
		}
		return fmt.Sprintf("🔴 %s needs input", sessionName)
	case "task_complete":
		return fmt.Sprintf("✅ %s finished", sessionName)
	default:
		return fmt.Sprintf("📢 %s: %s", sessionName, eventType)
	}
}

// SendToast sends a tmux status bar message.
func SendToast(message string, durationMs int) {
	client := tmux.RunTmux("list-clients", "-F", "#{client_name}")
	lines := strings.Split(client, "\n")
	if len(lines) == 0 || lines[0] == "" {
		return
	}
	tmux.RunTmux("display-message", "-t", lines[0], "-d", fmt.Sprintf("%d", durationMs), message)
}

// SendBell sends a terminal bell character.
func SendBell() {
	tmux.RunTmux("run-shell", `printf '\a'`)
}

// DetectOsNotifyCmd returns the best available OS notification command, or nil.
func DetectOsNotifyCmd() []string {
	// WSL
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		if path, err := exec.LookPath("wsl-notify-send"); err == nil {
			return []string{path}
		}
		return nil
	}

	// Linux
	if path, err := exec.LookPath("notify-send"); err == nil {
		return []string{path}
	}

	// macOS
	if path, err := exec.LookPath("osascript"); err == nil {
		return []string{path, "-e"}
	}

	return nil
}

// SendOsNotify sends a native OS notification.
func sendOsNotifyArgs(title, message string) (string, []string) {
	cmd := DetectOsNotifyCmd()
	if cmd == nil {
		return "", nil
	}
	// osascript needs a different invocation
	if strings.Contains(cmd[0], "osascript") {
		script := fmt.Sprintf(`display notification "%s" with title "%s"`, message, title)
		return cmd[0], []string{"-e", script}
	}
	return cmd[0], []string{title, message}
}

// SendOsNotify sends a native OS notification.
func SendOsNotify(title, message string) {
	bin, args := sendOsNotifyArgs(title, message)
	if bin == "" {
		return
	}
	exec.Command(bin, args...).Start()
}

// Deliver dispatches a pre-built message through the enabled channels.
func Deliver(message string, toast, bell, osNotify bool, durationMs int) {
	if toast {
		SendToast(message, durationMs)
	}
	if bell {
		SendBell()
	}
	if osNotify {
		SendOsNotify("nagare", message)
	}
}
