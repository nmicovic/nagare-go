package board

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/session"
	"github.com/nemke/nagare-go/internal/theme"
	"github.com/nemke/nagare-go/internal/tickets"
)

// The detail overlay is the board's reading view: everything a ticket carries,
// and the one action that ticket is waiting for. Enter opens it from any lane,
// so a Ready ticket is read before it is handed to an agent rather than
// dispatched from a two-line card.

func (m *Model) startDetail() {
	ticket, ok := m.selectedTicket()
	if !ok {
		return
	}
	m.detailMode = true
	m.detailTicket = ticket
	m.detailOffset = 0
}

func (m *Model) closeDetail() {
	m.detailMode = false
	m.detailTicket = tickets.Ticket{}
	m.detailOffset = 0
}

// syncDetail keeps the open overlay on the live ticket, since the board reloads
// every second and an agent may submit or finish while it is being read.
func (m *Model) syncDetail() {
	if !m.detailMode {
		return
	}
	for _, ticket := range m.tickets {
		if ticket.ID == m.detailTicket.ID {
			m.detailTicket = ticket
			return
		}
	}
	m.closeDetail()
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.statusErr = ""
	m.statusNote = ""
	lines := m.detailLines(m.detailInnerWidth())
	page := m.detailPage(len(lines))
	maxOffset := max(0, len(lines)-page)
	switch msg.String() {
	case "esc", "q":
		m.closeDetail()
	case "up", "k":
		m.detailOffset = max(0, m.detailOffset-1)
	case "down", "j":
		m.detailOffset = min(maxOffset, m.detailOffset+1)
	case "pgup", "ctrl+u":
		m.detailOffset = max(0, m.detailOffset-page)
	case "pgdown", "ctrl+d":
		m.detailOffset = min(maxOffset, m.detailOffset+page)
	case "home", "g":
		m.detailOffset = 0
	case "end", "G":
		m.detailOffset = maxOffset
	case "enter":
		return m.detailPrimaryAction()
	case "d":
		return m.startRun()
	case "e":
		m.result = Result{Action: ActionEdit, TicketID: m.detailTicket.ID}
		return m, tea.Quit
	case "v":
		return m.startReview()
	case "p":
		m.startPullRequest()
	case "[":
		m.moveSelected(-1)
		m.syncDetail()
	case "]":
		m.moveSelected(1)
		m.syncDetail()
	}
	return m, nil
}

// detailPrimaryAction runs the one thing the ticket's lane implies: start an
// isolated attempt while nobody owns it, jump to the agent once one does.
func (m Model) detailPrimaryAction() (tea.Model, tea.Cmd) {
	ticket := m.detailTicket
	if ticket.Status == tickets.StatusBacklog || ticket.Status == tickets.StatusReady {
		return m.startRun()
	}
	if assigned, found := m.assignedSession(ticket); found {
		session.SwitchToPane(assigned)
		return m, tea.Quit
	}
	if ticket.AssigneeSession != "" {
		m.statusErr = "assigned agent is no longer running"
		return m, nil
	}
	m.statusErr = "move this ticket to backlog or ready to start a new attempt"
	return m, nil
}

func (m Model) detailOuterWidth() int {
	return min(min(96, max(46, m.width*3/4)), max(24, m.width-4))
}

func (m Model) detailInnerWidth() int {
	return max(20, m.detailOuterWidth()-6)
}

// detailPage windows the body by rendered rows against a measured empty box,
// so a short terminal shows fewer rows instead of losing the bottom border to
// the frame clamp.
func (m Model) detailPage(total int) int {
	available := max(8, m.height-4) - m.detailChromeHeight()
	return max(1, min(max(1, total), available))
}

func (m Model) detailChromeHeight() int {
	return lipgloss.Height(m.detailBox(nil))
}

