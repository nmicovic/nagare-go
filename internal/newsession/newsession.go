package newsession

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nemke/nagare-go/internal/session"
	"github.com/nemke/nagare-go/internal/theme"
)

// Model is the full new session form.
type Model struct {
	pathInput       textinput.Model
	nameInput       textinput.Model
	agent           string
	continueSession bool
	focus           int // 0=path, 1=name, 2=agent, 3=continue
	suggestions     []string
	sugCursor       int
	lastPathValue   string // cached to avoid redundant ListDirectories
	width           int
	height          int
	done            bool
	result          string
	err             error
}

// New creates a new session form model.
func New() Model {
	pathTi := textinput.New()
	pathTi.Placeholder = "~/Projects/my-project"
	pathTi.Focus()
	pathTi.Width = 40

	nameTi := textinput.New()
	nameTi.Placeholder = "my-project"
	nameTi.Width = 40

	return Model{
		pathInput:       pathTi,
		nameInput:       nameTi,
		agent:           "claude",
		continueSession: true,
		focus:           0,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, tea.Quit
	case "tab":
		m.focus = (m.focus + 1) % 4
		m.updateFocus()
		return m, nil
	case "shift+tab":
		m.focus = (m.focus + 3) % 4
		m.updateFocus()
		return m, nil
	case "enter":
		if m.focus == 0 && len(m.suggestions) > 0 {
			// Accept suggestion
			m.pathInput.SetValue(m.suggestions[m.sugCursor])
			m.suggestions = nil
			// Auto-fill name from path
			base := filepath.Base(m.pathInput.Value())
			if base != "" && m.nameInput.Value() == "" {
				m.nameInput.SetValue(base)
			}
			m.focus = 1
			m.updateFocus()
			return m, nil
		}
		return m.submit()
	case "up":
		if m.focus == 0 && len(m.suggestions) > 0 {
			if m.sugCursor > 0 {
				m.sugCursor--
			}
			return m, nil
		}
	case "down":
		if m.focus == 0 && len(m.suggestions) > 0 {
			if m.sugCursor < len(m.suggestions)-1 {
				m.sugCursor++
			}
			return m, nil
		}
	case "left":
		if m.focus == 2 {
			m.agent = cycleAgent(agents, m.agent, -1)
			return m, nil
		}
		if m.focus == 3 {
			m.continueSession = !m.continueSession
			return m, nil
		}
	case "right":
		if m.focus == 2 {
			m.agent = cycleAgent(agents, m.agent, 1)
			return m, nil
		}
		if m.focus == 3 {
			m.continueSession = !m.continueSession
			return m, nil
		}
	}

	// Forward to focused text input
	var cmd tea.Cmd
	switch m.focus {
	case 0:
		m.pathInput, cmd = m.pathInput.Update(msg)
		if v := m.pathInput.Value(); v != m.lastPathValue {
			m.lastPathValue = v
			m.suggestions = session.ListDirectories(v, 5)
			m.sugCursor = 0
		}
	case 1:
		m.nameInput, cmd = m.nameInput.Update(msg)
	}
	return m, cmd
}

func (m *Model) updateFocus() {
	m.pathInput.Blur()
	m.nameInput.Blur()

	switch m.focus {
	case 0:
		m.pathInput.Focus()
	case 1:
		m.nameInput.Focus()
	}
}

// resolvedPath returns the effective working directory that will be created,
// mirroring the logic in session.Create so the user sees exactly where the
// session will land before pressing Enter.
func (m Model) resolvedPath() string {
	path := m.pathInput.Value()
	if path == "" {
		return ""
	}
	name := m.nameInput.Value()
	if name != "" {
		path = filepath.Join(path, name)
	}
	return session.ExpandPath(session.ResolvePath(path))
}

func (m Model) submit() (tea.Model, tea.Cmd) {
	path := m.pathInput.Value()
	if path == "" {
		return m, nil
	}
	name := m.nameInput.Value()

	sessionName, err := session.Create(path, name, m.agent, m.continueSession)
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

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	c := theme.Current().Colors
	title := renderTitle("New Session")

	// Path field
	pathField := "  Path:  " + m.pathInput.View()

	// Suggestions
	var sugStr string
	if len(m.suggestions) > 0 && m.focus == 0 {
		var sugLines []string
		for i, s := range m.suggestions {
			line := "    " + s
			if i == m.sugCursor {
				line = lipgloss.NewStyle().
					Background(c.Primary).
					Foreground(c.Background).
					Render("  → " + s)
			}
			sugLines = append(sugLines, line)
		}
		sugStr = strings.Join(sugLines, "\n") + "\n"
	}

	// Name field
	nameField := "  Name:  " + m.nameInput.View()

	// Resolved directory preview — show full target path + exists/new badge
	var dirLine string
	if rp := m.resolvedPath(); rp != "" {
		_, statErr := os.Stat(rp)
		var badge string
		if os.IsNotExist(statErr) {
			badge = lipgloss.NewStyle().Foreground(c.Warning).Render(" (will be created)")
		} else {
			badge = lipgloss.NewStyle().Foreground(c.Success).Render(" (exists)")
		}
		dirLine = "  →  " + lipgloss.NewStyle().Foreground(c.Accent).Bold(true).Render(rp) + badge
	}

	// Agent field
	agentStr := renderAgentPicker(agents, m.agent, m.focus == 2)

	// Continue field
	continueStr := "  "
	if m.continueSession {
		continueStr += "[x] Continue previous session"
	} else {
		continueStr += "[ ] Continue previous session"
	}
	if m.focus == 3 {
		continueStr = lipgloss.NewStyle().
			Background(c.Primary).
			Foreground(c.Background).
			Render(continueStr)
	}

	hint := renderHint("Enter: Create  Tab: Next  Esc: Cancel")
	errStr := renderError(m.err)

	content := strings.Join([]string{
		"",
		pathField,
		sugStr,
		nameField,
		dirLine,
		"",
		agentStr,
		"",
		continueStr,
		"",
		errStr,
		hint,
	}, "\n")

	return renderCenteredBox(title, content, m.width, m.height)
}
