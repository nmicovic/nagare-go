package board

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/theme"
	"github.com/nemke/nagare-go/internal/tickets"
)

func TestTodayBoardIncludesPlannedAndActiveWork(t *testing.T) {
	today := time.Now().Format(time.DateOnly)
	yesterday := time.Now().Add(-24 * time.Hour).Format(time.DateOnly)
	now := time.Now()
	model := Model{
		todayOnly: true,
		tickets: []tickets.Ticket{
			{ID: "planned", Title: "Planned", Status: tickets.StatusReady, Priority: tickets.PriorityMedium, PlannedFor: today},
			{ID: "active", Title: "Active", Status: tickets.StatusRunning, Priority: tickets.PriorityMedium, PlannedFor: yesterday},
			{ID: "backlog", Title: "Later", Status: tickets.StatusBacklog, Priority: tickets.PriorityMedium},
			{ID: "done", Title: "Finished", Status: tickets.StatusDone, Priority: tickets.PriorityMedium, CompletedAt: &now},
		},
	}

	if got := len(model.columnTickets(tickets.StatusReady)); got != 1 {
		t.Fatalf("ready count = %d, want 1", got)
	}
	if got := len(model.columnTickets(tickets.StatusRunning)); got != 1 {
		t.Fatalf("running count = %d, want active work even when planned earlier", got)
	}
	if got := len(model.columnTickets(tickets.StatusBacklog)); got != 0 {
		t.Fatalf("backlog count = %d, want 0 in Today view", got)
	}
	if got := len(model.columnTickets(tickets.StatusDone)); got != 1 {
		t.Fatalf("done count = %d, want today's completion", got)
	}
}

func TestNumberKeysJumpToLanes(t *testing.T) {
	model := Model{column: len(tickets.BoardStatuses) - 1}
	for index, status := range tickets.BoardStatuses {
		key := rune('1' + index)
		next, _ := model.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
		model = next.(Model)
		if model.column != index {
			t.Errorf("key %q selected column %d, want %d", key, model.column, index)
		}
		if model.currentStatus() != status {
			t.Errorf("key %q selected status %q, want %q", key, model.currentStatus(), status)
		}
	}

	footer := ansi.Strip(Model{width: 200}.renderFooter())
	if !strings.Contains(footer, "1-5 jump") {
		t.Errorf("footer does not advertise numbered lane jumps: %q", footer)
	}
}

func TestMoveSelectedPersistsWorkflowTransition(t *testing.T) {
	store := tickets.NewStore(t.TempDir())
	ticket, err := store.Create(tickets.CreateInput{
		Title:      "Move me",
		Status:     tickets.StatusReady,
		Priority:   tickets.PriorityMedium,
		PlannedFor: time.Now().Format(time.DateOnly),
	})
	if err != nil {
		t.Fatal(err)
	}
	model := Model{
		store:     store,
		tickets:   []tickets.Ticket{ticket},
		column:    statusIndex(tickets.StatusReady),
		cursors:   map[tickets.Status]int{},
		todayOnly: true,
	}
	model.moveSelected(1)

	updated, err := store.Get(ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != tickets.StatusRunning {
		t.Fatalf("status = %q, want running", updated.Status)
	}
	if model.column != statusIndex(tickets.StatusRunning) {
		t.Fatalf("column = %d, want running column", model.column)
	}
}

func TestAssignmentPromptCarriesTicketContract(t *testing.T) {
	ticket := tickets.Ticket{ID: "ticket-123", Title: "Build board", Description: "Keep tickets durable."}
	prompt := assignmentPrompt(ticket)
	for _, want := range []string{"ticket-123", "Build board", "Keep tickets durable.", "submit_ticket", "what changed", "repository", "human review"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("assignment prompt missing %q: %s", want, prompt)
		}
	}
}

func TestNewTicketCanRemainUnassignedWithoutProject(t *testing.T) {
	store := tickets.NewStore(t.TempDir())
	form := Form{
		store: store,
		state: &formState{
			title:    "Sort out today's work",
			priority: string(tickets.PriorityMedium),
			today:    true,
		},
		agents: map[string]models.Session{},
	}
	if err := form.save(); err != nil {
		t.Fatal(err)
	}
	all, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("ticket count = %d, want 1", len(all))
	}
	if all[0].ProjectPath != "" || all[0].AssigneeSession != "" {
		t.Fatalf("unassigned ticket = %#v, want no project or assignee", all[0])
	}
}

