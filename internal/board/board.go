// Package board implements the local, cross-project ticket board.
package board

import (
	"fmt"
	"image/color"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/orchestrator"
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
type launchMsg struct {
	sessionName string
	err         error
}
type reconcileMsg struct {
	err error
}
type archiveMsg struct {
	err error
}
type reviewMsg struct {
	review orchestrator.Review
	err    error
}
type pullRequestMsg struct {
	pr  orchestrator.PullRequest
	err error
}

// Model is the ticket board Bubble Tea model.
type Model struct {
	store           *tickets.Store
	tickets         []tickets.Ticket
	sessions        []models.Session
	column          int
	cursors         map[tickets.Status]int
	width           int
	height          int
	todayOnly       bool
	guideMode       bool
	guidePage       int
	runMode         bool
	runAgents       []models.AgentType
	runCursor       int
	runAgent        models.AgentType
	modelMode       bool
	modelInput      textinput.Model
	launching       bool
	agentsMode      bool
	availableAgents []models.Session
	agentsCursor    int
	deleteMode      bool
	archiveMode     bool
	archiveTicket   tickets.Ticket
	deleteTicket    tickets.Ticket
	detailMode      bool
	detailTicket    tickets.Ticket
	detailOffset    int
	reviewMode      bool
	reviewLoading   bool
	reviewTicket    tickets.Ticket
	reviewLines     []string
	reviewOffset    int
	prMode          bool
	prTicket        tickets.Ticket
	statusNote      string
	statusErr       string
	result          Result
	orchestrator    *orchestrator.Service
	manageSessions  bool
	active          bool
	refreshEpoch    uint64
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
		store:        store,
		orchestrator: orchestrator.NewDefaultService(),
		cursors:      make(map[tickets.Status]int),
		todayOnly:    true,
		modelInput:   newModelInput(),
	}
}

