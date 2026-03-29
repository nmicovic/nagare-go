package picker

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/nemke/nagare-go/internal/models"
	"github.com/sahilm/fuzzy"
	"github.com/nemke/nagare-go/internal/state"
	"github.com/nemke/nagare-go/internal/theme"
	"github.com/nemke/nagare-go/internal/tmux"
)

// ViewMode controls the session display layout.
type ViewMode int

const (
	ListView ViewMode = iota
	GridView
)

// SortMode controls the session sort order.
type SortMode int

const (
	SortByStatus SortMode = iota
	SortByName
	SortByAgent
)

// Message types for async updates.
type SessionsUpdatedMsg []models.Session
type PreviewUpdatedMsg string

type tickScanMsg struct{}
type tickPreviewMsg struct{}

// Model is the main Bubble Tea model for the picker TUI.
type Model struct {
	sessions    []models.Session
	filtered    []models.Session
	cursor      int
	viewMode    ViewMode
	sortMode    SortMode
	preview     string
	width       int
	height      int
	statesDir   string
	searchInput textinput.Model
}

// New creates a new picker model with default settings.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "search sessions..."
	ti.Prompt = " / "
	ti.CharLimit = 64

	ti.Focus()

	return Model{
		statesDir:   state.DefaultStatesDir(),
		searchInput: ti,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		doScan(m.statesDir),
		doPreviewTick(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case SessionsUpdatedMsg:
		m.sessions = []models.Session(msg)
		m.applyFilter()
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return tickScanMsg{} })

	case PreviewUpdatedMsg:
		m.preview = string(msg)
		return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return tickPreviewMsg{} })

	case tickScanMsg:
		return m, doScan(m.statesDir)

	case tickPreviewMsg:
		return m, m.doPreview()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	leftOuter := m.width / 5
	rightOuter := m.width - leftOuter

	left := m.viewLeft(leftOuter, m.height)
	right := m.viewRight(rightOuter, m.height)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// --- Key handling ---

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case keyEscape:
		return m, tea.Quit
	case keyEnter:
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			target := fmt.Sprintf("%s:%d", s.Name, s.WindowIndex)
			tmux.RunTmux("switch-client", "-t", target)
			return m, tea.Quit
		}
	case keyUp:
		if m.cursor > 0 {
			m.cursor--
			return m, m.doPreview()
		}
		return m, nil
	case keyDown:
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			return m, m.doPreview()
		}
		return m, nil
	case keyToggleView:
		if m.viewMode == ListView {
			m.viewMode = GridView
		} else {
			m.viewMode = ListView
		}
		return m, nil
	case keyCycleTheme:
		theme.CycleNext()
		return m, nil
	case keyApprove:
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			tmux.RunTmux("send-keys", "-t", tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex), "y", "Enter")
		}
		return m, nil
	case keyApproveAlways:
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			tmux.RunTmux("send-keys", "-t", tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex), "a", "Enter")
		}
		return m, nil
	default:
		// All other keys go to search input
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.applyFilter()
		return m, cmd
	}

	return m, nil
}

// --- Commands ---

func doScan(statesDir string) tea.Cmd {
	return func() tea.Msg {
		hookStates := state.LoadAllStates(statesDir)
		sessions := tmux.ScanSessions(hookStates)
		return SessionsUpdatedMsg(sessions)
	}
}

func doPreviewTick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return tickPreviewMsg{} })
}

func (m Model) doPreview() tea.Cmd {
	if len(m.filtered) == 0 {
		return func() tea.Msg { return PreviewUpdatedMsg("") }
	}
	s := m.filtered[m.cursor]
	return func() tea.Msg {
		content := CapturePreview(s.Name, s.WindowIndex, s.PaneIndex)
		return PreviewUpdatedMsg(content)
	}
}

// --- Filtering & sorting ---