// detailLines returns the scrollable body, already wrapped to width, so the
// overlay windows by rendered rows rather than by logical fields.
func (m Model) detailLines(width int) []string {
	colors := theme.Current().Colors
	ticket := m.detailTicket
	lines := wrapDetail(ticket.Title, width,
		lipgloss.NewStyle().Foreground(colors.Primary).Background(colors.Overlay).Bold(true))
	lines = append(lines, blankDetailLine(width))

	lines = append(lines, detailField("Repo", projectLabel(ticket), width)...)
	if ticket.TargetBranch != "" {
		lines = append(lines, detailField("Target", ticket.TargetBranch, width)...)
	}
	if ticket.PlannedFor != "" {
		lines = append(lines, detailField("Planned", ticket.PlannedFor, width)...)
	}
	lines = append(lines, detailField("Created", detailTime(ticket.CreatedAt), width)...)
	lines = append(lines, detailField("Updated", detailTime(ticket.UpdatedAt), width)...)
	if ticket.CompletedAt != nil {
		lines = append(lines, detailField("Completed", detailTime(*ticket.CompletedAt), width)...)
	}
	lines = append(lines, detailField("Agent", m.detailAssignee(ticket), width)...)
	if ticket.ActiveAttemptID != "" {
		lines = append(lines, detailField("Attempt", ticket.ActiveAttemptID, width)...)
	}
	if ticket.PullRequestURL != "" {
		lines = append(lines, detailField("PR", fmt.Sprintf("#%d  %s", ticket.PullRequestNumber, ticket.PullRequestURL), width)...)
	}

	lines = append(lines, blankDetailLine(width), detailSection("Description", width))
	description := strings.TrimSpace(ticket.Description)
	body := lipgloss.NewStyle().Foreground(colors.Foreground).Background(colors.Overlay)
	if description == "" {
		description = "No description. Press e to add context before an agent starts."
		body = body.Foreground(colors.Muted).Italic(true)
	}
	lines = append(lines, wrapDetail(description, width, body)...)

	if summary := strings.TrimSpace(ticket.SubmittedSummary); summary != "" {
		lines = append(lines, blankDetailLine(width), detailSection("Agent report", width))
		lines = append(lines, wrapDetail(summary, width,
			lipgloss.NewStyle().Foreground(colors.Foreground).Background(colors.Overlay))...)
	}
	return lines
}

func (m Model) detailAssignee(ticket tickets.Ticket) string {
	if ticket.AssigneeSession == "" {
		return "Unassigned"
	}
	label := ticket.AssigneeSession
	if ticket.AssigneeAgent != "" {
		label = models.AgentLabel(models.AgentType(ticket.AssigneeAgent)) + "  ·  " + label
	}
	if assigned, ok := m.assignedSession(ticket); ok {
		return label + "  ·  " + models.StatusLabel(assigned.Status)
	}
	return label + "  ·  pane closed"
}

func projectLabel(ticket tickets.Ticket) string {
	if ticket.ProjectPath == "" {
		return "None. Press e to name the repository this ticket changes."
	}
	return ticket.ProjectPath
}

func detailTime(at time.Time) string {
	if at.IsZero() {
		return "—"
	}
	return at.Local().Format("2006-01-02 15:04")
}

func detailField(label, value string, width int) []string {
	colors := theme.Current().Colors
	labelWidth := min(12, max(9, width/5))
	valueWidth := max(8, width-labelWidth)
	head := lipgloss.NewStyle().
		Foreground(colors.Subtle).
		Background(colors.Overlay).
		Bold(true).
		Width(labelWidth).
		Render(ansi.Truncate(strings.ToUpper(label), max(1, labelWidth-1), ""))
	indent := lipgloss.NewStyle().Background(colors.Overlay).Width(labelWidth).Render("")
	rows := wrapDetail(value, valueWidth,
		lipgloss.NewStyle().Foreground(colors.Foreground).Background(colors.Overlay))
	for index := range rows {
		if index == 0 {
			rows[index] = head + rows[index]
			continue
		}
		rows[index] = indent + rows[index]
	}
	return rows
}

func detailSection(label string, width int) string {
	colors := theme.Current().Colors
	return lipgloss.NewStyle().
		Foreground(colors.Accent).
		Background(colors.Overlay).
		Bold(true).
		Width(width).
		Render(strings.ToUpper(label))
}

