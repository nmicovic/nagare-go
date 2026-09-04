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

	"github.com/nemke/nagare-go/internal/attempts"
	"github.com/nemke/nagare-go/internal/git"
	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/orchestrator"
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

func TestRunOpensAgentPickerForRepositoryTicket(t *testing.T) {
	model := Model{
		tickets: []tickets.Ticket{{
			ID:          "ticket",
			Title:       "Isolated work",
			ProjectPath: "/repo",
			Status:      tickets.StatusReady,
			Priority:    tickets.PriorityMedium,
		}},
		column:  statusIndex(tickets.StatusReady),
		cursors: map[tickets.Status]int{tickets.StatusReady: 0},
	}
	next, cmd := model.handleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	updated := next.(Model)
	if cmd != nil {
		t.Fatal("opening run picker returned a command")
	}
	if !updated.runMode || len(updated.runAgents) != 7 {
		t.Fatalf("run picker = %#v", updated)
	}
	if updated.runAgents[0] != models.AgentClaude || updated.runAgents[1] != models.AgentCodex {
		t.Fatalf("agent order = %#v", updated.runAgents)
	}
}

func TestManagedTicketMustBeArchivedBeforeDeletion(t *testing.T) {
	model := Model{
		tickets: []tickets.Ticket{{
			ID: "ticket", Title: "Protected", ActiveAttemptID: "attempt",
			Status: tickets.StatusDone, Priority: tickets.PriorityMedium,
		}},
		column:  statusIndex(tickets.StatusDone),
		cursors: map[tickets.Status]int{tickets.StatusDone: 0},
	}
	model.startDelete()
	if model.deleteMode {
		t.Fatal("managed ticket entered delete mode")
	}
	if !strings.Contains(model.statusErr, "archive") {
		t.Fatalf("status error = %q", model.statusErr)
	}
	model.startArchive()
	if !model.archiveMode || model.archiveTicket.ID != "ticket" {
		t.Fatalf("archive mode = %#v", model)
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

func TestReviewOverlayFormatsAndScrollsSubmittedDiff(t *testing.T) {
	ticket := tickets.Ticket{
		ID: "ticket", Title: "Review this change", Status: tickets.StatusReview,
		ActiveAttemptID: "attempt", Priority: tickets.PriorityMedium,
	}
	review := orchestrator.Review{
		Attempt: attempts.Attempt{
			ID: "attempt", Branch: "nagare/review", TargetBranch: "main", BaseCommit: "abc123",
		},
		Git: git.Review{
			Commits: 1, Stat: " file.txt | 2 +-", Diff: "--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\ncontext",
		},
	}
	model := Model{
		tickets:   []tickets.Ticket{ticket},
		column:    statusIndex(tickets.StatusReview),
		cursors:   map[tickets.Status]int{},
		todayOnly: false,
		width:     90,
		height:    18,
	}
	started, cmd := model.startReview()
	model = started.(Model)
	if !model.reviewMode || !model.reviewLoading || cmd == nil {
		t.Fatalf("review did not start: %#v, cmd=%v", model, cmd)
	}
	updated, _ := model.Update(reviewMsg{review: review})
	model = updated.(Model)
	if model.reviewLoading || len(model.reviewLines) == 0 {
		t.Fatalf("review did not load: %#v", model)
	}
	rendered := ansi.Strip(model.renderReviewDialog())
	for _, want := range []string{"Submitted diff", "nagare/review", "abc123"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("review overlay missing %q:\n%s", want, rendered)
		}
	}
	formatted := strings.Join(model.reviewLines, "\n")
	for _, want := range []string{"STAT", "DIFF", "+new"} {
		if !strings.Contains(formatted, want) {
			t.Errorf("formatted review missing %q:\n%s", want, formatted)
		}
	}
	previous := model.reviewOffset
	scrolled, _ := model.handleReviewKey(tea.KeyPressMsg{Code: tea.KeyPgDown})
	model = scrolled.(Model)
	if model.reviewOffset <= previous {
		t.Fatalf("page down did not scroll: %d -> %d", previous, model.reviewOffset)
	}
}

func TestPullRequestRequiresReviewAndConfirmation(t *testing.T) {
	reviewTicket := tickets.Ticket{
		ID: "ticket", Title: "Create PR", Status: tickets.StatusReview,
		ActiveAttemptID: "attempt", TargetBranch: "origin/main", Priority: tickets.PriorityHigh,
	}
	model := Model{
		tickets:   []tickets.Ticket{reviewTicket},
		column:    statusIndex(tickets.StatusReview),
		cursors:   map[tickets.Status]int{},
		todayOnly: false,
		width:     90,
		height:    24,
	}
	model.startPullRequest()
	if !model.prMode || model.prTicket.ID != reviewTicket.ID {
		t.Fatalf("PR confirmation did not open: %#v", model)
	}
	dialog := ansi.Strip(model.renderPullRequestDialog())
	for _, want := range []string{"Push branch", "origin/main", "non-force", "y/enter create PR"} {
		if !strings.Contains(dialog, want) {
			t.Errorf("PR dialog missing %q:\n%s", want, dialog)
		}
	}
	canceled, cmd := model.handlePullRequestKey(tea.KeyPressMsg{Code: 'n', Text: "n"})
	model = canceled.(Model)
	if model.prMode || cmd != nil {
		t.Fatalf("PR cancellation = %#v, cmd=%v", model, cmd)
	}

	model.tickets[0].Status = tickets.StatusReady
	model.column = statusIndex(tickets.StatusReady)
	model.startPullRequest()
	if model.prMode || !strings.Contains(model.statusErr, "only review") {
		t.Fatalf("non-review PR action = %#v", model)
	}
}

func TestCardShowsPersistedPullRequestNumber(t *testing.T) {
	card := ansi.Strip((Model{}).renderCard(tickets.Ticket{
		ID: "12345678-abcd", Title: "PR ready", ProjectPath: "/tmp/app",
		Priority: tickets.PriorityMedium, PullRequestNumber: 42,
	}, 48, true))
	if !strings.Contains(card, "PR #42") {
		t.Fatalf("card missing PR number:\n%s", card)
	}
}

func plainGuide(rendered string) string {
	plain := ansi.Strip(rendered)
	plain = strings.Map(func(char rune) rune {
		if char >= '\u2500' && char <= '\u257f' {
			return ' '
		}
		return char
	}, plain)
	return strings.Join(strings.Fields(plain), " ")
}
func guideCopy(page int) string {
	content := []string{boardGuidePages[page].title, boardGuidePages[page].lead}
	for _, line := range boardGuidePages[page].lines {
		content = append(content, strings.ReplaceAll(line.text, "\n", ""))
	}
	return strings.Join(content, " ")
}

func TestBoardGuideExplainsWorkflowAndManagedWorktreeLocation(t *testing.T) {
	model := Model{
		cursors:   map[tickets.Status]int{},
		todayOnly: false,
		width:     100,
		height:    30,
	}
	opened, cmd := model.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	model = opened.(Model)
	if !model.guideMode || model.guidePage != 0 || cmd != nil {
		t.Fatalf("guide open = %#v, cmd=%v", model, cmd)
	}
	first := plainGuide(model.renderGuideDialog())
	for _, want := range []string{"NAGARE / BOARD GUIDE", "BACKLOG", "READY", "RUNNING", "REVIEW", "DONE", "Start with the outcome"} {
		if !strings.Contains(first, want) {
			t.Errorf("guide first page missing %q:\n%s", want, first)
		}
	}

	second, _ := model.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	model = second.(Model)
	isolation := guideCopy(1)
	for _, want := range []string{
		"Run without touching your checkout",
		"nagare/<ticket>-<attempt>",
		"~/.local/share/nagare/workspaces/",
		"<attempt-id>/<repo>/",
		"original checkout stays",
	} {
		if !strings.Contains(isolation, want) {
			t.Errorf("guide isolation page missing %q:\n%s", want, isolation)
		}
	}

	third, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	model = third.(Model)
	review := guideCopy(2)
	for _, want := range []string{"Inspect before you publish", "Dirty worktrees cannot create a PR.", "authenticated gh CLI"} {
		if !strings.Contains(review, want) {
			t.Errorf("guide review page missing %q:\n%s", want, review)
		}
	}

	fourth, _ := model.Update(tea.KeyPressMsg{Code: '4', Text: "4"})
	model = fourth.(Model)
	finish := guideCopy(3)
	for _, want := range []string{"Close the loop, keep the history", "Archive only a clean Done worktree", "branch, commits, PR identity"} {
		if !strings.Contains(finish, want) {
			t.Errorf("guide finish page missing %q:\n%s", want, finish)
		}
	}

	closed, _ := model.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	if closed.(Model).guideMode {
		t.Fatal("question mark did not close guide")
	}
}

func TestBoardFooterAdvertisesGuide(t *testing.T) {
	footer := ansi.Strip(Model{width: 100}.renderFooter())
	if !strings.Contains(footer, "? guide") {
		t.Fatalf("footer does not advertise guide: %q", footer)
	}
}

func TestBoardGuideFitsCommonTerminalSizes(t *testing.T) {
	for _, size := range []struct {
		width  int
		height int
	}{
		{width: 110, height: 32},
		{width: 80, height: 26},
		{width: 60, height: 24},
	} {
		model := Model{width: size.width, height: size.height, guideMode: true}
		rendered := model.renderGuideDialog()
		if got, limit := lipgloss.Height(rendered), size.height-4; got > limit {
			t.Errorf("%dx%d guide height = %d, exceeds %d", size.width, size.height, got, limit)
		}
		for lineNumber, line := range strings.Split(rendered, "\n") {
			if got := lipgloss.Width(line); got > size.width {
				t.Errorf("%dx%d line %d width = %d", size.width, size.height, lineNumber, got)
			}
		}
	}
}
