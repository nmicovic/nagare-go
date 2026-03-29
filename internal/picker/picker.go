package picker

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/state"
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
	searchMode  bool
	viewMode    ViewMode
	sortMode    SortMode
	preview     string
	width       int
	height      int
	styles      Styles
	themeIndex  int
	statesDir   string
	searchInput textinput.Model
}

// New creates a new picker model with default settings.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "search sessions..."
	ti.Prompt = " / "
	ti.CharLimit = 64

	return Model{
		styles:      NewStyles("tokyonight"),
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
		return m, tea.Tick(1*time.Second, func(time.Time) tea.Msg { return tickPreviewMsg{} })

	case tickScanMsg:
		return m, doScan(m.statesDir)

	case tickPreviewMsg:
		return m, m.doPreview()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.searchMode {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.applyFilter()
		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	leftWidth := m.width*2/5 - 2
	rightWidth := m.width - leftWidth - 5
	contentHeight := m.height - 2

	left := m.viewLeft(leftWidth, contentHeight)
	right := m.viewRight(rightWidth, contentHeight)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// --- Key handling ---

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// In search mode, handle escape and enter specially.
	if m.searchMode {
		switch key {
		case keyEscape:
			m.searchMode = false
			m.searchInput.Blur()
			m.searchInput.SetValue("")
			m.applyFilter()
			return m, nil
		case keyEnter:
			m.searchMode = false
			m.searchInput.Blur()
			return m, nil
		default:
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			m.applyFilter()
			return m, cmd
		}
	}

	switch key {
	case keyUp, keyK:
		if m.cursor > 0 {
			m.cursor--
		}
	case keyDown, keyJ:
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case keyEnter:
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			target := fmt.Sprintf("%s:%d", s.Name, s.WindowIndex)
			tmux.RunTmux("switch-client", "-t", target)
			return m, tea.Quit
		}
	case keySearch:
		m.searchMode = true
		return m, m.searchInput.Focus()
	case keyQuit:
		return m, tea.Quit
	case keyEscape:
		return m, tea.Quit
	case keyApprove:
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			tmux.RunTmux("send-keys", "-t", tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex), "y", "Enter")
		}
	case keyApproveAlways:
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			tmux.RunTmux("send-keys", "-t", tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex), "a", "Enter")
		}
	case keyInlinePrompt:
		// Placeholder for inline prompt feature.
	case keyToggleView:
		if m.viewMode == ListView {
			m.viewMode = GridView
		} else {
			m.viewMode = ListView
		}
	case keyCycleTheme:
		names := ThemeNames()
		m.themeIndex = (m.themeIndex + 1) % len(names)
		m.styles = NewStyles(names[m.themeIndex])
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
	return tea.Tick(1*time.Second, func(time.Time) tea.Msg { return tickPreviewMsg{} })
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
	query := strings.ToLower(m.searchInput.Value())
	if query == "" {
		m.filtered = make([]models.Session, len(m.sessions))
		copy(m.filtered, m.sessions)
	} else {
		m.filtered = m.filtered[:0]
		for _, s := range m.sessions {
			if strings.Contains(strings.ToLower(s.Name), query) ||
				strings.Contains(strings.ToLower(s.Path), query) {
				m.filtered = append(m.filtered, s)
			}
		}
	}

	m.sortFiltered()

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

func (m Model) viewLeft(width, height int) string {
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
	stats := m.styles.Muted.Render(fmt.Sprintf(" %d sessions | %d waiting | %d running", total, waiting, running))
	b.WriteString(stats)
	b.WriteString("\n\n")

	// Search input
	if m.searchMode {
		b.WriteString(m.styles.SearchInput.Width(width).Render(m.searchInput.View()))
	} else {
		b.WriteString(m.styles.Muted.Render(" / to search"))
	}
	b.WriteString("\n\n")

	// Session list
	listHeight := height - 6
	if listHeight < 1 {
		listHeight = 1
	}

	if m.viewMode == ListView {
		b.WriteString(m.renderListView(width, listHeight))
	} else {
		b.WriteString(m.renderGridView(width, listHeight))
	}

	return m.styles.SessionList.Width(width).Height(height).Render(b.String())
}

