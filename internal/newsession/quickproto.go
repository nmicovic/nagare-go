package newsession

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/nemke/nagare-go/internal/session"
)

// QuickModel is the quick prototype form (name + agent only).
type QuickModel struct {
	nameInput textinput.Model
	agent     string
	focus     int // 0=name, 1=agent
	width     int
	height    int
	done      bool
	result    string
	err       error
}

// NewQuick creates a new quick prototype form model.
func NewQuick() QuickModel {
	nameTi := textinput.New()
	nameTi.Placeholder = "my-prototype"
	nameTi.Focus()
	nameTi.Width = 30

	return QuickModel{
		nameInput: nameTi,
		agent:     "claude",
		focus:     0,
	}
}

func (m QuickModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m QuickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m QuickModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, tea.Quit
	case "tab":
		m.focus = (m.focus + 1) % 2
		m.updateFocus()
		return m, nil
	case "enter":
		return m.submit()
	case "left":
		if m.focus == 1 {
			m.agent = cycleAgent(quickAgents, m.agent, -1)
		}
		return m, nil
	case "right":
		if m.focus == 1 {
			m.agent = cycleAgent(quickAgents, m.agent, 1)
		}
		return m, nil
	}

	// Forward to focused text input
	var cmd tea.Cmd
	if m.focus == 0 {
		m.nameInput, cmd = m.nameInput.Update(msg)
	}
	return m, cmd
}

func (m *QuickModel) updateFocus() {
	if m.focus == 0 {
		m.nameInput.Focus()
	} else {
		m.nameInput.Blur()
	}
}

func (m QuickModel) submit() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		return m, nil
	}

	// Quick prototype always creates fresh (no continue)
	sessionName, err := session.Create(name, name, m.agent, false)
	if err != nil {
		m.err = err
		return m, nil
	}

	m.done = true
	m.result = sessionName

	// Switch to the session
	session.SwitchToSession(sessionName)

	return m, tea.Quit
}

func (m QuickModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	title := renderTitle("Quick Prototype")
	nameField := "  Name:  " + m.nameInput.View()

	content := strings.Join([]string{
		"",
		nameField,
		"",
		renderAgentPicker(quickAgents, m.agent, m.focus == 1),
		"",
		renderError(m.err),
		renderHint("Enter: Create  Tab: Next  Esc: Cancel"),
	}, "\n")

	return renderCenteredBox(title, content, m.width, m.height)
}
