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
	"github.com/nemke/nagare-go/internal/config"
	"github.com/nemke/nagare-go/internal/log"
	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/state"
	"github.com/nemke/nagare-go/internal/theme"
	"github.com/nemke/nagare-go/internal/tmux"
	"github.com/sahilm/fuzzy"
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
type gridPreviewsMsg map[string]string

// Model is the main Bubble Tea model for the picker TUI.
type Model struct {
	sessions      []models.Session
	filtered      []models.Session
	cursor        int
	viewMode      ViewMode
	sortMode      SortMode
	preview       string
	width         int
	height        int
	statesDir     string
	searchInput   textinput.Model
	showHelp      bool              // F1 help overlay
	showHelpBar   bool              // bottom hint bar
	showThemePick bool              // Ctrl+t theme picker overlay
	themeNames    []string          // cached sorted theme names
	themeCursor   int               // cursor in theme picker
	themeOriginal string            // theme before opening picker (for cancel)
	gridPreviews  map[string]string // cached grid cell previews keyed by pane target
	registry      *state.Registry
	renameMode    bool
	renameSession models.Session
}

// New creates a new picker model with default settings.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "search sessions..."
	ti.Prompt = " > "
	ti.CharLimit = 64

	ti.Focus()

	cfg, _ := config.Load()

	return Model{
		statesDir:   state.DefaultStatesDir(),
		searchInput: ti,
		showHelpBar: cfg.Picker.ShowHelpBar,
		registry:    state.NewRegistry(state.DefaultRegistryPath()),
	}
}

// markDead writes a dead state for a session before killing it.
func markDead(s models.Session, statesDir string) {
	state.WriteState(statesDir, models.SessionState{
		State:     "dead",
		SessionID: s.SessionID,
		Cwd:       s.Path,
		Event:     "ManualKill",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// selectedSession returns the currently selected session, if any.
func (m Model) selectedSession() (models.Session, bool) {
	if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		return models.Session{}, false
	}
	return m.filtered[m.cursor], true
}

// isStarred returns whether a session is starred in the registry.
func (m Model) isStarred(name string) bool {
	s := m.registry.Find(name)
	return s != nil && s.Starred
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
		log.Debug("scan: %d sessions", len(m.sessions))
		m.applyFilter()
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return tickScanMsg{} })

	case PreviewUpdatedMsg:
		m.preview = string(msg)
		return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return tickPreviewMsg{} })

	case gridPreviewsMsg:
		m.gridPreviews = map[string]string(msg)
		return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return tickPreviewMsg{} })

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

	// Render base content
	contentHeight := m.height
	if m.showHelpBar {
		contentHeight = m.height - 1
	}

	var base string
	if m.viewMode == GridView {
		base = m.viewGrid(m.width, contentHeight)
	} else {
		leftOuter := m.width / 5
		rightOuter := m.width - leftOuter
		left := m.viewLeft(leftOuter, contentHeight)
		right := m.viewRight(rightOuter, contentHeight)
		base = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}

	if m.showHelpBar {
		base = base + "\n" + helpBar(m.width)
	}

	// Overlays drawn on top of base content
	if m.showHelp {
		overlay := helpOverlay(m.width, m.height)
		return placeOverlay(m.width, m.height, overlay, base)
	}
	if m.showThemePick {
		overlay := themePickOverlay(m.themeNames, m.themeCursor, m.width, m.height)
		return placeOverlay(m.width, m.height, overlay, base)
	}

	return base
}

