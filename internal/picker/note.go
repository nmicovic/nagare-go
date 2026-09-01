package picker

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nemke/nagare-go/internal/log"
	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/theme"
)

func (m Model) noteFor(name string) string {
	if m.notes == nil {
		return ""
	}
	return m.notes.Get(name)
}

func (m Model) openNote() (tea.Model, tea.Cmd) {
	s, ok := m.selectedSession()
	if !ok {
		return m, nil
	}
	m.noteMode = true
	m.noteTarget = s
	m.noteInput.SetValue(m.noteFor(s.Name))
	m.noteInput.CursorEnd()
	m.noteInput.Focus()
	return m, nil
}

func (m Model) handleNoteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEscape:
		m.noteMode = false
		return m, nil
	case keyEnter:
		text := strings.TrimSpace(m.noteInput.Value())
		if err := m.notes.Set(m.noteTarget.Name, text); err != nil {
			log.Error("save note: %v", err)
			m.statusErr = "could not save note"
		}
		m.noteMode = false
		return m, nil
	default:
		var cmd tea.Cmd
		m.noteInput, cmd = m.noteInput.Update(msg)
		return m, cmd
	}
}

func (m Model) renderNoteOverlay() string {
	c := theme.Current().Colors
	title := lipgloss.NewStyle().Foreground(c.Primary).Bold(true).
		Render("Note: " + m.noteTarget.Name)
	hint := lipgloss.NewStyle().Foreground(c.Muted).
		Render("Enter save  Esc cancel")
	content := title + "\n\n" + m.noteInput.View() + "\n\n" + hint
	return dialogStyle().Padding(1, 2).Render(onPlane(content, c.Overlay))
}

func (m Model) noteInfoLine(s models.Session, innerWidth int) string {
	note := m.noteFor(s.Name)
	if note == "" {
		return ""
	}
	c := theme.Current().Colors
	label := lipgloss.NewStyle().Foreground(c.Muted)
	val := lipgloss.NewStyle().Foreground(c.Foreground)
	maxW := innerWidth - 10
	if maxW < 8 {
		maxW = 8
	}
	return fmt.Sprintf("  %s  %s\n", label.Render("Note  "), val.Render(truncate(note, maxW)))
}
