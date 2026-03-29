package popup

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/nemke/nagare-go/internal/session"
	"github.com/nemke/nagare-go/internal/theme"
	"github.com/nemke/nagare-go/internal/tmux"
)

// Model is the Bubble Tea model for the popup notification TUI.
type Model struct {
	sessionName    string
	eventType      string
	message        string
	workingSeconds int
	timeout        int
	countdown      int
	preview        string
	width          int
	height         int
}

// New creates a new popup notification model.
func New(sessionName, eventType, message string, timeout, workingSeconds int) Model {
	return Model{
		sessionName:    sessionName,
		eventType:      eventType,
		message:        message,
		workingSeconds: workingSeconds,
		timeout:        timeout,
		countdown:      timeout,
	}
}

// tickMsg is sent every second for countdown.
type tickMsg struct{}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		m.doPreview(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m Model) doPreview() tea.Cmd {
	return func() tea.Msg {
		content := tmux.RunTmux("capture-pane", "-e", "-t", m.sessionName, "-p")
		return previewMsg(content)
	}
}

type previewMsg string

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		m.countdown--
		if m.countdown <= 0 {
			return m, tea.Quit
		}
		return m, tea.Batch(tickCmd(), m.doPreview())

	case previewMsg:
		m.preview = string(msg)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, tea.Quit
	case "enter":
		session.SwitchToSession(m.sessionName)
		return m, tea.Quit
	case "ctrl+y":
		if m.eventType == "needs_input" {
			tmux.RunTmux("send-keys", "-t", m.sessionName, "Enter")
			return m, tea.Quit
		}
	case "ctrl+a":
		if m.eventType == "needs_input" {
			tmux.RunTmux("send-keys", "-t", m.sessionName, "Down", "Enter")
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	c := theme.Current().Colors

	innerWidth := m.width - 4
	if innerWidth < 10 {
		innerWidth = 10
	}

	// Header
	header := m.renderHeader(innerWidth)

	// Preview
	previewHeight := m.height - 7 // header + separator + hint + padding
	if previewHeight < 3 {
		previewHeight = 3
	}
	preview := m.renderPreview(innerWidth, previewHeight)

	// Hint bar
	hint := m.renderHintBar(innerWidth)

	// Separator line
	separator := lipgloss.NewStyle().
		Foreground(c.Border).
		Render(strings.Repeat("─", innerWidth))

	content := strings.Join([]string{
		header,
		separator,
		preview,
		separator,
		hint,
	}, "\n")

	return lipgloss.NewStyle().
		Background(c.Background).
		Foreground(c.Foreground).
		Width(m.width).
		Height(m.height).
		Padding(1).
		Render(content)
}

func (m Model) renderHeader(width int) string {
	c := theme.Current().Colors

	var statusText string
	var statusColor lipgloss.AdaptiveColor
	if m.eventType == "needs_input" {
		statusText = "● NEEDS INPUT"
		statusColor = c.Error
	} else if m.eventType == "task_complete" {
		duration := formatDuration(m.workingSeconds)
		statusText = fmt.Sprintf("● TASK COMPLETE (worked %s)", duration)
		statusColor = c.Success
	} else {
		statusText = "● NOTIFICATION"
		statusColor = c.Primary
	}

	status := lipgloss.NewStyle().
		Foreground(statusColor).
		Bold(true).
		Render(statusText)

	// Truncate message if too long
	msg := m.message
	maxMsg := width - 4
	if ansi.StringWidth(msg) > maxMsg {
		msg = ansi.Truncate(msg, maxMsg-3, "...")
	}

	return status + "\n  " + msg
}

func (m Model) renderPreview(width, height int) string {
	if m.preview == "" {
		return lipgloss.NewStyle().
			Foreground(theme.Current().Colors.Muted).
			Render("  No preview available")
	}

	lines := strings.Split(m.preview, "\n")

	// Trim trailing empty lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	// Take the bottom portion (most recent)
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}

	// Truncate lines to width
	for i, line := range lines {
		if ansi.StringWidth(line) > width {
			lines[i] = ansi.Truncate(line, width, "")
		}
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderHintBar(width int) string {
	c := theme.Current().Colors

	var hints []string
	hints = append(hints, fmt.Sprintf("Enter Jump"))

	if m.eventType == "needs_input" {
		hints = append(hints, "Ctrl+y Allow")
		hints = append(hints, "Ctrl+a Always")
	}

	hints = append(hints, "Esc Dismiss")

	left := strings.Join(hints, "  ")
	right := fmt.Sprintf("Auto-closing in %ds", m.countdown)

	// Pad to align right
	leftWidth := ansi.StringWidth(left)
	rightWidth := ansi.StringWidth(right)
	padding := width - leftWidth - rightWidth
	if padding < 1 {
		padding = 1
	}

	return left + strings.Repeat(" ", padding) +
		lipgloss.NewStyle().Foreground(c.Muted).Render(right)
}

func formatDuration(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	return fmt.Sprintf("%dh%dm", seconds/3600, (seconds%3600)/60)
}
