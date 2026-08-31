// Package board implements the local, cross-project ticket board.
package board

import (
	"fmt"
	"image/color"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nemke/nagare-go/internal/git"
	"github.com/nemke/nagare-go/internal/mcp"
	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/session"
	"github.com/nemke/nagare-go/internal/state"
	"github.com/nemke/nagare-go/internal/theme"
	"github.com/nemke/nagare-go/internal/tickets"
	"github.com/nemke/nagare-go/internal/tmux"
)

const (
	ActionNone = ""
	ActionNew  = "new"
	ActionEdit = "edit"
)

// Result describes an action that main must run outside the board program.
type Result struct {
	Action   string
	TicketID string
}

type tickMsg struct {
	epoch uint64
}

type refreshMsg struct {
	epoch           uint64
	tickets         []tickets.Ticket
	sessions        []models.Session
	err             error
	scannedSessions bool
}

// Model is the ticket board Bubble Tea model.
type Model struct {
	store            *tickets.Store
	tickets          []tickets.Ticket
	sessions         []models.Session
	column           int
	cursors          map[tickets.Status]int
	width            int
	height           int
	todayOnly        bool
	delegateMode     bool
	delegateSessions []models.Session
	delegateCursor   int
	agentsMode       bool
	availableAgents  []models.Session
	agentsCursor     int
	deleteMode       bool
	deleteTicket     tickets.Ticket
	statusNote       string
	statusErr        string
	result           Result
	manageSessions   bool
	active           bool
	refreshEpoch     uint64
}

// New creates a standalone ticket board.
func New(store *tickets.Store) Model {
	model := NewDeferred(store)
	model.manageSessions = true
	model.active = true
	model.refreshEpoch = 1
	return model
}

// NewDeferred creates an unloaded board for embedding in another view.
func NewDeferred(store *tickets.Store) Model {
	return Model{
		store:     store,
		cursors:   make(map[tickets.Status]int),
		todayOnly: true,
	}
}

// Activate refreshes a deferred board and starts its update ticker.
func (m *Model) Activate() tea.Cmd {
	m.active = true
	m.refreshEpoch++
	return m.refresh()
}

// Deactivate stops a deferred board's update ticker after its current tick.
func (m *Model) Deactivate() {
	m.active = false
	m.refreshEpoch++
}

// SetSessions supplies live sessions from the embedding picker, avoiding a
// duplicate tmux and git scan inside the board.
func (m *Model) SetSessions(sessions []models.Session) {
	m.sessions = sessions
}

// Result returns the action selected before the board exited.
func (m Model) Result() Result { return m.result }

func (m Model) Init() tea.Cmd {
	if !m.active {
		return nil
	}
	refresh := m.refresh()
	if m.manageSessions {
		return tea.Batch(tea.RequestBackgroundColor, refresh)
	}
	return refresh
}
func tick(epoch uint64) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{epoch: epoch} })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		theme.SetDarkBackground(msg.IsDark())
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		if !m.active || msg.epoch != m.refreshEpoch {
			return m, nil
		}
		return m, m.refresh()
	case refreshMsg:
		if msg.epoch != m.refreshEpoch {
			return m, nil
		}
		if msg.err != nil {
			m.statusErr = msg.err.Error()
		} else {
			m.tickets = msg.tickets
		}
		if msg.scannedSessions {
			m.sessions = msg.sessions
		}
		m.clampCursors()
		if !m.active {
			return m, nil
		}
		return m, tick(m.refreshEpoch)
	case tea.KeyMsg:
		if m.deleteMode {
			return m.handleDeleteKey(msg)
		}
		if m.delegateMode {
			return m.handleDelegateKey(msg)
		}
		if m.agentsMode {
			return m.handleAgentsKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	m.statusErr = ""
	m.statusNote = ""

	switch key {
	case "q", "esc":
		return m, tea.Quit
	case "left", "h":
		if m.column > 0 {
			m.column--
		}
	case "right", "l":
		if m.column < len(tickets.BoardStatuses)-1 {
			m.column++
		}
	case "up", "k":
		status := m.currentStatus()
		if m.cursors[status] > 0 {
			m.cursors[status]--
		}
	case "down", "j":
		status := m.currentStatus()
		if m.cursors[status] < len(m.columnTickets(status))-1 {
			m.cursors[status]++
		}
	case "n":
		m.result = Result{Action: ActionNew}
		return m, tea.Quit
	case "e":
		if ticket, ok := m.selectedTicket(); ok {
			m.result = Result{Action: ActionEdit, TicketID: ticket.ID}
			return m, tea.Quit
		}
	case "t":
		m.todayOnly = !m.todayOnly
		m.clampCursors()
	case "[":
		m.moveSelected(-1)
	case "]":
		m.moveSelected(1)
	case "x":
		m.startDelete()
	case "d":
		m.startDelegation()
	case "a":
		m.showAvailableAgents()
	case "enter":
		if ticket, ok := m.selectedTicket(); ok && ticket.AssigneeSession != "" {
			if assigned, found := m.assignedSession(ticket); found {
				session.SwitchToPane(assigned)
				return m, tea.Quit
			}
			m.statusErr = "assigned agent is no longer running"
		}
	}
	return m, nil
}