// --- Key handling ---

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	log.Debug("key: %q type=%d", key, msg.Type)

	// Theme picker intercepts all keys when open
	if m.showThemePick {
		return m.handleThemePickKey(key)
	}

	// Rename mode intercepts keys before normal handling
	if m.renameMode {
		return m.handleRenameKey(msg)
	}

	switch key {
	case keyHelp:
		m.showHelp = !m.showHelp
		return m, nil
	case keyEscape:
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		return m, tea.Quit
	case keyEnter:
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			target := fmt.Sprintf("%s:%d", s.Name, s.WindowIndex)
			tmux.RunTmux("switch-client", "-t", target)
			return m, tea.Quit
		}
	case keyUp:
		if m.viewMode == GridView {
			cols := gridColumns(len(m.filtered))
			if m.cursor-cols >= 0 {
				m.cursor -= cols
			}
		} else if m.cursor > 0 {
			m.cursor--
		}
		return m, m.doPreview()
	case keyDown:
		if m.viewMode == GridView {
			cols := gridColumns(len(m.filtered))
			if m.cursor+cols < len(m.filtered) {
				m.cursor += cols
			}
		} else if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
		return m, m.doPreview()
	case "left":
		if m.viewMode == GridView && m.cursor > 0 {
			m.cursor--
		}
		return m, m.doPreview()
	case "right":
		if m.viewMode == GridView && m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
		return m, m.doPreview()
	case keyToggleView:
		if m.viewMode == ListView {
			m.viewMode = GridView
			log.Info("switched to grid view")
		} else {
			m.viewMode = ListView
			log.Info("switched to list view")
		}
		return m, nil
	case keyCycleTheme:
		m.showThemePick = true
		m.themeNames = theme.Names()
		m.themeOriginal = theme.Current().Name
		// Set cursor to current theme
		for i, name := range m.themeNames {
			if name == m.themeOriginal {
				m.themeCursor = i
				break
			}
		}
		return m, nil
	case keyApprove:
		if s, ok := m.selectedSession(); ok && s.Status == models.StatusWaitingInput {
			tmux.RunTmux("send-keys", "-t", tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex), "Enter")
			log.Info("approved %s", s.Name)
		}
		return m, nil
	case keyApproveAlways:
		if s, ok := m.selectedSession(); ok && s.Status == models.StatusWaitingInput {
			tmux.RunTmux("send-keys", "-t", tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex), "Down", "Enter")
			log.Info("approved always %s", s.Name)
		}
		return m, nil
	case keyUnload:
		if s, ok := m.selectedSession(); ok {
			markDead(s, m.statesDir)
			tmux.RunTmux("kill-pane", "-t", tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex))
			log.Info("unloaded pane %s", s.Name)
			return m, doScan(m.statesDir)
		}
		return m, nil
	case keyKillSession:
		if s, ok := m.selectedSession(); ok {
			markDead(s, m.statesDir)
			tmux.RunTmux("kill-session", "-t", s.Name)
			log.Info("killed session %s", s.Name)
			return m, doScan(m.statesDir)
		}
		return m, nil
	case keyStar:
		if s, ok := m.selectedSession(); ok {
			// Auto-register if not in registry
			if m.registry.Find(s.Name) == nil {
				m.registry.Register(s.Name, s.Path, string(s.AgentType))
			}
			starred := m.registry.ToggleStar(s.Name)
			if starred {
				log.Info("starred %s", s.Name)
			} else {
				log.Info("unstarred %s", s.Name)
			}
			// Refresh registry after toggle
			m.registry = state.NewRegistry(state.DefaultRegistryPath())
		}
		return m, nil
	case keyCycleSort:
		switch m.sortMode {
		case SortByStatus:
			m.sortMode = SortByName
		case SortByName:
			m.sortMode = SortByAgent
		case SortByAgent:
			m.sortMode = SortByStatus
		}
		m.applyFilter()
		log.Info("sort mode: %d", m.sortMode)
		return m, nil
	case keyRename:
		if s, ok := m.selectedSession(); ok {
			m.renameMode = true
			m.renameSession = s
			m.searchInput.SetValue(s.Name)
			m.searchInput.CursorEnd()
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

// handleRenameKey handles key input during rename mode.
func (m Model) handleRenameKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case keyEscape:
		// Cancel rename
		m.renameMode = false
		m.searchInput.SetValue("")
		return m, nil
	case keyEnter:
		// Finish rename
		newName := strings.TrimSpace(m.searchInput.Value())
		m.renameMode = false
		m.searchInput.SetValue("")

		if newName == "" || newName == m.renameSession.Name {
			return m, nil
		}

		oldName := m.renameSession.Name

		// Check if name already exists
		existing := tmux.RunTmux("list-sessions", "-F", "#{session_name}")
		for _, line := range strings.Split(existing, "\n") {
			if strings.TrimSpace(line) == newName {
				log.Info("rename failed: %s already exists", newName)
				return m, nil
			}
		}

		// Count sessions with same name (multi-agent check)
		count := 0
		for _, s := range m.sessions {
			if s.Name == oldName {
				count++
			}
		}

		if count > 1 {
			// Rename just the window
			target := fmt.Sprintf("%s:%d", oldName, m.renameSession.WindowIndex)
			tmux.RunTmux("rename-window", "-t", target, newName)
			log.Info("renamed window %s -> %s", target, newName)
		} else {
			// Rename the tmux session
			tmux.RunTmux("rename-session", "-t", oldName, newName)
			log.Info("renamed session %s -> %s", oldName, newName)

			// Update registry
			if existing := m.registry.Find(oldName); existing != nil {
				path := existing.Path
				agent := existing.Agent
				m.registry.Remove(oldName)
				m.registry.Register(newName, path, agent)
			}
		}

		return m, doScan(m.statesDir)
	default:
		// Forward to search input for text editing
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
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

	// Grid view: capture all visible sessions in background
	if m.viewMode == GridView {
		sessions := make([]models.Session, len(m.filtered))
		copy(sessions, m.filtered)
		return func() tea.Msg {
			previews := make(map[string]string)
			for _, s := range sessions {
				target := tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex)
				previews[target] = CapturePreview(s.Name, s.WindowIndex, s.PaneIndex)
			}
			return gridPreviewsMsg(previews)
		}
	}

	// List view: capture only the selected session
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
	// Pre-build starred set to avoid per-comparison registry lookups
	starred := make(map[string]bool)
	for _, s := range m.filtered {
		starred[s.Name] = m.isStarred(s.Name)
	}

	sort.SliceStable(m.filtered, func(i, j int) bool {
		si := starred[m.filtered[i].Name]
		sj := starred[m.filtered[j].Name]
		if si != sj {
			return si
		}
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
	if m.renameMode {
		m.searchInput.Prompt = " Rename: "
	} else {
		m.searchInput.Prompt = " > "
	}
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

		star := ""
		if m.isStarred(s.Name) {
			star = " ★"
		}

		var line string
		if i == m.cursor {
			agentChar := string(models.AgentLabel(s.AgentType)[0])
			line = lipgloss.NewStyle().
				Background(c.Primary).
				Foreground(c.Background).
				Bold(true).
				PaddingLeft(1).
				Width(width).
				Render(fmt.Sprintf("> ● %s  %s%s", name, agentChar, star))
		} else {
			nameStyled := lipgloss.NewStyle().Foreground(c.Foreground).Render(name)
			starStyled := lipgloss.NewStyle().Foreground(c.Warning).Render(star)
			content := fmt.Sprintf(" %s %s %s%s", dot, nameStyled, badge, starStyled)
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
		// Border = 2 lines, padding = 0 vertical for preview panel
		maxLines := previewHeight - 2
		if maxLines < 1 {
			maxLines = 1
		}
		lines := strings.Split(previewContent, "\n")
		// Trim trailing empty lines
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		// Take the bottom portion
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

// --- Grid view ---

func gridColumns(count int) int {
	if count <= 2 {
		return 1
	}
	if count <= 4 {
		return 2
	}
	return 3
}

func (m Model) viewGrid(totalWidth, totalHeight int) string {
	c := theme.Current().Colors

	if len(m.filtered) == 0 {
		return panelStyle().
			Width(totalWidth).
			Height(totalHeight).
			Render(mutedStyle().Render("No sessions found"))
	}

	// Search bar at top (1 line + 1 blank line = 2 lines)
	searchBar := m.searchInput.View()

	cols := gridColumns(len(m.filtered))
	cellWidth := totalWidth / cols
	numRows := (len(m.filtered) + cols - 1) / cols
	cellHeight := (totalHeight - 2 - numRows) / numRows // search + row gaps
	if cellHeight < 8 {
		cellHeight = 8
	}

	// Build rows of cells
	var rows []string
	for i := 0; i < len(m.filtered); i += cols {
		var cells []string
		for j := 0; j < cols && i+j < len(m.filtered); j++ {
			idx := i + j
			s := m.filtered[idx]

			// Header: status dot + name + agent badge
			dot := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(s.Status))).Render("●")
			statusLabel := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(s.Status))).Render(models.StatusLabel(s.Status))
			agentBadge := lipgloss.NewStyle().
				Foreground(lipgloss.Color(models.AgentColor(s.AgentType))).
				Background(lipgloss.Color(models.AgentBgColor(s.AgentType))).
				Padding(0, 1).
				Render(models.AgentLabel(s.AgentType))

			header := fmt.Sprintf(" %s %s %s  %s", dot, s.Name, agentBadge, statusLabel)

			// Meta line: path + git branch
			meta := mutedStyle().Render(fmt.Sprintf("   %s", s.Path))
			if s.Details.GitBranch != "" {
				meta += mutedStyle().Render(fmt.Sprintf("  (%s)", s.Details.GitBranch))
			}

			// Separator between header and preview
			innerWidth := cellWidth - 6 // borders + padding
			if innerWidth < 10 {
				innerWidth = 10
			}
			separator := lipgloss.NewStyle().Foreground(c.Border).Render(strings.Repeat("─", innerWidth))

			// Preview: capture pane content for this session
			previewHeight := cellHeight - 7
			if previewHeight < 1 {
				previewHeight = 1
			}

			preview := m.getGridPreview(s, innerWidth, previewHeight)

			content := header + "\n" + meta + "\n" + separator + "\n" + preview

			// Border color: bright for selected, muted for others
			borderColor := c.Border
			if idx == m.cursor {
				borderColor = c.Primary
			}

			cell := lipgloss.NewStyle().
				Background(c.Background).
				Foreground(c.Foreground).
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(borderColor).
				BorderBackground(c.Background).
				Width(cellWidth).
				Height(cellHeight).
				Padding(1).
				Render(content)

			cells = append(cells, cell)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}

	grid := strings.Join(rows, "\n")
	return " " + searchBar + "\n" + grid
}

func (m Model) getGridPreview(s models.Session, width, height int) string {
	target := tmux.PaneTarget(s.Name, s.WindowIndex, s.PaneIndex)
	content := m.gridPreviews[target]
	if content == "" {
		return mutedStyle().Render("Loading...")
	}

	lines := strings.Split(content, "\n")
	// Trim leading blank lines
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		if ansi.StringWidth(line) > width {
			lines[i] = ansi.Truncate(line, width, "")
		}
	}
	return strings.Join(lines, "\n")
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