func TestAvailableAgentsOnlyIncludesIdleSessions(t *testing.T) {
	sessions := []models.Session{
		{Name: "working", Status: models.StatusRunning},
		{Name: "idle-b", Status: models.StatusIdle},
		{Name: "waiting", Status: models.StatusWaitingInput},
		{Name: "idle-a", Status: models.StatusIdle},
	}
	available := idleAgents(sessions)
	if len(available) != 2 || available[0].Name != "idle-a" || available[1].Name != "idle-b" {
		t.Fatalf("idleAgents() = %#v", available)
	}

	model := Model{sessions: sessions}
	model.showAvailableAgents()
	if !model.agentsMode || len(model.availableAgents) != 2 {
		t.Fatalf("available-agent overlay not opened: %#v", model)
	}
}

func TestSelectedCardHasCompleteFocusFrameAndMetadata(t *testing.T) {
	model := Model{}
	card := model.renderCard(tickets.Ticket{
		ID:       "12345678-abcd",
		Title:    "Polish the board",
		Priority: tickets.PriorityHigh,
	}, 30, true)
	plain := ansi.Strip(card)
	for _, corner := range []string{"╭", "╮", "╰", "╯"} {
		if !strings.Contains(plain, corner) {
			t.Errorf("selected card missing frame corner %q:\n%s", corner, plain)
		}
	}
	for _, metadata := range []string{"Polish the board", "General", "@Unassigned", "12345678", "HIGH"} {
		if !strings.Contains(plain, metadata) {
			t.Errorf("selected card missing %q:\n%s", metadata, plain)
		}
	}
	if got := lipgloss.Height(card); got != 5 {
		t.Errorf("selected card height = %d, want 5", got)
	}
	for index, line := range strings.Split(card, "\n") {
		if got := lipgloss.Width(line); got != 30 {
			t.Errorf("card line %d width = %d, want 30", index, got)
		}
	}
}

func TestNarrowCardDoesNotWrapMetadata(t *testing.T) {
	card := (Model{}).renderCard(tickets.Ticket{
		ID:       "12345678-abcd",
		Title:    "A narrow ticket title",
		Priority: tickets.PriorityMedium,
	}, 19, false)
	if got := lipgloss.Height(card); got != 5 {
		t.Fatalf("narrow card height = %d, want 5:\n%s", got, ansi.Strip(card))
	}
	for index, line := range strings.Split(card, "\n") {
		if got := lipgloss.Width(line); got != 19 {
			t.Errorf("narrow card line %d width = %d, want 19", index, got)
		}
	}
	if !strings.Contains(ansi.Strip(card), "MEDIUM") {
		t.Errorf("narrow card lost priority:\n%s", ansi.Strip(card))
	}
}

func TestBoardHeaderShowsViewPositionAndFilter(t *testing.T) {
	model := Model{width: 100, todayOnly: true}
	header := ansi.Strip(model.renderHeader())
	for _, want := range []string{"NAGARE", "LIST", "BOARD", "GRID", "TODAY", "0 tickets"} {
		if !strings.Contains(header, want) {
			t.Errorf("header missing %q: %s", want, header)
		}
	}
}

func TestKanbanLanesShareRowsHorizontally(t *testing.T) {
	model := Model{
		width:     120,
		height:    20,
		todayOnly: false,
		cursors:   map[tickets.Status]int{},
	}
	rendered := ansi.Strip(model.renderColumns(16))
	lines := strings.Split(rendered, "\n")
	if got := strings.Count(lines[0], "╭"); got != len(tickets.BoardStatuses) {
		t.Fatalf("top row has %d lane frames, want %d:\n%s", got, len(tickets.BoardStatuses), rendered)
	}
	if got := strings.Count(lines[len(lines)-1], "╰"); got != len(tickets.BoardStatuses) {
		t.Fatalf("bottom row has %d lane frames, want %d:\n%s", got, len(tickets.BoardStatuses), rendered)
	}
	for _, status := range tickets.BoardStatuses {
		if !strings.Contains(lines[1], tickets.StatusLabel(status)) {
			t.Errorf("header row missing %s:\n%s", status, rendered)
		}
	}
}