func (m Model) handleDeleteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		m.confirmDelete()
	case "n", "esc", "q", "x":
		m.deleteMode = false
		m.deleteTicket = tickets.Ticket{}
	}
	return m, nil
}

func (m Model) handleDelegateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.delegateMode = false
		m.delegateSessions = nil
		m.delegateCursor = 0
	case "up", "k":
		if m.delegateCursor > 0 {
			m.delegateCursor--
		}
	case "down", "j":
		if m.delegateCursor < len(m.delegateSessions)-1 {
			m.delegateCursor++
		}
	case "enter":
		m.delegateSelected()
	}
	return m, nil
}

func (m Model) handleAgentsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "a":
		m.agentsMode = false
		m.availableAgents = nil
		m.agentsCursor = 0
	case "up", "k":
		if m.agentsCursor > 0 {
			m.agentsCursor--
		}
	case "down", "j":
		if m.agentsCursor < len(m.availableAgents)-1 {
			m.agentsCursor++
		}
	case "enter":
		if m.agentsCursor >= 0 && m.agentsCursor < len(m.availableAgents) {
			session.SwitchToPane(m.availableAgents[m.agentsCursor])
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) refresh() tea.Cmd {
	store := m.store
	epoch := m.refreshEpoch
	scanSessions := m.manageSessions
	return func() tea.Msg {
		loaded, err := store.List()
		var sessions []models.Session
		if scanSessions {
			sessions = scanAgentSessions()
		}
		return refreshMsg{
			epoch:           epoch,
			tickets:         loaded,
			sessions:        sessions,
			err:             err,
			scannedSessions: scanSessions,
		}
	}
}

func (m *Model) reload() {
	loaded, err := m.store.List()
	if err != nil {
		m.statusErr = err.Error()
	} else {
		m.tickets = loaded
	}
	if m.manageSessions {
		m.sessions = scanAgentSessions()
	}
	m.clampCursors()
}

func scanAgentSessions() []models.Session {
	statesDir := state.DefaultStatesDir()
	return tmux.ScanSessions(state.LoadStatesByPaneID(statesDir), state.LoadAllStates(statesDir))
}

func (m Model) currentStatus() tickets.Status {
	return tickets.BoardStatuses[m.column]
}

func (m Model) selectedTicket() (tickets.Ticket, bool) {
	status := m.currentStatus()
	items := m.columnTickets(status)
	cursor := m.cursors[status]
	if cursor < 0 || cursor >= len(items) {
		return tickets.Ticket{}, false
	}
	return items[cursor], true
}

func (m Model) columnTickets(status tickets.Status) []tickets.Ticket {
	result := make([]tickets.Ticket, 0)
	for _, ticket := range m.tickets {
		if ticket.Status == status && m.visible(ticket) {
			result = append(result, ticket)
		}
	}
	return result
}

