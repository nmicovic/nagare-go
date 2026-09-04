package board

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/nemke/nagare-go/internal/git"
	"github.com/nemke/nagare-go/internal/session"
	"github.com/nemke/nagare-go/internal/theme"
	"github.com/nemke/nagare-go/internal/tickets"
)

var errTitleRequired = errors.New("title is required")

type formState struct {
	title        string
	description  string
	projectPath  string
	targetBranch string
	priority     string
	today        bool
}

// Form is the create/edit ticket form launched from the board.
type Form struct {
	form     *huh.Form
	state    *formState
	store    *tickets.Store
	ticketID string
	width    int
	height   int
	err      error
	done     bool
}

// NewForm creates a ticket form. A nil ticket creates a new ticket; otherwise
// the form updates the supplied ticket.
func NewForm(store *tickets.Store, ticket *tickets.Ticket) Form {
	state := &formState{
		priority:     string(tickets.PriorityMedium),
		today:        true,
		targetBranch: "main",
	}
	if cwd, err := os.Getwd(); err == nil {
		if root := git.MainRoot(cwd); root != "" {
			state.projectPath = root
			if branch := git.DefaultBranch(root); branch != "" {
				state.targetBranch = branch
			}
		}
	}
	ticketID := ""
	if ticket != nil {
		ticketID = ticket.ID
		state.title = ticket.Title
		state.description = ticket.Description
		state.projectPath = ticket.ProjectPath
		state.targetBranch = ticket.TargetBranch
		if state.targetBranch == "" {
			state.targetBranch = "main"
		}
		state.priority = string(ticket.Priority)
		state.today = ticket.PlannedFor == time.Now().Format(time.DateOnly)
	}
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
			huh.NewInput().
				Title("Repository path").
				Description("Git repository used for isolated ticket worktrees").
				Placeholder("~/Projects/my-project").
				Value(&state.projectPath),
			huh.NewInput().
				Title("Target branch").
				Description("New attempts start from this branch without changing the source checkout").
				Placeholder("main").
				Value(&state.targetBranch),
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

	return Form{form: form, state: state, store: store, ticketID: ticketID}
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

	projectPath := strings.TrimSpace(m.state.projectPath)
	targetBranch := strings.TrimSpace(m.state.targetBranch)
	if projectPath != "" {
		projectPath = session.ExpandPath(projectPath)
		root := git.MainRoot(projectPath)
		if root == "" {
			return fmt.Errorf("%s is not a git repository", projectPath)
		}
		projectPath = root
		if targetBranch == "" {
			targetBranch = git.DefaultBranch(root)
		}
		if targetBranch == "" {
			return fmt.Errorf("target branch is required")
		}
		if _, err := git.ResolveBaseCommit(root, targetBranch); err != nil {
			return err
		}
	} else {
		targetBranch = ""
	}

	if m.ticketID == "" {
		_, err := m.store.Create(tickets.CreateInput{
			Title:        m.state.title,
			Description:  m.state.description,
			ProjectPath:  projectPath,
			TargetBranch: targetBranch,
			Status:       status,
			Priority:     tickets.Priority(m.state.priority),
			PlannedFor:   plannedFor,
		})
		return err
	}

	before, err := m.store.Get(m.ticketID)
	if err != nil {
		return err
	}
	if (before.Status == tickets.StatusRunning || before.Status == tickets.StatusReview) &&
		(before.ProjectPath != projectPath || before.TargetBranch != targetBranch) {
		return fmt.Errorf("cannot retarget an active ticket")
	}
	_, err = m.store.Update(m.ticketID, func(ticket *tickets.Ticket) error {
		ticket.Title = m.state.title
		ticket.Description = m.state.description
		ticket.ProjectPath = projectPath
		ticket.TargetBranch = targetBranch
		ticket.Priority = tickets.Priority(m.state.priority)
		ticket.PlannedFor = plannedFor
		if ticket.Status == tickets.StatusBacklog || ticket.Status == tickets.StatusReady {
			ticket.Status = status
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("save ticket: %w", err)
	}
	return nil
}
