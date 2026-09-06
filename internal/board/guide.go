package board

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nemke/nagare-go/internal/theme"
	"github.com/nemke/nagare-go/internal/tickets"
)

type guideLine struct {
	label string
	text  string
}

type guidePage struct {
	badge string
	title string
	lead  string
	lines []guideLine
}

var boardGuidePages = []guidePage{
	{
		badge: "PLAN",
		title: "Start with the outcome",
		lead:  "A ticket carries the result you want, the repository to change, and the branch that should receive it.",
		lines: []guideLine{
			{label: "n", text: "Create a ticket. Add a clear title, useful context, repository, and target branch."},
			{label: "Backlog", text: "Keep future work here. Move a ticket with [ and ] when its next step changes."},
			{label: "Ready", text: "Use this lane for work that an agent can start without another decision."},
			{label: "Target", text: "Nagare resolves the target branch to an immutable base commit before work begins."},
		},
	},
	{
		badge: "ISOLATE",
		title: "Run without touching your checkout",
		lead:  "Every run gets its own branch, worktree, and agent pane. Your original checkout stays on its current branch.",
		lines: []guideLine{
			{label: "d", text: "Choose an agent, then optionally select its model. Empty model input uses the agent default."},
			{label: "Branch", text: "nagare/<ticket>-<attempt> records exactly which branch belongs to the run."},
			{label: "Worktree", text: "~/.local/share/nagare/workspaces/\n<attempt-id>/<repo>/"},
			{label: "Running", text: "Press Enter on the ticket to jump back to its assigned agent pane."},
		},
	},
	{
		badge: "REVIEW",
		title: "Inspect before you publish",
		lead:  "Agent submission moves the ticket to Review. Nothing is pushed merely because the agent finished.",
		lines: []guideLine{
			{label: "v", text: "Open commit counts, dirty-file counts, diff statistics, and the unified diff from the base."},
			{label: "Commit", text: "Commit remaining changes in the managed worktree. Dirty worktrees cannot create a PR."},
			{label: "p", text: "Confirm a non-force push of only the recorded branch, then create or recover its GitHub PR."},
			{label: "Requires", text: "An authenticated gh CLI and an origin remote. Retries reuse the existing pull request."},
		},
	},
	{
		badge: "FINISH",
		title: "Close the loop, keep the history",
		lead:  "Done is a human decision. Cleanup removes the disposable checkout, not the branch, commits, or pull request.",
		lines: []guideLine{
			{label: "]", text: "Move the reviewed ticket to Done after the result is accepted."},
			{label: "Pane", text: "Close the assigned agent pane before archiving its worktree."},
			{label: "c", text: "Archive only a clean Done worktree. Nagare refuses cleanup when files are still dirty."},
			{label: "Retained", text: "The branch, commits, PR identity, and attempt record remain available after archive."},
		},
	},
}

func (m Model) handleGuideKey(key string) Model {
	switch key {
	case "esc", "q", "?":
		m.guideMode = false
		m.guidePage = 0
	case "left", "h":
		m.guidePage = max(0, m.guidePage-1)
	case "right", "l", "enter", " ":
		m.guidePage = min(len(boardGuidePages)-1, m.guidePage+1)
	case "1", "2", "3", "4":
		m.guidePage = int(key[0] - '1')
	}
	return m
}