func (m Model) visible(ticket tickets.Ticket) bool {
	if ticket.Status == tickets.StatusCanceled {
		return false
	}
	if !m.todayOnly {
		return true
	}
	today := time.Now().Format(time.DateOnly)
	if ticket.PlannedFor == today || ticket.Status == tickets.StatusRunning || ticket.Status == tickets.StatusReview {
		return true
	}
	return ticket.Status == tickets.StatusDone && ticket.CompletedAt != nil && ticket.CompletedAt.Local().Format(time.DateOnly) == today
}

func (m *Model) clampCursors() {
	for _, status := range tickets.BoardStatuses {
		count := len(m.columnTickets(status))
		if count == 0 {
			m.cursors[status] = 0
		} else if m.cursors[status] >= count {
			m.cursors[status] = count - 1
		}
	}
}

func (m *Model) moveSelected(delta int) {
	ticket, ok := m.selectedTicket()
	if !ok {
		return
	}
	next := m.column + delta
	if next < 0 || next >= len(tickets.BoardStatuses) {
		return
	}
	status := tickets.BoardStatuses[next]
	_, err := m.store.Update(ticket.ID, func(current *tickets.Ticket) error {
		current.Status = status
		if status == tickets.StatusReady && current.PlannedFor == "" {
			current.PlannedFor = time.Now().Format(time.DateOnly)
		}
		if status == tickets.StatusDone {
			now := time.Now().UTC()
			current.CompletedAt = &now
		} else {
			current.CompletedAt = nil
		}
		return nil
	})
	if err != nil {
		m.statusErr = err.Error()
		return
	}
	m.column = next
	m.reload()
}

func (m *Model) startDelete() {
	ticket, ok := m.selectedTicket()
	if !ok {
		return
	}
	m.deleteMode = true
	m.deleteTicket = ticket
}

func (m *Model) confirmDelete() {
	ticket := m.deleteTicket
	m.deleteMode = false
	m.deleteTicket = tickets.Ticket{}
	if err := m.store.Delete(ticket.ID); err != nil {
		m.statusErr = err.Error()
		return
	}
	for index := range m.tickets {
		if m.tickets[index].ID == ticket.ID {
			m.tickets = append(m.tickets[:index], m.tickets[index+1:]...)
			break
		}
	}
	m.statusNote = fmt.Sprintf("deleted %q", ticket.Title)
	m.clampCursors()
}

func (m *Model) startDelegation() {
	ticket, ok := m.selectedTicket()
	if !ok {
		return
	}
	projectRoot := git.MainRoot(ticket.ProjectPath)
	for _, candidate := range m.sessions {
		if candidate.Status != models.StatusIdle {
			continue
		}
		if projectRoot != "" && git.MainRoot(candidate.Path) != projectRoot {
			continue
		}
		m.delegateSessions = append(m.delegateSessions, candidate)
	}
	sort.SliceStable(m.delegateSessions, func(i, j int) bool {
		return m.delegateSessions[i].Name < m.delegateSessions[j].Name
	})
	if len(m.delegateSessions) == 0 {
		m.statusErr = "no idle agent is available for this project"
		return
	}
	m.delegateMode = true
	m.delegateCursor = 0
}

func (m *Model) showAvailableAgents() {
	m.availableAgents = idleAgents(m.sessions)
	if len(m.availableAgents) == 0 {
		m.statusErr = "no idle agents are available"
		return
	}
	m.agentsMode = true
	m.agentsCursor = 0
}

func idleAgents(sessions []models.Session) []models.Session {
	available := make([]models.Session, 0, len(sessions))
	for _, candidate := range sessions {
		if candidate.Status == models.StatusIdle {
			available = append(available, candidate)
		}
	}
	sort.SliceStable(available, func(i, j int) bool {
		return available[i].Name < available[j].Name
	})
	return available
}

