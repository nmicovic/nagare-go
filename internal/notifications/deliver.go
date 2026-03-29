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
	exec.Command("tmux", "run-shell", `printf '\a'`).Run()
}

// DetectOsNotifyCmd returns the best available OS notification command, or nil.
func DetectOsNotifyCmd() []string {
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		if path, err := exec.LookPath("wsl-notify-send"); err == nil {
			return []string{path}
		}
		return nil
	}

	if path, err := exec.LookPath("notify-send"); err == nil {
		return []string{path}
	}

	return nil
}

// SendOsNotify sends a native OS notification.
func SendOsNotify(title, message string) {
	cmd := DetectOsNotifyCmd()
	if cmd == nil {
		return
	}
	args := append(cmd, title, message)
	exec.Command(args[0], args[1:]...).Start()
}

// Deliver dispatches notifications based on config flags.
func Deliver(sessionName, eventType, notificationType string, toast, bell, osNotify bool, durationMs int) {
	message := BuildToastMessage(sessionName, eventType, notificationType)

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