func (m Model) renderGuideDialog() string {
	colors := theme.Current().Colors
	pageIndex := min(max(0, m.guidePage), len(boardGuidePages)-1)
	page := boardGuidePages[pageIndex]
	outerWidth := min(88, max(42, m.width*4/5))
	outerWidth = min(outerWidth, max(28, m.width-4))
	innerWidth := max(22, outerWidth-6)

	brand := lipgloss.NewStyle().Foreground(colors.Primary).Bold(true).Render("NAGARE")
	context := lipgloss.NewStyle().Foreground(colors.Muted).Render("  /  BOARD GUIDE")
	counter := lipgloss.NewStyle().
		Foreground(colors.Subtle).
		Background(colors.SelBg).
		Bold(true).
		Padding(0, 1).
		Render(fmt.Sprintf("%02d OF %02d", pageIndex+1, len(boardGuidePages)))
	header := brand + context
	header += strings.Repeat(" ", max(1, innerWidth-lipgloss.Width(header)-lipgloss.Width(counter)))
	header += counter

	progress := renderGuideProgress(pageIndex, innerWidth)
	stage := lipgloss.NewStyle().Foreground(colors.Primary).Bold(true).
		Render(fmt.Sprintf("%02d", pageIndex+1))
	badge := lipgloss.NewStyle().Foreground(colors.Muted).Bold(true).
		Render("  " + page.badge)
	title := lipgloss.NewStyle().Foreground(colors.Foreground).Bold(true).
		Render("  " + page.title)
	lead := lipgloss.NewStyle().Foreground(colors.Subtle).Width(innerWidth).Render(page.lead)

	compact := innerWidth < 64 || m.height < 27
	var grid string
	if compact {
		lead = ansi.Truncate(page.lead, innerWidth, "…")
		rows := make([]string, len(page.lines))
		for index, line := range page.lines {
			rows[index] = renderCompactGuideLine(line, innerWidth)
		}
		grid = strings.Join(rows, "\n")
	} else {
		const cardGap = 2
		cardWidth := (innerWidth - cardGap) / 2
		cards := make([]string, len(page.lines))
		for index, line := range page.lines {
			cards[index] = renderGuideCard(line, cardWidth, 5)
		}
		gap := lipgloss.NewStyle().Background(colors.Overlay).Width(cardGap).Render("")
		grid = strings.Join([]string{
			lipgloss.JoinHorizontal(lipgloss.Top, cards[0], gap, cards[1]),
			lipgloss.JoinHorizontal(lipgloss.Top, cards[2], gap, cards[3]),
		}, "\n")
	}

	dots := make([]string, len(boardGuidePages))
	for index := range dots {
		dot := "○"
		style := lipgloss.NewStyle().Foreground(colors.Border)
		if index == pageIndex {
			dot = "●"
			style = style.Foreground(colors.Primary)
		}
		dots[index] = style.Render(dot)
	}
	pager := strings.Join(dots, "  ")
	hintText := "←/→ page   1–4 jump   ? close"
	hintWidth := max(10, innerWidth-lipgloss.Width(pager)-2)
	hint := lipgloss.NewStyle().Foreground(colors.Muted).
		Render(ansi.Truncate(hintText, hintWidth, "…"))
	footer := pager + strings.Repeat(" ", max(2, innerWidth-lipgloss.Width(pager)-lipgloss.Width(hint))) + hint

	sectionGap := "\n\n"
	if m.height < 27 {
		sectionGap = "\n"
	}
	body := header + sectionGap + progress + sectionGap +
		stage + badge + title + "\n" + lead + sectionGap +
		grid + sectionGap + footer
	box := lipgloss.NewStyle().
		Width(outerWidth).
		Background(colors.Overlay).
		Foreground(colors.Foreground).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForegroundBlend(colors.GradientFrom, colors.GradientTo).
		BorderBackground(colors.Overlay).
		Padding(1, 2).
		Render(theme.OnPlane(body, colors.Overlay))
	return lipgloss.Place(m.width, max(8, m.height-4), lipgloss.Center, lipgloss.Center, box)
}

func renderGuideProgress(pageIndex, width int) string {
	colors := theme.Current().Colors
	activeStatus := min(len(tickets.BoardStatuses)-1, pageIndex+1)
	parts := make([]string, 0, len(tickets.BoardStatuses)*2-1)
	for index, status := range tickets.BoardStatuses {
		if index > 0 {
			parts = append(parts, lipgloss.NewStyle().
				Foreground(colors.Border).
				Background(colors.Overlay).
				Render(" ─ "))
		}
		label := fmt.Sprintf("%s %s", statusIcon(status), strings.ToUpper(tickets.StatusLabel(status)))
		style := lipgloss.NewStyle().
			Foreground(colors.Muted).
			Background(colors.Overlay)
		if index == activeStatus {
			style = style.
				Foreground(colors.Background).
				Background(colors.Primary).
				Bold(true).
				Padding(0, 1)
		}
		parts = append(parts, style.Render(label))
	}
	return ansi.Truncate(strings.Join(parts, ""), width, "…")
}

func renderGuideCard(line guideLine, width, height int) string {
	colors := theme.Current().Colors
	label := lipgloss.NewStyle().
		Foreground(colors.Primary).
		Background(colors.Surface).
		Bold(true).
		Render(strings.ToUpper(line.label))
	text := lipgloss.NewStyle().
		Foreground(colors.Foreground).
		Background(colors.Surface).
		Width(max(8, width-4)).
		Render(line.text)
	content := label + "\n" + text
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(colors.Surface).
		Foreground(colors.Foreground).
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true).
		BorderForeground(colors.BorderFocus).
		BorderBackground(colors.Surface).
		Padding(0, 1).
		Render(theme.OnPlane(content, colors.Surface))
}

func renderCompactGuideLine(line guideLine, width int) string {
	colors := theme.Current().Colors
	labelWidth := min(11, max(7, width/5))
	label := lipgloss.NewStyle().
		Foreground(colors.Primary).
		Background(colors.Overlay).
		Bold(true).
		Width(labelWidth).
		Render(strings.ToUpper(line.label))
	textWidth := max(8, width-labelWidth-2)
	text := lipgloss.NewStyle().
		Foreground(colors.Subtle).
		Background(colors.Overlay).
		Render(ansi.Truncate(strings.ReplaceAll(line.text, "\n", " "), textWidth, "…"))
	return theme.OnPlane(label+"  "+text, colors.Overlay)
}