func (m *Model) delegateSelected() {
	if m.delegateCursor < 0 || m.delegateCursor >= len(m.delegateSessions) {
		return
	}
	ticket, ok := m.selectedTicket()
	if !ok {
		m.delegateMode = false
		return
	}
	target := m.delegateSessions[m.delegateCursor]
	if err := assignTicket(m.store, ticket, target); err != nil {
		m.statusErr = err.Error()
	} else {
		m.statusNote = "delegated to " + target.Name
	}
	m.delegateMode = false
	m.delegateSessions = nil
	m.delegateCursor = 0
	m.column = statusIndex(tickets.StatusRunning)
	m.reload()
}

func assignTicket(store *tickets.Store, ticket tickets.Ticket, target models.Session) error {
	previous := ticket
	projectPath := git.MainRoot(target.Path)
	if projectPath == "" {
		projectPath = target.Path
	}
	_, err := store.Update(ticket.ID, func(current *tickets.Ticket) error {
		current.Status = tickets.StatusRunning
		current.PlannedFor = time.Now().Format(time.DateOnly)
		current.ProjectPath = projectPath
		current.AssigneeSession = target.Name
		current.AssigneePaneID = target.PaneID
		current.AssigneeAgent = string(target.AgentType)
		current.SubmittedSummary = ""
		return nil
	})
	if err != nil {
		return err
	}

	response := mcp.SendMessageHandler("nagare-board", mcp.SendMessageInput{
		Target:  target.Name,
		Message: assignmentPrompt(ticket),
	})
	if !strings.HasPrefix(response, "Error") {
		return nil
	}
	_, _ = store.Update(ticket.ID, func(current *tickets.Ticket) error {
		*current = previous
		return nil
	})
	return fmt.Errorf("%s", response)
}

func assignmentPrompt(ticket tickets.Ticket) string {
	var description string
	if ticket.Description != "" {
		description = "\n\nDescription and acceptance criteria:\n" + ticket.Description
	}
	return fmt.Sprintf("You have been assigned Nagare ticket %s: %s%s\n\nWork in the current project. When the requested outcome is implemented and verified, call submit_ticket with ticket_id %q and a concise summary of the result and verification. Do not mark the ticket done; it requires human review.", ticket.ID, ticket.Title, description, ticket.ID)
}

func statusIndex(status tickets.Status) int {
	for i, candidate := range tickets.BoardStatuses {
		if candidate == status {
			return i
		}
	}
	return 0
}

func (m Model) assignedSession(ticket tickets.Ticket) (models.Session, bool) {
	for _, candidate := range m.sessions {
		if ticket.AssigneePaneID != "" && candidate.PaneID == ticket.AssigneePaneID {
			return candidate, true
		}
		if ticket.AssigneePaneID == "" && candidate.SessionName == ticket.AssigneeSession {
			return candidate, true
		}
	}
	return models.Session{}, false
}