func newModelInput() textinput.Model {
	input := textinput.New()
	input.CharLimit = 128
	input.Prompt = ""
	return input
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
	commands := []tea.Cmd{m.refresh()}
	if m.orchestrator != nil {
		service, store := m.orchestrator, m.store
		commands = append(commands, func() tea.Msg {
			return reconcileMsg{err: service.Reconcile(store)}
		})
	}
	if m.manageSessions {
		commands = append(commands, tea.RequestBackgroundColor)
	}
	return tea.Batch(commands...)
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
		m.syncDetail()
		if !m.active {
			return m, nil
		}
		return m, tick(m.refreshEpoch)
	case launchMsg:
		m.launching = false
		if msg.err != nil {
			m.statusErr = msg.err.Error()
		} else {
			m.statusNote = "started isolated agent " + msg.sessionName
			m.column = statusIndex(tickets.StatusRunning)
		}
		m.reload()
		return m, nil
	case archiveMsg:
		if msg.err != nil {
			m.statusErr = msg.err.Error()
		} else {
			m.statusNote = "archived clean worktree; branch retained"
		}
		m.reload()
		return m, nil
	case reviewMsg:
		m.reviewLoading = false
		if msg.err != nil {
			m.reviewMode = false
			m.reviewTicket = tickets.Ticket{}
			m.reviewLines = nil
			m.statusErr = msg.err.Error()
		} else {
			m.reviewLines = formatReviewLines(m.reviewTicket, msg.review)
			m.reviewOffset = 0
		}
		return m, nil
	case pullRequestMsg:
		if msg.err != nil {
			m.statusErr = msg.err.Error()
		} else {
			m.statusNote = fmt.Sprintf("created pull request #%d  %s", msg.pr.Number, msg.pr.URL)
		}
		m.reload()
		return m, nil
	case reconcileMsg:
		if msg.err != nil {
			m.statusErr = msg.err.Error()
		}
		return m, nil
	case tea.KeyMsg:
		if m.guideMode {
			return m.handleGuideKey(msg.String()), nil
		}
		if m.reviewMode {
			return m.handleReviewKey(msg)
		}
		if m.prMode {
			return m.handlePullRequestKey(msg)
		}
		if m.deleteMode {
			return m.handleDeleteKey(msg)
		}
		if m.archiveMode {
			return m.handleArchiveKey(msg)
		}
		if m.modelMode {
			return m.handleModelKey(msg)
		}
		if m.runMode {
			return m.handleRunKey(msg)
		}
		if m.agentsMode {
			return m.handleAgentsKey(msg)
		}
		if m.detailMode {
			return m.handleDetailKey(msg)
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
	case "1", "2", "3", "4", "5":
		column := int(key[0] - '1')
		if column < len(tickets.BoardStatuses) {
			m.column = column
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
	case "c":
		m.startArchive()
	case "x":
		m.startDelete()
	case "d":
		return m.startRun()
	case "a":
		m.showAvailableAgents()
	case "v":
		return m.startReview()
	case "p":
		m.startPullRequest()
	case "?":
		m.guideMode = true
		m.guidePage = 0
	case "enter":
		m.startDetail()
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
func (m Model) handleArchiveKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		service := m.orchestrator
		if service == nil {
			service = orchestrator.NewDefaultService()
		}
		store, ticketID := m.store, m.archiveTicket.ID
		m.archiveMode = false
		m.archiveTicket = tickets.Ticket{}
		m.statusNote = "checking managed worktree..."
		return m, func() tea.Msg {
			return archiveMsg{err: service.Archive(store, ticketID)}
		}
	case "n", "esc", "q", "c":
		m.archiveMode = false
		m.archiveTicket = tickets.Ticket{}
	}
	return m, nil
}
func (m Model) handleReviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	page := m.reviewViewportHeight()
	maxOffset := max(0, len(m.reviewLines)-page)
	switch msg.String() {
	case "esc", "q", "v":
		m.reviewMode = false
		m.reviewLoading = false
		m.reviewTicket = tickets.Ticket{}
		m.reviewLines = nil
		m.reviewOffset = 0
	case "up", "k":
		m.reviewOffset = max(0, m.reviewOffset-1)
	case "down", "j":
		m.reviewOffset = min(maxOffset, m.reviewOffset+1)
	case "pgup", "ctrl+u":
		m.reviewOffset = max(0, m.reviewOffset-page)
	case "pgdown", "ctrl+d":
		m.reviewOffset = min(maxOffset, m.reviewOffset+page)
	case "home", "g":
		m.reviewOffset = 0
	case "end", "G":
		m.reviewOffset = maxOffset
	}
	return m, nil
}

func (m Model) handlePullRequestKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		service := m.orchestrator
		if service == nil {
			service = orchestrator.NewDefaultService()
		}
		store, ticketID := m.store, m.prTicket.ID
		m.prMode = false
		m.prTicket = tickets.Ticket{}
		m.statusNote = "checking and pushing recorded branch..."
		return m, func() tea.Msg {
			pr, err := service.CreatePullRequest(store, ticketID)
			return pullRequestMsg{pr: pr, err: err}
		}
	case "n", "esc", "q", "p":
		m.prMode = false
		m.prTicket = tickets.Ticket{}
	}
	return m, nil
}

func (m Model) handleRunKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.runMode = false
		m.runAgents = nil
		m.runCursor = 0
		m.runAgent = models.AgentUnknown
		m.modelInput = newModelInput()
	case "up", "k":
		if m.runCursor > 0 {
			m.runCursor--
		}
	case "down", "j":
		if m.runCursor < len(m.runAgents)-1 {
			m.runCursor++
		}
	case "enter":
		if m.runCursor < 0 || m.runCursor >= len(m.runAgents) {
			return m, nil
		}
		agent := m.runAgents[m.runCursor]
		if session.SupportsModelSelection(string(agent)) {
			m.runAgent = agent
			m.runMode = false
			m.modelMode = true
			m.modelInput.SetValue("")
			return m, m.modelInput.Focus()
		}
		return m.launchRun(agent, "")
	}
	return m, nil
}