func (m Model) renderListView(width, height int) string {
	if len(m.filtered) == 0 {
		return m.styles.Muted.Render("  No sessions found")
	}

	// Scroll window
	start := 0
	if m.cursor >= height {
		start = m.cursor - height + 1
	}
	end := start + height
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	var lines []string
	for i := start; i < end; i++ {
		s := m.filtered[i]
		statusDot := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(s.Status))).Render("●")
		agentBadge := lipgloss.NewStyle().
			Foreground(lipgloss.Color(models.AgentColor(s.AgentType))).
			Render(models.AgentLabel(s.AgentType))

		name := s.Name
		if len(name) > width-20 {
			name = name[:width-20] + "..."
		}

		line := fmt.Sprintf(" %s %s  %s", statusDot, name, agentBadge)

		if i == m.cursor {
			line = m.styles.SelectedItem.Render("> " + line[1:])
		} else {
			line = m.styles.SessionItem.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderGridView(width, height int) string {
	if len(m.filtered) == 0 {
		return m.styles.Muted.Render("  No sessions found")
	}

	cols := 2
	cellWidth := (width - 2) / cols
	if cellWidth < 15 {
		cols = 1
		cellWidth = width - 2
	}

	var rows []string
	for i := 0; i < len(m.filtered); i += cols {
		var cells []string
		for c := 0; c < cols && i+c < len(m.filtered); c++ {
			idx := i + c
			s := m.filtered[idx]
			statusDot := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(s.Status))).Render("●")
			name := s.Name
			if len(name) > cellWidth-6 {
				name = name[:cellWidth-6] + ".."
			}
			cell := fmt.Sprintf(" %s %s", statusDot, name)
			if idx == m.cursor {
				cell = m.styles.SelectedItem.Width(cellWidth).Render(cell)
			} else {
				cell = m.styles.SessionItem.Width(cellWidth).Render(cell)
			}
			cells = append(cells, cell)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	return strings.Join(rows, "\n")
}

func (m Model) viewRight(width, height int) string {
	if len(m.filtered) == 0 {
		return m.styles.DetailPanel.Width(width).Height(height).Render(
			m.styles.Muted.Render("No session selected"),
		)
	}

	s := m.filtered[m.cursor]

	// Detail section
	var detail strings.Builder
	detail.WriteString(m.styles.Title.Render(s.Name))
	detail.WriteString("\n\n")

	statusColor := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(s.Status)))
	agentColor := lipgloss.NewStyle().
		Foreground(lipgloss.Color(models.AgentColor(s.AgentType))).
		Background(lipgloss.Color(models.AgentBgColor(s.AgentType)))

	detail.WriteString(fmt.Sprintf("  Path     %s\n", m.styles.Muted.Render(s.Path)))
	detail.WriteString(fmt.Sprintf("  Agent    %s\n", agentColor.Render(models.AgentLabel(s.AgentType))))
	detail.WriteString(fmt.Sprintf("  Status   %s\n", statusColor.Render(models.StatusLabel(s.Status))))

	if s.Details.GitBranch != "" {
		detail.WriteString(fmt.Sprintf("  Branch   %s\n", m.styles.Muted.Render(s.Details.GitBranch)))
	}
	if s.Details.Model != "" {
		detail.WriteString(fmt.Sprintf("  Model    %s\n", m.styles.Muted.Render(s.Details.Model)))
	}
	if s.Details.ContextUsage != "" {
		detail.WriteString(fmt.Sprintf("  Context  %s\n", m.styles.Muted.Render(s.Details.ContextUsage)))
	}

	detailHeight := height / 3
	detailStr := m.styles.DetailPanel.Width(width).Height(detailHeight).Render(detail.String())

	// Preview section
	previewHeight := height - detailHeight - 2
	previewContent := m.preview
	if previewContent == "" {
		previewContent = m.styles.Muted.Render("No preview available")
	} else {
		// Trim to fit
		lines := strings.Split(previewContent, "\n")
		if len(lines) > previewHeight-2 {
			lines = lines[len(lines)-previewHeight+2:]
		}
		previewContent = strings.Join(lines, "\n")
	}
	previewStr := m.styles.PreviewPanel.Width(width).Height(previewHeight).Render(previewContent)

	return lipgloss.JoinVertical(lipgloss.Left, detailStr, previewStr)
}