func (m Model) View() tea.View {
	content := m.view()
	if m.width > 0 && m.height > 0 {
		content = lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(content)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func (m Model) view() string {
	if m.width == 0 {
		return "Loading..."
	}
	colors := theme.Current().Colors
	header := m.renderHeader()
	boardHeight := max(7, m.height-3)
	columns := m.renderColumns(boardHeight)
	footer := m.renderFooter()
	if m.statusErr != "" {
		footer = lipgloss.NewStyle().Foreground(colors.Error).Bold(true).
			Render(ansi.Truncate("✕  "+m.statusErr, m.width, ""))
	} else if m.statusNote != "" {
		footer = lipgloss.NewStyle().Foreground(colors.Success).Bold(true).
			Render(ansi.Truncate("✓  "+m.statusNote, m.width, ""))
	}
	if m.delegateMode {
		columns = m.renderDelegateDialog()
	}
	if m.agentsMode {
		columns = m.renderAgentsDialog("Available agents", m.availableAgents, m.agentsCursor, "j/k choose  enter open  a/esc close")
	}
	if m.deleteMode {
		columns = m.renderDeleteDialog()
	}
	return lipgloss.NewStyle().
		Background(colors.Background).
		Foreground(colors.Foreground).
		Height(m.height).
		Render(header + "\n\n" + columns + "\n" + footer)
}

func (m Model) renderHeader() string {
	colors := theme.Current().Colors
	brand := lipgloss.NewStyle().Foreground(colors.Primary).Bold(true).Render("NAGARE")
	views := lipgloss.NewStyle().Foreground(colors.Muted).Render("‹ LIST") + "  " +
		lipgloss.NewStyle().Foreground(colors.Background).Background(colors.Primary).Bold(true).Padding(0, 1).Render("BOARD") + "  " +
		lipgloss.NewStyle().Foreground(colors.Muted).Render("GRID ›")
	filter := "TODAY"
	if !m.todayOnly {
		filter = "ALL"
	}
	filterPill := lipgloss.NewStyle().Foreground(colors.Background).Background(colors.Accent).Bold(true).Padding(0, 1).Render(filter)
	count := 0
	for _, ticket := range m.tickets {
		if m.visible(ticket) {
			count++
		}
	}
	noun := "tickets"
	if count == 1 {
		noun = "ticket"
	}
	left := brand + "  " + views
	right := filterPill + "  " + lipgloss.NewStyle().Foreground(colors.Subtle).Render(fmt.Sprintf("%d %s", count, noun))
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	return ansi.Truncate(left+strings.Repeat(" ", gap)+right, m.width, "")
}

func (m Model) renderFooter() string {
	colors := theme.Current().Colors
	key := func(binding, action string) string {
		return lipgloss.NewStyle().Foreground(colors.Accent).Bold(true).Render(binding) + " " +
			lipgloss.NewStyle().Foreground(colors.Muted).Render(action)
	}
	hints := []string{
		key("Tab/⇧Tab", "views"),
		key("h/l", "lane"),
		key("j/k", "card"),
		key("[/]", "move"),
		key("n", "new"),
		key("x", "delete"),
		key("d", "delegate"),
		key("a", "agents"),
		key("t", "today"),
		key("q", "quit"),
	}
	return ansi.Truncate(strings.Join(hints, lipgloss.NewStyle().Foreground(colors.Border).Render("  │  ")), m.width, "")
}

func (m Model) renderColumns(height int) string {
	visibleCount := max(1, min(len(tickets.BoardStatuses), m.width/24))
	start := m.column - visibleCount/2
	if start < 0 {
		start = 0
	}
	if start+visibleCount > len(tickets.BoardStatuses) {
		start = len(tickets.BoardStatuses) - visibleCount
	}
	gap := 1
	columnWidth := max(21, (m.width-gap*(visibleCount-1))/visibleCount)
	parts := make([]string, 0, visibleCount*2-1)
	for index := start; index < start+visibleCount; index++ {
		parts = append(parts, m.renderColumn(index, columnWidth, height))
		if index < start+visibleCount-1 {
			parts = append(parts, strings.Repeat(" ", gap))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m Model) renderColumn(index, width, height int) string {
	colors := theme.Current().Colors
	status := tickets.BoardStatuses[index]
	items := m.columnTickets(status)
	focused := index == m.column
	contentWidth := max(12, width-4)
	contentHeight := max(5, height-2)

	laneBackground := colors.Surface
	laneFill := lipgloss.NewStyle().Background(laneBackground)
	statusStyle := lipgloss.NewStyle().Foreground(statusColor(status)).Background(laneBackground).Bold(true)
	title := statusStyle.Render(statusIcon(status) + " " + tickets.StatusLabel(status))
	countStyle := lipgloss.NewStyle().Foreground(colors.Muted).Background(colors.Overlay).Padding(0, 1)
	if focused {
		countStyle = countStyle.Foreground(colors.Background).Background(statusColor(status)).Bold(true)
	}
	count := countStyle.Render(fmt.Sprintf("%d", len(items)))
	gap := max(1, contentWidth-lipgloss.Width(title)-lipgloss.Width(count))
	content := title + laneFill.Render(strings.Repeat(" ", gap)) + count
	content += "\n" + lipgloss.NewStyle().
		Foreground(colors.Border).
		Background(laneBackground).
		Render(strings.Repeat("─", contentWidth))

	available := max(1, (contentHeight-2)/5)
	cursor := m.cursors[status]
	start := 0
	if cursor >= available {
		start = cursor - available + 1
	}
	for itemIndex := start; itemIndex < len(items) && itemIndex < start+available; itemIndex++ {
		content += "\n" + m.renderCard(items[itemIndex], contentWidth, focused && itemIndex == cursor)
	}
	if len(items) == 0 {
		empty := lipgloss.NewStyle().
			Width(contentWidth).
			Align(lipgloss.Center).
			Foreground(colors.Muted).
			Background(laneBackground).
			Italic(true).
			Render(emptyColumnCopy(status))
		content += "\n\n" + empty
	}

	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(colors.Surface).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colors.Border).
		Padding(0, 1)
	if focused {
		style = style.BorderForeground(colors.BorderFocus)
	}
	return style.Render(content)
}

func statusIcon(status tickets.Status) string {
	switch status {
	case tickets.StatusBacklog:
		return "○"
	case tickets.StatusReady:
		return "◆"
	case tickets.StatusRunning:
		return "▶"
	case tickets.StatusReview:
		return "◇"
	case tickets.StatusDone:
		return "✓"
	default:
		return "·"
	}
}

func statusColor(status tickets.Status) color.Color {
	colors := theme.Current().Colors
	switch status {
	case tickets.StatusBacklog:
		return colors.Muted
	case tickets.StatusReady:
		return colors.Accent
	case tickets.StatusRunning:
		return colors.Warning
	case tickets.StatusReview:
		return colors.Secondary
	case tickets.StatusDone:
		return colors.Success
	default:
		return colors.Subtle
	}
}

func emptyColumnCopy(status tickets.Status) string {
	switch status {
	case tickets.StatusBacklog:
		return "Nothing queued"
	case tickets.StatusReady:
		return "Nothing ready"
	case tickets.StatusRunning:
		return "No agent working"
	case tickets.StatusReview:
		return "Nothing to review"
	case tickets.StatusDone:
		return "Nothing finished"
	default:
		return "No tickets"
	}
}

func (m Model) renderCard(ticket tickets.Ticket, width int, selected bool) string {
	colors := theme.Current().Colors
	bodyWidth := max(8, width-4)
	project := "General"
	if ticket.ProjectPath != "" {
		project = filepath.Base(ticket.ProjectPath)
	}
	assignee := "Unassigned"
	if ticket.AssigneeSession != "" {
		assignee = ticket.AssigneeSession
		if assigned, ok := m.assignedSession(ticket); ok {
			assignee += " · " + models.StatusLabel(assigned.Status)
		}
	}

	marker := "  "
	if selected {
		marker = "› "
	}
	title := marker + ticket.Title
	meta := "⌂ " + project + "  ·  @" + assignee
	shortID := ticket.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	cardBackground := colors.Overlay
	fill := lipgloss.NewStyle().Background(cardBackground)
	priority := priorityLabel(ticket.Priority, cardBackground)
	priorityWidth := lipgloss.Width(priority)
	bottom := fill.Render(strings.Repeat(" ", max(0, bodyWidth-priorityWidth))) + priority
	if len(shortID)+1+priorityWidth <= bodyWidth {
		bottomGap := bodyWidth - len(shortID) - priorityWidth
		bottom = lipgloss.NewStyle().
			Foreground(colors.Muted).
			Background(cardBackground).
			Render(shortID) +
			fill.Render(strings.Repeat(" ", bottomGap)) +
			priority
	}

	style := lipgloss.NewStyle().
		Width(width).
		Background(cardBackground).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colors.Border).
		Padding(0, 1)
	titleStyle := lipgloss.NewStyle().
		Width(bodyWidth).
		Foreground(colors.Foreground).
		Background(cardBackground)
	if selected {
		style = style.BorderForeground(colors.BorderFocus)
		titleStyle = titleStyle.Foreground(colors.Primary).Bold(true)
	}
	metaStyle := lipgloss.NewStyle().
		Width(bodyWidth).
		Foreground(colors.Subtle).
		Background(cardBackground)
	body := titleStyle.Render(ansi.Truncate(title, bodyWidth, "…")) + "\n" +
		metaStyle.Render(ansi.Truncate(meta, bodyWidth, "…")) + "\n" +
		bottom
	return style.Render(body)
}

func priorityLabel(priority tickets.Priority, background color.Color) string {
	colors := theme.Current().Colors
	style := lipgloss.NewStyle().Bold(true).Background(background)
	switch priority {
	case tickets.PriorityUrgent:
		return style.Foreground(colors.Error).Render("!! URGENT")
	case tickets.PriorityHigh:
		return style.Foreground(colors.Warning).Render("! HIGH")
	case tickets.PriorityLow:
		return style.Foreground(colors.Muted).Render("· LOW")
	default:
		return style.Foreground(colors.Accent).Render("• MEDIUM")
	}
}

func (m Model) renderDeleteDialog() string {
	colors := theme.Current().Colors
	outerWidth := min(min(max(40, m.width/2), 68), max(20, m.width-4))
	innerWidth := max(16, outerWidth-6)
	title := lipgloss.NewStyle().Foreground(colors.Error).Bold(true).Render("Delete ticket?")
	name := lipgloss.NewStyle().Foreground(colors.Foreground).Bold(true).
		Render(ansi.Truncate(m.deleteTicket.Title, innerWidth, "…"))
	warning := lipgloss.NewStyle().Foreground(colors.Muted).
		Render("This permanently removes the ticket.")
	hint := lipgloss.NewStyle().Foreground(colors.Error).Bold(true).Render("y/enter delete") + "  " +
		lipgloss.NewStyle().Foreground(colors.Muted).Render("n/esc cancel")
	body := strings.Join([]string{title, "", name, warning, "", hint}, "\n")
	box := lipgloss.NewStyle().
		Width(outerWidth).
		Background(colors.Overlay).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colors.Error).
		Padding(1, 2).
		Render(body)
	return lipgloss.Place(m.width, max(8, m.height-4), lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderDelegateDialog() string {
	return m.renderAgentsDialog("Delegate ticket", m.delegateSessions, m.delegateCursor, "j/k choose  enter delegate  esc cancel")
}

func (m Model) renderAgentsDialog(title string, agents []models.Session, cursor int, hint string) string {
	colors := theme.Current().Colors
	outerWidth := min(max(40, m.width/2), 76)
	innerWidth := max(20, outerWidth-6)
	var body strings.Builder
	body.WriteString(lipgloss.NewStyle().Foreground(colors.Primary).Bold(true).Render(title))
	body.WriteString("  ")
	body.WriteString(lipgloss.NewStyle().Foreground(colors.Background).Background(colors.Success).Bold(true).Padding(0, 1).Render("IDLE"))
	body.WriteString("\n\n")
	for index, candidate := range agents {
		line := fmt.Sprintf("%s  ·  %s  ·  %s", candidate.Name, models.AgentLabel(candidate.AgentType), filepath.Base(candidate.Path))
		row := lipgloss.NewStyle().Foreground(colors.Foreground).Width(innerWidth).PaddingLeft(2)
		if index == cursor {
			line = "› " + line
			row = lipgloss.NewStyle().
				Foreground(colors.Primary).
				Background(colors.SelBg).
				Bold(true).
				Width(innerWidth).
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForegroundBlend(colors.Accent, colors.Primary, colors.Accent).
				Padding(0, 1)
		}
		body.WriteString(row.Render(ansi.Truncate(line, max(8, innerWidth-4), "…")))
		body.WriteByte('\n')
	}
	body.WriteString("\n")
	body.WriteString(lipgloss.NewStyle().Foreground(colors.Muted).Render(hint))
	box := lipgloss.NewStyle().
		Width(outerWidth).
		Background(colors.Overlay).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForegroundBlend(colors.Primary, colors.Secondary, colors.Primary).
		Padding(1, 2).
		Render(body.String())
	return lipgloss.Place(m.width, max(8, m.height-4), lipgloss.Center, lipgloss.Center, box)
}