func (m Model) handleModelKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modelMode = false
		m.modelInput.Blur()
		m.statusErr = ""
		m.modelInput.SetValue("")
		m.runMode = true
		return m, nil
	case "enter":
		model := strings.TrimSpace(m.modelInput.Value())
		if err := session.ValidateModelSelection(string(m.runAgent), model); err != nil {
			m.statusErr = err.Error()
			return m, nil
		}
		return m.launchRun(m.runAgent, model)
	default:
		m.statusErr = ""
		var command tea.Cmd
		m.modelInput, command = m.modelInput.Update(msg)
		return m, command
	}
}

func (m Model) launchRun(agent models.AgentType, model string) (tea.Model, tea.Cmd) {
	ticket, ok := m.selectedTicket()
	if !ok {
		m.runMode = false
		m.modelMode = false
		return m, nil
	}
	service := m.orchestrator
	if service == nil {
		service = orchestrator.NewDefaultService()
		m.orchestrator = service
	}
	spec := session.AgentSpec{Agent: string(agent), Model: strings.TrimSpace(model)}
	store := m.store
	m.runMode = false
	m.runAgents = nil
	m.runCursor = 0
	m.runAgent = models.AgentUnknown
	m.modelMode = false
	m.modelInput.Blur()
	m.modelInput.SetValue("")
	m.closeDetail()
	m.launching = true
	m.statusNote = "provisioning isolated worktree..."
	return m, func() tea.Msg {
		attempt, err := service.Start(store, ticket.ID, spec)
		return launchMsg{sessionName: attempt.SessionName, err: err}
	}
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
	m.syncDetail()
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
	if ticket.ActiveAttemptID != "" {
		m.statusErr = "archive the managed attempt before deleting this ticket"
		return
	}
	m.deleteMode = true
	m.deleteTicket = ticket
}
func (m *Model) startArchive() {
	ticket, ok := m.selectedTicket()
	if !ok {
		return
	}
	if ticket.Status != tickets.StatusDone {
		m.statusErr = "only done tickets can be archived"
		return
	}
	if ticket.ActiveAttemptID == "" {
		m.statusErr = "ticket has no managed attempt"
		return
	}
	m.archiveMode = true
	m.archiveTicket = ticket
}
func (m Model) startReview() (tea.Model, tea.Cmd) {
	ticket, ok := m.selectedTicket()
	if !ok {
		return m, nil
	}
	if ticket.Status != tickets.StatusReview {
		m.statusErr = "only review tickets have a submitted diff"
		return m, nil
	}
	if ticket.ActiveAttemptID == "" {
		m.statusErr = "ticket has no managed attempt"
		return m, nil
	}
	service := m.orchestrator
	if service == nil {
		service = orchestrator.NewDefaultService()
		m.orchestrator = service
	}
	m.reviewMode = true
	m.reviewLoading = true
	m.reviewTicket = ticket
	m.reviewLines = nil
	m.reviewOffset = 0
	store := m.store
	return m, func() tea.Msg {
		review, err := service.Review(store, ticket.ID)
		return reviewMsg{review: review, err: err}
	}
}

func (m *Model) startPullRequest() {
	ticket, ok := m.selectedTicket()
	if !ok {
		return
	}
	if ticket.Status != tickets.StatusReview {
		m.statusErr = "only review tickets can create a pull request"
		return
	}
	if ticket.ActiveAttemptID == "" {
		m.statusErr = "ticket has no managed attempt"
		return
	}
	if ticket.PullRequestURL != "" {
		m.statusNote = fmt.Sprintf("pull request #%d  %s", ticket.PullRequestNumber, ticket.PullRequestURL)
		return
	}
	m.prMode = true
	m.prTicket = ticket
}

func (m Model) reviewViewportHeight() int {
	return max(1, m.height-14)
}