func (m *Model) applyFilter() {
	query := m.searchInput.Value()
	if query == "" {
		m.filtered = make([]models.Session, len(m.sessions))
		copy(m.filtered, m.sessions)
		m.sortFiltered()
	} else {
		// Build search targets: "name path" for each session
		targets := make([]string, len(m.sessions))
		for i, s := range m.sessions {
			targets[i] = s.Name + " " + s.Path
		}

		matches := fuzzy.Find(query, targets)
		m.filtered = make([]models.Session, len(matches))
		for i, match := range matches {
			m.filtered[i] = m.sessions[match.Index]
		}
		// fuzzy.Find returns results sorted by score (best first)
		// so cursor 0 = best match
		m.cursor = 0
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m *Model) sortFiltered() {
	sort.SliceStable(m.filtered, func(i, j int) bool {
		switch m.sortMode {
		case SortByName:
			return m.filtered[i].Name < m.filtered[j].Name
		case SortByAgent:
			return m.filtered[i].AgentType < m.filtered[j].AgentType
		default: // SortByStatus
			return statusOrder(m.filtered[i].Status) < statusOrder(m.filtered[j].Status)
		}
	})
}

func statusOrder(s models.SessionStatus) int {
	switch s {
	case models.StatusWaitingInput:
		return 0
	case models.StatusRunning:
		return 1
	case models.StatusIdle:
		return 2
	case models.StatusDead:
		return 3
	default:
		return 4
	}
}

// --- View rendering ---

func (m Model) viewLeft(outerWidth, outerHeight int) string {
	innerWidth := outerWidth - 4
	if innerWidth < 10 {
		innerWidth = 10
	}

	var b strings.Builder

	// Dashboard stats
	total := len(m.sessions)
	waiting := 0
	running := 0
	for _, s := range m.sessions {
		switch s.Status {
		case models.StatusWaitingInput:
			waiting++
		case models.StatusRunning:
			running++
		}
	}
	b.WriteString(mutedStyle().Render(fmt.Sprintf(" %d sessions | %d waiting | %d running", total, waiting, running)))
	b.WriteString("\n\n")

	// Search input (always active)
	b.WriteString(m.searchInput.View())
	b.WriteString("\n\n")

	// Session list
	listHeight := outerHeight - 10
	if listHeight < 1 {
		listHeight = 1
	}

	if m.viewMode == ListView {
		b.WriteString(m.renderListView(innerWidth, listHeight))
	} else {
		b.WriteString(m.renderGridView(innerWidth, listHeight))
	}

	return panelStyle().
		Width(outerWidth).
		Height(outerHeight).
		Render(b.String())
}

func (m Model) renderListView(width, height int) string {
	if len(m.filtered) == 0 {
		return mutedStyle().Render("  No sessions found")
	}

	start := 0
	if m.cursor >= height {
		start = m.cursor - height + 1
	}
	end := start + height
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	c := theme.Current().Colors

	var lines []string
	for i := start; i < end; i++ {
		s := m.filtered[i]
		dot := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(s.Status))).Render("●")
		badge := lipgloss.NewStyle().
			Foreground(lipgloss.Color(models.AgentColor(s.AgentType))).
			Background(lipgloss.Color(models.AgentBgColor(s.AgentType))).
			Padding(0, 1).
			Render(string(models.AgentLabel(s.AgentType)[0]))

		name := s.Name
		maxName := width - 20
		if maxName < 5 {
			maxName = 5
		}
		if utf8.RuneCountInString(name) > maxName {
			runes := []rune(name)
			name = string(runes[:maxName]) + "..."
		}

		var line string
		if i == m.cursor {
			// Selected: single style for entire line — no inner Render() calls
			// to avoid ANSI resets killing the background
			agentChar := string(models.AgentLabel(s.AgentType)[0])
			line = lipgloss.NewStyle().
				Background(c.Primary).
				Foreground(c.Background).
				Bold(true).
				PaddingLeft(1).
				Width(width).
				Render(fmt.Sprintf("> ● %s  %s", name, agentChar))
		} else {
			nameStyled := lipgloss.NewStyle().Foreground(c.Foreground).Render(name)
			content := fmt.Sprintf(" %s %s %s", dot, nameStyled, badge)
			line = lipgloss.NewStyle().
				Background(c.Background).
				Foreground(c.Foreground).
				PaddingLeft(2).
				Width(width).
				Render(content)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderGridView(width, height int) string {
	if len(m.filtered) == 0 {
		return mutedStyle().Render("  No sessions found")
	}

	cols := 2
	cellWidth := (width - 2) / cols
	if cellWidth < 15 {
		cols = 1
		cellWidth = width - 2
	}

	c := theme.Current().Colors

	var rows []string
	for i := 0; i < len(m.filtered); i += cols {
		var cells []string
		for j := 0; j < cols && i+j < len(m.filtered); j++ {
			idx := i + j
			s := m.filtered[idx]
			dot := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(s.Status))).Render("●")
			name := s.Name
			maxLen := cellWidth - 6
			if utf8.RuneCountInString(name) > maxLen {
				runes := []rune(name)
				name = string(runes[:maxLen]) + ".."
			}
			content := fmt.Sprintf(" %s %s", dot, name)
			var cell string
			if idx == m.cursor {
				cell = lipgloss.NewStyle().
					Background(c.Primary).Foreground(c.Background).Bold(true).
					Width(cellWidth).Render(content)
			} else {
				cell = lipgloss.NewStyle().
					Background(c.Background).Foreground(c.Foreground).
					Width(cellWidth).Render(content)
			}
			cells = append(cells, cell)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	return strings.Join(rows, "\n")
}

func (m Model) viewRight(outerWidth, outerHeight int) string {
	if len(m.filtered) == 0 {
		return panelStyle().
			Width(outerWidth).
			Height(outerHeight).
			Render(mutedStyle().Render("No session selected"))
	}

	innerWidth := outerWidth - 4
	if innerWidth < 10 {
		innerWidth = 10
	}

	s := m.filtered[m.cursor]
	c := theme.Current().Colors

	// Detail section
	label := lipgloss.NewStyle().Foreground(c.Muted)
	val := lipgloss.NewStyle().Foreground(c.Foreground)
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(s.Status)))
	agentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(models.AgentColor(s.AgentType))).
		Background(lipgloss.Color(models.AgentBgColor(s.AgentType))).
		Padding(0, 1)

	var detail strings.Builder
	detail.WriteString(titleStyle().Render(s.Name))
	detail.WriteString("\n\n")
	detail.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Path  "), val.Render(s.Path)))
	detail.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Agent "), agentStyle.Render(models.AgentLabel(s.AgentType))))
	detail.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Status"), statusStyle.Render(models.StatusLabel(s.Status))))

	if s.Details.GitBranch != "" {
		detail.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Branch"), val.Render(s.Details.GitBranch)))
	}
	if s.Details.LastActivity != "" {
		detail.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Active"), val.Render(formatTimeAgo(s.Details.LastActivity))))
	}
	if s.LastMessage != "" {
		// Truncate long messages
		msg := s.LastMessage
		maxLen := innerWidth - 12
		if maxLen > 0 && len(msg) > maxLen {
			msg = msg[:maxLen] + "..."
		}
		detail.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Last  "), mutedStyle().Render(msg)))
	}

	detailHeight := outerHeight / 5
	detailStr := panelStyle().
		Width(outerWidth).
		Height(detailHeight).
		Render(detail.String())

	// Preview section
	previewHeight := outerHeight - detailHeight
	if previewHeight < 3 {
		previewHeight = 3
	}

	previewContent := m.preview
	if previewContent == "" {
		previewContent = mutedStyle().Render("No preview available")
	} else {
		maxLines := previewHeight - 4
		if maxLines < 1 {
			maxLines = 1
		}
		lines := strings.Split(previewContent, "\n")
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
		}
		for i, line := range lines {
			if ansi.StringWidth(line) > innerWidth {
				lines[i] = ansi.Truncate(line, innerWidth, "")
			}
		}
		previewContent = strings.Join(lines, "\n")
	}

	previewStr := previewPanelStyle().
		Width(outerWidth).
		Height(previewHeight).
		Render(previewContent)

	return lipgloss.JoinVertical(lipgloss.Left, detailStr, previewStr)
}

// formatTimeAgo converts an ISO 8601 timestamp to a human-readable relative time.
func formatTimeAgo(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh %dm ago", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
