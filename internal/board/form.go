package board

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/theme"
	"github.com/nemke/nagare-go/internal/tickets"
)

var errTitleRequired = errors.New("title is required")

type formState struct {
	title       string
	description string
	assignee    string
	priority    string
	today       bool
}

// Form is the create/edit ticket form launched from the board.
type Form struct {
	form     *huh.Form
	state    *formState
	store    *tickets.Store
	agents   map[string]models.Session
	ticketID string
	width    int
	height   int
	err      error
	done     bool
}

// NewForm creates a ticket form. A nil ticket creates a new ticket; otherwise
// the form updates the supplied ticket and its optional agent assignment.
func NewForm(store *tickets.Store, ticket *tickets.Ticket) Form {
	state := &formState{
		priority: string(tickets.PriorityMedium),
		today:    true,
	}
	ticketID := ""
	if ticket != nil {
		ticketID = ticket.ID
		state.title = ticket.Title
		state.description = ticket.Description
		state.assignee = ticket.AssigneeSession
		state.priority = string(ticket.Priority)
		state.today = ticket.PlannedFor == time.Now().Format(time.DateOnly)
	}

	agents, agentOptions := ticketAgentOptions(state.assignee)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Ticket title").
				Placeholder("Outcome to accomplish").
				Validate(func(value string) error {
					if strings.TrimSpace(value) == "" {
						return errTitleRequired
					}
					return nil
				}).
				Value(&state.title),
			huh.NewText().
				Title("Description").
				Description("Context and acceptance criteria sent to the agent").
				CharLimit(4000).
				Value(&state.description),
			huh.NewSelect[string]().
				Title("Assign to agent").
				Description("Start immediately with an idle agent, or leave unassigned").
				Options(agentOptions...).
				Value(&state.assignee),
			huh.NewSelect[string]().
				Title("Priority").
				Options(huh.NewOptions("urgent", "high", "medium", "low")...).
				Value(&state.priority),
			huh.NewConfirm().
				Title("Plan for today?").
				Affirmative("Today").
				Negative("Backlog").
				Value(&state.today),
		),
	).WithTheme(theme.FormTheme{}).WithShowHelp(true)

	return Form{form: form, state: state, store: store, agents: agents, ticketID: ticketID}
}

func ticketAgentOptions(current string) (map[string]models.Session, []huh.Option[string]) {
	sessions := scanAgentSessions()
	sort.SliceStable(sessions, func(i, j int) bool { return sessions[i].Name < sessions[j].Name })

	agents := make(map[string]models.Session)
	options := []huh.Option[string]{huh.NewOption("Unassigned", "")}
	for _, candidate := range sessions {
		if candidate.Status != models.StatusIdle && candidate.Name != current {
			continue
		}
		agents[candidate.Name] = candidate
		project := filepath.Base(candidate.Path)
		label := fmt.Sprintf("%s · %s · %s", candidate.Name, models.AgentLabel(candidate.AgentType), project)
		options = append(options, huh.NewOption(label, candidate.Name))
	}
	if current != "" {
		if _, found := agents[current]; !found {
			options = append(options, huh.NewOption(current+" · unavailable", current))
		}
	}
	return agents, options
}

func (m Form) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, m.form.Init())
}

func (m Form) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		theme.SetDarkBackground(msg.IsDark())

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if msg.String() == "esc" {
			return m, tea.Quit
		}
	}

	next, cmd := m.form.Update(msg)
	if form, ok := next.(*huh.Form); ok {
		m.form = form
	}
	if m.form.State == huh.StateCompleted && !m.done {
		m.done = true
		if err := m.save(); err != nil {
			m.err = err
			return m, nil
		}
		return m, tea.Quit
	}
	return m, cmd
}

func (m Form) View() tea.View {
	view := tea.NewView(m.view())
	view.AltScreen = true
	return view
}

func (m Form) view() string {
	if m.width == 0 {
		return "Loading..."
	}
	colors := theme.Current().Colors
	title := "New Ticket"
	if m.ticketID != "" {
		title = "Edit Ticket"
	}
	body := m.form.View()
	if m.err != nil {
		body += "\n" + lipgloss.NewStyle().Foreground(colors.Error).Render("Error: "+m.err.Error())
	}
	box := lipgloss.NewStyle().
		Background(colors.Background).
		Foreground(colors.Foreground).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colors.Border).
		Padding(1, 2).
		Render(lipgloss.NewStyle().Foreground(colors.Primary).Bold(true).Render(title) + "\n\n" + body)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Form) save() error {
	plannedFor := ""
	status := tickets.StatusBacklog
	if m.state.today {
		plannedFor = time.Now().Format(time.DateOnly)
		status = tickets.StatusReady
	}
	if m.ticketID == "" {
		ticket, err := m.store.Create(tickets.CreateInput{
			Title:       m.state.title,
			Description: m.state.description,
			Status:      status,
			Priority:    tickets.Priority(m.state.priority),
			PlannedFor:  plannedFor,
		})
		if err != nil || m.state.assignee == "" {
			return err
		}
		target, ok := m.agents[m.state.assignee]
		if !ok {
			return fmt.Errorf("agent %q is no longer available", m.state.assignee)
		}
		return assignTicket(m.store, ticket, target)
	}

	before, err := m.store.Get(m.ticketID)
	if err != nil {
		return err
	}
	updated, err := m.store.Update(m.ticketID, func(ticket *tickets.Ticket) error {
		ticket.Title = m.state.title
		ticket.Description = m.state.description
		ticket.Priority = tickets.Priority(m.state.priority)
		ticket.PlannedFor = plannedFor
		if ticket.Status == tickets.StatusBacklog || ticket.Status == tickets.StatusReady {
			ticket.Status = status
		}
		if m.state.assignee == "" && ticket.AssigneeSession != "" {
			ticket.Status = status
			ticket.ProjectPath = ""
			ticket.AssigneeSession = ""
			ticket.AssigneePaneID = ""
			ticket.AssigneeAgent = ""
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("save ticket: %w", err)
	}
	if m.state.assignee == "" || m.state.assignee == before.AssigneeSession {
		return nil
	}
	target, ok := m.agents[m.state.assignee]
	if !ok {
		return fmt.Errorf("agent %q is no longer available", m.state.assignee)
	}
	return assignTicket(m.store, updated, target)
}