func formatReviewLines(ticket tickets.Ticket, review orchestrator.Review) []string {
	lines := []string{
		ticket.Title,
		fmt.Sprintf("Branch: %s  →  %s", review.Attempt.Branch, review.Attempt.TargetBranch),
		fmt.Sprintf("Base: %s", review.Attempt.BaseCommit),
		fmt.Sprintf("Commits: %d  Uncommitted files: %d", review.Git.Commits, review.Git.DirtyFiles),
	}
	if review.Attempt.PullRequestURL != "" {
		lines = append(lines, fmt.Sprintf("PR #%d: %s", review.Attempt.PullRequestNumber, review.Attempt.PullRequestURL))
	}
	lines = append(lines, "", "STAT")
	stat := strings.TrimSpace(ansi.Strip(review.Git.Stat))
	if stat == "" {
		lines = append(lines, "No tracked changes.")
	} else {
		lines = append(lines, strings.Split(stat, "\n")...)
	}
	lines = append(lines, "", "DIFF")
	diff := strings.TrimSpace(ansi.Strip(review.Git.Diff))
	if diff == "" {
		lines = append(lines, "No tracked diff.")
	} else {
		lines = append(lines, strings.Split(diff, "\n")...)
	}
	return lines
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

func (m Model) startRun() (tea.Model, tea.Cmd) {
	if m.launching {
		m.statusErr = "a ticket attempt is already provisioning"
		return m, nil
	}
	ticket, ok := m.selectedTicket()
	if !ok {
		return m, nil
	}
	if ticket.Status != tickets.StatusBacklog && ticket.Status != tickets.StatusReady {
		m.statusErr = "only backlog or ready tickets can start a new attempt"
		return m, nil
	}
	if ticket.ProjectPath == "" {
		m.statusErr = "ticket has no repository; press e to set one"
		return m, nil
	}
	m.runAgent = models.AgentUnknown
	m.modelMode = false
	m.modelInput = newModelInput()
	m.runAgents = []models.AgentType{
		models.AgentClaude,
		models.AgentCodex,
		models.AgentOpenCode,
		models.AgentGemini,
		models.AgentCrush,
		models.AgentPi,
		models.AgentOhMyPi,
	}
	m.runMode = true
	m.runCursor = 0
	return m, nil
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

func assignmentPrompt(ticket tickets.Ticket) string {
	return orchestrator.AssignmentPrompt(ticket)
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
	if m.detailMode {
		columns = m.renderDetailDialog()
	}
	if m.runMode {
		columns = m.renderRunDialog()
	}
	if m.modelMode {
		columns = m.renderModelDialog()
	}
	if m.agentsMode {
		columns = m.renderAgentsDialog("Available agents", m.availableAgents, m.agentsCursor, "j/k choose  enter open  a/esc close")
	}
	if m.archiveMode {
		columns = m.renderArchiveDialog()
	}
	if m.deleteMode {
		columns = m.renderDeleteDialog()
	}
	if m.reviewMode {
		columns = m.renderReviewDialog()
	}
	if m.prMode {
		columns = m.renderPullRequestDialog()
	}
	if m.guideMode {
		columns = m.renderGuideDialog()
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
		key("?", "guide"),
		key("h/l", "lane"),
		key("1-5", "jump"),
		key("j/k", "card"),
		key("enter", "open"),
		key("[/]", "move"),
		key("n", "new"),
		key("x", "delete"),
		key("c", "archive"),
		key("d", "run"),
		key("v", "review"),
		key("p", "PR"),
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
	title := statusStyle.Render(fmt.Sprintf("%d  %s %s", index+1, statusIcon(status), tickets.StatusLabel(status)))
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
	if ticket.PullRequestNumber > 0 {
		meta += fmt.Sprintf("  ·  PR #%d", ticket.PullRequestNumber)
	}
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

func (m Model) renderReviewDialog() string {
	colors := theme.Current().Colors
	outerWidth := min(120, max(20, m.width-4))
	innerWidth := max(14, outerWidth-6)
	title := lipgloss.NewStyle().Foreground(colors.Secondary).Bold(true).Render("Submitted diff")
	badge := lipgloss.NewStyle().Foreground(colors.Background).Background(colors.Secondary).Bold(true).Padding(0, 1).Render("REVIEW")
	header := title + "  " + badge

	var rows []string
	if m.reviewLoading {
		rows = []string{lipgloss.NewStyle().Foreground(colors.Muted).Render("Loading managed attempt...")}
	} else {
		page := m.reviewViewportHeight()
		end := min(len(m.reviewLines), m.reviewOffset+page)
		for _, line := range m.reviewLines[m.reviewOffset:end] {
			line = strings.ReplaceAll(line, "\r", "")
			style := lipgloss.NewStyle().Foreground(colors.Foreground).Background(colors.Overlay)
			switch {
			case line == "STAT" || line == "DIFF":
				style = style.Foreground(colors.Accent).Bold(true)
			case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
				style = style.Foreground(colors.Success)
			case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
				style = style.Foreground(colors.Error)
			case strings.HasPrefix(line, "@@"):
				style = style.Foreground(colors.Secondary)
			}
			rows = append(rows, style.Render(ansi.Truncate(line, innerWidth, "…")))
		}
	}
	if len(rows) == 0 {
		rows = []string{lipgloss.NewStyle().Foreground(colors.Muted).Render("No diff output.")}
	}
	scroll := ""
	if !m.reviewLoading {
		end := min(len(m.reviewLines), m.reviewOffset+m.reviewViewportHeight())
		scroll = fmt.Sprintf("  %d-%d / %d", min(len(m.reviewLines), m.reviewOffset+1), end, len(m.reviewLines))
	}
	hint := lipgloss.NewStyle().Foreground(colors.Muted).
		Render("j/k scroll  pgup/pgdown page  g/G ends  v/esc close" + scroll)
	body := header + "\n\n" + strings.Join(rows, "\n") + "\n\n" + hint
	box := lipgloss.NewStyle().
		Width(outerWidth).
		Background(colors.Overlay).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colors.Secondary).
		Padding(1, 2).
		Render(body)
	return lipgloss.Place(m.width, max(8, m.height-4), lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderPullRequestDialog() string {
	colors := theme.Current().Colors
	outerWidth := min(min(max(48, m.width/2), 76), max(20, m.width-4))
	innerWidth := max(16, outerWidth-6)
	title := lipgloss.NewStyle().Foreground(colors.Warning).Bold(true).Render("Push branch and create pull request?")
	name := lipgloss.NewStyle().Foreground(colors.Foreground).Bold(true).
		Render(ansi.Truncate(m.prTicket.Title, innerWidth, "…"))
	target := lipgloss.NewStyle().Foreground(colors.Accent).
		Render("Target: " + m.prTicket.TargetBranch)
	warning := lipgloss.NewStyle().Foreground(colors.Muted).
		Render("Requires a clean submitted worktree. Push is non-force and limited to the recorded branch.")
	hint := lipgloss.NewStyle().Foreground(colors.Warning).Bold(true).Render("y/enter create PR") + "  " +
		lipgloss.NewStyle().Foreground(colors.Muted).Render("n/esc cancel")
	body := strings.Join([]string{title, "", name, target, warning, "", hint}, "\n")
	box := lipgloss.NewStyle().
		Width(outerWidth).
		Background(colors.Overlay).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colors.Warning).
		Padding(1, 2).
		Render(body)
	return lipgloss.Place(m.width, max(8, m.height-4), lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderArchiveDialog() string {
	colors := theme.Current().Colors
	outerWidth := min(min(max(44, m.width/2), 72), max(20, m.width-4))
	innerWidth := max(16, outerWidth-6)
	title := lipgloss.NewStyle().Foreground(colors.Accent).Bold(true).Render("Archive managed worktree?")
	name := lipgloss.NewStyle().Foreground(colors.Foreground).Bold(true).
		Render(ansi.Truncate(m.archiveTicket.Title, innerWidth, "…"))
	warning := lipgloss.NewStyle().Foreground(colors.Muted).
		Render("Only a clean worktree is removed. Its branch and commits are retained.")
	hint := lipgloss.NewStyle().Foreground(colors.Accent).Bold(true).Render("y/enter archive") + "  " +
		lipgloss.NewStyle().Foreground(colors.Muted).Render("n/esc cancel")
	body := strings.Join([]string{title, "", name, warning, "", hint}, "\n")
	box := lipgloss.NewStyle().
		Width(outerWidth).
		Background(colors.Overlay).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colors.Accent).
		Padding(1, 2).
		Render(body)
	return lipgloss.Place(m.width, max(8, m.height-4), lipgloss.Center, lipgloss.Center, box)
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

func (m Model) renderRunDialog() string {
	colors := theme.Current().Colors
	outerWidth := min(max(40, m.width/2), 68)
	innerWidth := max(20, outerWidth-6)
	var body strings.Builder
	body.WriteString(lipgloss.NewStyle().Foreground(colors.Primary).Bold(true).Render("Run isolated attempt"))
	body.WriteString("  ")
	body.WriteString(lipgloss.NewStyle().Foreground(colors.Background).Background(colors.Accent).Bold(true).Padding(0, 1).Render("WORKTREE"))
	body.WriteString("\n\n")
	for index, agent := range m.runAgents {
		line := models.AgentLabel(agent)
		row := lipgloss.NewStyle().Foreground(colors.Foreground).Width(innerWidth).PaddingLeft(2)
		if index == m.runCursor {
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
	body.WriteString(lipgloss.NewStyle().Foreground(colors.Muted).Render("j/k choose  enter continue  esc cancel"))
	box := lipgloss.NewStyle().
		Width(outerWidth).
		Background(colors.Overlay).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForegroundBlend(colors.Primary, colors.Secondary, colors.Primary).
		Padding(1, 2).
		Render(body.String())
	return lipgloss.Place(m.width, max(8, m.height-4), lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderModelDialog() string {
	colors := theme.Current().Colors
	outerWidth := min(min(max(44, m.width/2), 72), max(20, m.width-4))
	innerWidth := max(20, outerWidth-6)
	input := m.modelInput
	input.SetWidth(max(8, innerWidth-4))

	title := lipgloss.NewStyle().Foreground(colors.Primary).Bold(true).Render("Choose model")
	agent := lipgloss.NewStyle().
		Foreground(colors.Background).
		Background(colors.Accent).
		Bold(true).
		Padding(0, 1).
		Render(models.AgentLabel(m.runAgent))
	label := lipgloss.NewStyle().Foreground(colors.Foreground).Bold(true).Render("Model")
	field := lipgloss.NewStyle().
		Width(innerWidth).
		Foreground(colors.Foreground).
		Background(colors.Overlay).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colors.BorderFocus).
		Padding(0, 1).
		Render(input.View())
	examples := lipgloss.NewStyle().Foreground(colors.Subtle).Render(modelInputHint(m.runAgent))
	hint := lipgloss.NewStyle().Foreground(colors.Muted).
		Render("enter run  empty uses agent default  esc back")
	body := strings.Join([]string{title + "  " + agent, "", label, field, examples, "", hint}, "\n")
	box := lipgloss.NewStyle().
		Width(outerWidth).
		Background(colors.Overlay).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForegroundBlend(colors.Primary, colors.Secondary, colors.Primary).
		Padding(1, 2).
		Render(body)
	return lipgloss.Place(m.width, max(8, m.height-4), lipgloss.Center, lipgloss.Center, box)
}

func modelInputHint(agent models.AgentType) string {
	switch agent {
	case models.AgentClaude:
		return "Aliases: fable, opus, sonnet; full model IDs also work."
	case models.AgentOpenCode, models.AgentPi:
		return "Use a model ID or provider/model."
	case models.AgentOhMyPi:
		return "Use a model ID, provider/model, or fuzzy model name."
	default:
		return "Use the model ID accepted by this agent."
	}
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