func TestDeferredBoardReusesPickerSessions(t *testing.T) {
	store := tickets.NewStore(t.TempDir())
	model := NewDeferred(store)
	sessions := []models.Session{{Name: "picker-session", Status: models.StatusIdle}}
	model.SetSessions(sessions)

	cmd := model.Activate()
	next, _ := model.Update(cmd())
	model = next.(Model)

	if model.manageSessions {
		t.Fatal("deferred board unexpectedly owns session scanning")
	}
	if len(model.sessions) != 1 || model.sessions[0].Name != "picker-session" {
		t.Fatalf("sessions = %#v, want picker-provided sessions preserved", model.sessions)
	}
}

func TestDeactivatedBoardStopsTicker(t *testing.T) {
	model := NewDeferred(tickets.NewStore(t.TempDir()))
	model.active = true
	model.Deactivate()

	next, cmd := model.Update(tickMsg{epoch: model.refreshEpoch})
	updated := next.(Model)
	if updated.active {
		t.Fatal("board remained active after deactivation")
	}
	if cmd != nil {
		t.Fatal("inactive board scheduled another update tick")
	}
}

func TestDeleteRequiresConfirmationAndRemovesSelectedTicket(t *testing.T) {
	store := tickets.NewStore(t.TempDir())
	ticket, err := store.Create(tickets.CreateInput{
		Title:      "Delete me",
		Status:     tickets.StatusReady,
		Priority:   tickets.PriorityMedium,
		PlannedFor: time.Now().Format(time.DateOnly),
	})
	if err != nil {
		t.Fatal(err)
	}
	model := Model{
		store:     store,
		tickets:   []tickets.Ticket{ticket},
		column:    statusIndex(tickets.StatusReady),
		cursors:   map[tickets.Status]int{},
		todayOnly: true,
	}

	next, _ := model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	model = next.(Model)
	if !model.deleteMode {
		t.Fatal("x did not open delete confirmation")
	}
	if _, err := store.Get(ticket.ID); err != nil {
		t.Fatalf("ticket was deleted before confirmation: %v", err)
	}

	next, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = next.(Model)
	if model.deleteMode {
		t.Fatal("escape did not cancel delete confirmation")
	}
	if _, err := store.Get(ticket.ID); err != nil {
		t.Fatalf("canceling confirmation deleted ticket: %v", err)
	}

	next, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	model = next.(Model)
	next, _ = model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	model = next.(Model)
	if _, err := store.Get(ticket.ID); err == nil {
		t.Fatal("confirmed delete left ticket in store")
	}
	if len(model.tickets) != 0 {
		t.Fatalf("board retained %d tickets after delete", len(model.tickets))
	}
	if !strings.Contains(model.statusNote, "Delete me") {
		t.Fatalf("delete status = %q, want deleted ticket title", model.statusNote)
	}
}

func TestBoardPlanesStaySolidAcrossThemes(t *testing.T) {
	originalTheme := theme.Current().Name
	defer func() {
		_ = theme.Set(originalTheme)
		theme.SetDarkBackground(true)
	}()

	ticket := tickets.Ticket{
		ID:       "12345678-abcd",
		Title:    "Solid card",
		Status:   tickets.StatusReady,
		Priority: tickets.PriorityMedium,
	}
	for _, name := range theme.Names() {
		if err := theme.Set(name); err != nil {
			t.Fatal(err)
		}
		for _, dark := range []bool{true, false} {
			theme.SetDarkBackground(dark)
			colors := theme.Current().Colors

			card := (Model{}).renderCard(ticket, 30, true)
			overlay := backgroundSequence(colors.Overlay)
			if !strings.Contains(card, overlay) {
				t.Errorf("theme %q dark=%v: selected card does not use the overlay plane", name, dark)
			}
			selected := backgroundSequence(colors.SelBg)
			if selected != overlay && strings.Contains(card, selected) {
				t.Errorf("theme %q dark=%v: selected card reintroduced the saturated selection fill", name, dark)
			}

			model := Model{
				column:    statusIndex(tickets.StatusBacklog),
				cursors:   map[tickets.Status]int{},
				todayOnly: false,
			}
			column := model.renderColumn(model.column, 30, 12)
			copyAt := strings.Index(column, emptyColumnCopy(tickets.StatusBacklog))
			if copyAt < 0 {
				t.Fatalf("theme %q dark=%v: empty copy missing", name, dark)
			}
			prefix := column[:copyAt]
			backgroundAt := strings.LastIndex(prefix, backgroundSequence(colors.Surface))
			resetAt := strings.LastIndex(prefix, "\x1b[0m")
			if backgroundAt < resetAt {
				t.Errorf("theme %q dark=%v: empty copy falls back to a different background", name, dark)
			}
		}
	}
}

func backgroundSequence(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}