func blankDetailLine(width int) string {
	return lipgloss.NewStyle().Background(theme.Current().Colors.Overlay).Width(width).Render("")
}

// wrapDetail renders once at width and splits the result, so every returned
// element is exactly one terminal row — a long field wraps instead of being
// counted as one line and clipped by the viewport.
func wrapDetail(text string, width int, style lipgloss.Style) []string {
	text = strings.ReplaceAll(strings.TrimRight(text, "\n"), "\r", "")
	if text == "" {
		return nil
	}
	return strings.Split(style.Width(width).Render(text), "\n")
}

func (m Model) detailHint() string {
	switch m.detailTicket.Status {
	case tickets.StatusBacklog, tickets.StatusReady:
		return "enter run  e edit  [/] move  j/k scroll"
	case tickets.StatusRunning:
		return "enter jump to agent  e edit  [/] move  j/k scroll"
	case tickets.StatusReview:
		return "v diff  p PR  d rerun  [/] move  j/k scroll"
	default:
		return "e edit  [/] move  j/k scroll"
	}
}

func (m Model) renderDetailDialog() string {
	colors := theme.Current().Colors
	innerWidth := m.detailInnerWidth()
	ticket := m.detailTicket

	badge := lipgloss.NewStyle().
		Foreground(colors.Background).
		Background(statusColor(ticket.Status)).
		Bold(true).
		Padding(0, 1).
		Render(tickets.StatusLabel(ticket.Status))
	priority := priorityLabel(ticket.Priority, colors.Overlay)
	shortID := ticket.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	id := lipgloss.NewStyle().Foreground(colors.Muted).Background(colors.Overlay).Render(shortID)
	header := badge + lipgloss.NewStyle().Background(colors.Overlay).Render("  ") + priority
	gap := max(1, innerWidth-lipgloss.Width(header)-lipgloss.Width(id))
	header += lipgloss.NewStyle().Background(colors.Overlay).Render(strings.Repeat(" ", gap)) + id

	lines := m.detailLines(innerWidth)
	page := m.detailPage(len(lines))
	offset := min(m.detailOffset, max(0, len(lines)-page))
	end := min(len(lines), offset+page)

	scroll := ""
	if len(lines) > page {
		scroll = fmt.Sprintf("  %d-%d / %d", offset+1, end, len(lines))
	}
	return lipgloss.Place(m.width, max(8, m.height-4), lipgloss.Center, lipgloss.Center,
		m.detailBox(append([]string{header, blankDetailLine(innerWidth)}, append(lines[offset:end], blankDetailLine(innerWidth), m.detailHintLine(scroll))...)))
}

func (m Model) detailHintLine(scroll string) string {
	colors := theme.Current().Colors
	const exit = "  esc close"
	width := m.detailInnerWidth()
	hints := ansi.Truncate(m.detailHint(), max(0, width-len(exit)-len(scroll)), "…")
	return lipgloss.NewStyle().Foreground(colors.Muted).Background(colors.Overlay).Width(width).
		Render(hints + exit + scroll)
}

// detailBox renders the overlay frame around body rows. Called with no rows it
// measures its own chrome, which is what detailPage budgets against.
func (m Model) detailBox(body []string) string {
	colors := theme.Current().Colors
	if body == nil {
		body = []string{
			blankDetailLine(m.detailInnerWidth()),
			blankDetailLine(m.detailInnerWidth()),
			blankDetailLine(m.detailInnerWidth()),
			m.detailHintLine(""),
		}
	}
	return lipgloss.NewStyle().
		Width(m.detailOuterWidth()).
		Background(colors.Overlay).
		Foreground(colors.Foreground).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForegroundBlend(colors.GradientFrom, colors.GradientTo).
		BorderBackground(colors.Overlay).
		Padding(1, 2).
		Render(theme.OnPlane(strings.Join(body, "\n"), colors.Overlay))
}
