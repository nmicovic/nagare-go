package picker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nemke/nagare-go/internal/config"
	"github.com/nemke/nagare-go/internal/log"
	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/session"
	"github.com/nemke/nagare-go/internal/state"
	"github.com/nemke/nagare-go/internal/theme"
	"github.com/nemke/nagare-go/internal/tmux"
	"github.com/sahilm/fuzzy"
)

// ViewMode controls the session display layout.
type ViewMode int

const (
	ListView ViewMode = iota
	GridView
)

// SortMode controls the session sort order.
type SortMode int

const (
	SortByStatus SortMode = iota
	SortByName
	SortByAgent
)

// Message types for async updates.
type SessionsUpdatedMsg []models.Session
type PreviewUpdatedMsg string

type tickScanMsg struct{}
type tickPreviewMsg struct{}
type tickPulseMsg struct{}
type gridPreviewsMsg map[string]string

// Picker exit actions.
const (
	ActionNone       = ""
	ActionNew        = "new"
	ActionQuickProto = "quickproto"
)

// Result is returned when the picker exits with a special action.
type Result struct {
	Action string
	Target string
}

// Model is the main Bubble Tea model for the picker TUI.
type Model struct {
	sessions      []models.Session
	filtered      []models.Session
	cursor        int
	viewMode      ViewMode
	sortMode      SortMode
	preview       string
	width         int
	height        int
	statesDir     string
	searchInput   textinput.Model
	showHelp      bool              // F1 help overlay
	showHelpBar   bool              // bottom hint bar
	showThemePick bool              // Ctrl+t theme picker overlay
	themeNames    []string          // cached sorted theme names
	themeCursor   int               // cursor in theme picker
	themeOriginal string            // theme before opening picker (for cancel)
	showSaved     bool              // show saved (unloaded) sessions
	gridPreviews  map[string]string // cached grid cell previews keyed by pane target
	registry      *state.Registry
	renameMode    bool
	renameSession models.Session
	worktreeMode  bool
	worktreeOn    models.Session
	result        Result
	promptMode    bool
	promptTarget  models.Session
	promptInput   textinput.Model
	lastQuery     string   // previous search query, to detect query changes in applyFilter
	gridOrder     []string // frozen display order for grid view (session keys); nil = not yet snapshotted
	pulseOn       bool     // 1Hz toggle used to breathe the status dot on running/waiting sessions
	testNoScan    bool     // test hook: disable the live tmux scanner (see export_test.go)
}

// New creates a new picker model with default settings.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "search sessions..."
	ti.Prompt = " > "
	ti.CharLimit = 64

	ti.Focus()

	pi := textinput.New()
	pi.Placeholder = "type prompt to send..."
	pi.CharLimit = 500
	pi.SetWidth(60)

	cfg, _ := config.Load()

	return Model{
		statesDir:   state.DefaultStatesDir(),
		searchInput: ti,
		showHelpBar: cfg.Picker.ShowHelpBar,
		registry:    state.NewRegistry(state.DefaultRegistryPath()),
		promptInput: pi,
	}
}

// markDead writes a dead state for a session before killing it.
func markDead(s models.Session, statesDir string) {
	state.WriteState(statesDir, models.SessionState{
		State:     string(models.StatusDead),
		SessionID: s.SessionID,
		Cwd:       s.Path,
		Event:     "ManualKill",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// selectedSession returns the currently selected session, if any.
func (m Model) selectedSession() (models.Session, bool) {
	if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		return models.Session{}, false
	}
	return m.filtered[m.cursor], true
}

// approvable reports whether Ctrl+y should send Enter to a session in this
// status. Includes StatusRunning because the hook state lags: Claude fires
// PreToolUse ("working") first, then Notification/permission_prompt
// ("waiting_input") once the dialog is actually rendered — the yellow→red
// transition can take a few seconds. Allowing approval during "working"
// covers that gap. Idle/dead/saved sessions are skipped so a stray Ctrl+y
// doesn't submit an empty prompt or hit a shell.
func approvable(status models.SessionStatus) bool {
	return status == models.StatusWaitingInput || status == models.StatusRunning
}

// sessionKey returns a stable identifier for a session. The cursor tracks this
// key across re-filters so the selection follows the session, not the index.
func sessionKey(s models.Session) string {
	if s.Status == models.StatusSaved {
		return "saved:" + s.Name
	}
	return tmux.PaneTarget(s.SessionName, s.WindowIndex, s.PaneIndex)
}

// snapshotGridOrder captures the current m.filtered order into gridOrder so
// subsequent scans preserve cell positions in grid view.
func (m *Model) snapshotGridOrder() {
	m.gridOrder = make([]string, len(m.filtered))
	for i, s := range m.filtered {
		m.gridOrder[i] = sessionKey(s)
	}
}

// applyGridOrder rebuilds m.filtered honoring the frozen gridOrder: existing
// sessions keep their slot, sessions not in the snapshot (newly appeared) are
// appended, and sessions no longer visible are dropped. gridOrder is updated
// to the new final order.
func (m *Model) applyGridOrder(visible []models.Session) {
	byKey := make(map[string]models.Session, len(visible))
	for _, s := range visible {
		byKey[sessionKey(s)] = s
	}

	result := make([]models.Session, 0, len(visible))
	order := make([]string, 0, len(visible))

	for _, k := range m.gridOrder {
		if s, ok := byKey[k]; ok {
			result = append(result, s)
			order = append(order, k)
			delete(byKey, k)
		}
	}
	for _, s := range visible {
		k := sessionKey(s)
		if _, still := byKey[k]; still {
			result = append(result, s)
			order = append(order, k)
		}
	}

	m.filtered = result
	m.gridOrder = order
}

// isStarred returns whether a session is starred in the registry. A Model
// without a registry has no stars rather than panicking, which keeps sorting
// testable in isolation.
func (m Model) isStarred(name string) bool {
	if m.registry == nil {
		return false
	}
	s := m.registry.Find(name)
	return s != nil && s.Starred
}

// Result returns the picker's result (action to take after quitting).
func (m Model) Result() Result {
	return m.result
}

func (m Model) Init() tea.Cmd {
	if m.testNoScan {
		return nil
	}
	return tea.Batch(
		doScan(m.statesDir),
		doPreviewTick(),
		doPulseTick(),
	)
}

// doPulseTick fires once per second. The handler flips a bool that the
// status-dot renderer consults to alternate Faint on running/waiting
// sessions — a low-amplitude "breathing" pulse that reads as active work
// without the noise of a spinning cursor.
func doPulseTick() tea.Cmd {
	return tea.Tick(1*time.Second, func(time.Time) tea.Msg { return tickPulseMsg{} })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case SessionsUpdatedMsg:
		m.sessions = []models.Session(msg)
		m.mergeSavedSessions()
		log.Debug("scan: %d sessions (%d saved)", len(m.sessions), m.countSaved())
		m.applyFilter()
		if m.testNoScan {
			return m, nil
		}
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return tickScanMsg{} })

	case PreviewUpdatedMsg:
		m.preview = string(msg)
		return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return tickPreviewMsg{} })

	case gridPreviewsMsg:
		m.gridPreviews = map[string]string(msg)
		return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return tickPreviewMsg{} })

	case tickScanMsg:
		return m, doScan(m.statesDir)

	case tickPreviewMsg:
		return m, m.doPreview()

	case tickPulseMsg:
		m.pulseOn = !m.pulseOn
		return m, doPulseTick()

	case tea.KeyMsg:
		return m.handleKey(msg)

	case editorDoneMsg:
		defer os.Remove(msg.path)
		if msg.err != nil {
			log.Error("editor prompt failed: %v", msg.err)
			return m, nil
		}
		data, err := os.ReadFile(msg.path)
		if err != nil {
			log.Error("editor prompt read: %v", err)
			return m, nil
		}
		text := strings.TrimSpace(string(data))
		if text != "" {
			sendPromptToPane(m.promptTarget, text)
			log.Info("editor prompt sent to %s", m.promptTarget.Name)
		}
		return m, nil

	case configEditDoneMsg:
		if msg.err != nil {
			log.Error("config editor failed: %v", msg.err)
		}
		// Config may have changed — reload theme
		if cfg, err := config.Load(); err == nil {
			theme.Set(cfg.Appearance.Theme)
		}
		return m, nil
	}

	return m, nil
}

func (m Model) View() tea.View {
	content := m.view()
	// Last line of defense. Every panel is already pinned with fitBox, but a
	// frame even one row too tall scrolls the alt screen and smears the whole
	// UI, so clamp the assembled frame rather than trusting the arithmetic.
	if m.width > 0 && m.height > 0 {
		content = lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(content)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m Model) view() string {
	if m.width == 0 {
		return "Loading..."
	}

	// Render the help bar first and measure it. It soft-wraps, and how many
	// lines it takes depends on the terminal width — assuming a fixed two
	// pushed the panels off the bottom of a narrow terminal.
	bar := ""
	contentHeight := m.height
	if m.showHelpBar {
		bar = helpBar(m.width)
		contentHeight = m.height - lipgloss.Height(bar)
		if contentHeight < 1 {
			contentHeight = 1
		}
	}

	var base string
	if m.viewMode == GridView {
		base = m.viewGrid(m.width, contentHeight)
	} else {
		leftOuter := m.width / 5
		rightOuter := m.width - leftOuter
		left := m.viewLeft(leftOuter, contentHeight)
		right := m.viewRight(rightOuter, contentHeight)
		base = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}

	if m.showHelpBar {
		base = base + "\n" + bar
	}

	// Overlays drawn on top of base content
	if m.showHelp {
		overlay := helpOverlay(m.width, m.height)
		return placeOverlay(m.width, m.height, overlay, base)
	}
	if m.showThemePick {
		overlay := themePickOverlay(m.themeNames, m.themeCursor, m.width, m.height)
		return placeOverlay(m.width, m.height, overlay, base)
	}
	if m.promptMode {
		overlay := m.renderPromptOverlay()
		return placeOverlay(m.width, m.height, overlay, base)
	}

	return base
}

// renderPromptOverlay renders the inline prompt dialog.
func (m Model) renderPromptOverlay() string {
	c := theme.Current().Colors
	title := lipgloss.NewStyle().Foreground(c.Primary).Bold(true).
		Render("Send to: " + m.promptTarget.Name)
	hint := lipgloss.NewStyle().Foreground(c.Muted).
		Render("Enter send  Esc cancel")
	content := title + "\n\n" + m.promptInput.View() + "\n\n" + hint
	return dialogStyle().Padding(1, 2).Render(content)
}

// --- Key handling ---

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	log.Debug("key: %q", key)

	// Theme picker intercepts all keys when open
	if m.showThemePick {
		return m.handleThemePickKey(key)
	}

	// Rename mode intercepts keys before normal handling
	if m.renameMode {
		return m.handleRenameKey(msg)
	}

	// Worktree-name entry intercepts keys the same way
	if m.worktreeMode {
		return m.handleWorktreeKey(msg)
	}

	// Prompt mode intercepts keys
	if m.promptMode {
		return m.handlePromptKey(msg)
	}

	switch key {
	case keyHelp:
		m.showHelp = !m.showHelp
		return m, nil
	case keyEscape:
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		return m, tea.Quit
	case keyEnter:
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			if s.Status == models.StatusSaved {
				agent := string(s.AgentType)
				if agent == "" || agent == "unknown" {
					agent = "claude"
				}
				name, err := session.Load(s.Path, s.Name, agent)
				if err != nil {
					log.Error("load session: %v", err)
					return m, nil
				}
				session.SwitchToSession(name)
				return m, tea.Quit
			}
			session.SwitchToPane(s)
			return m, tea.Quit
		}
	case keyUp:
		if m.viewMode == GridView {
			cols := gridColumns(len(m.filtered))
			if m.cursor-cols >= 0 {
				m.cursor -= cols
			}
		} else if m.cursor > 0 {
			m.cursor--
		}
		return m, m.doPreview()
	case keyDown:
		if m.viewMode == GridView {
			cols := gridColumns(len(m.filtered))
			if m.cursor+cols < len(m.filtered) {
				m.cursor += cols
			}
		} else if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
		return m, m.doPreview()
	case "left":
		if m.viewMode == GridView && m.cursor > 0 {
			m.cursor--
		}
		return m, m.doPreview()
	case "right":
		if m.viewMode == GridView && m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
		return m, m.doPreview()
	case keyToggleView:
		if m.viewMode == ListView {
			m.viewMode = GridView
			// Fresh snapshot: start grid from current sorted order, then freeze.
			m.gridOrder = nil
			m.applyFilter()
			log.Info("switched to grid view")
		} else {
			m.viewMode = ListView
			m.gridOrder = nil
			log.Info("switched to list view")
		}
		return m, nil
	case keyCycleTheme:
		m.showThemePick = true
		m.themeNames = theme.Names()
		m.themeOriginal = theme.Current().Name
		// Set cursor to current theme
		for i, name := range m.themeNames {
			if name == m.themeOriginal {
				m.themeCursor = i
				break
			}
		}
		return m, nil
	case keyApprove:
		if s, ok := m.selectedSession(); ok {
			if approvable(s.Status) {
				tmux.RunTmux("send-keys", "-t", tmux.PaneTarget(s.SessionName, s.WindowIndex, s.PaneIndex), "Enter")
				log.Info("approved %s (status=%s)", s.Name, s.Status)
			} else {
				log.Info("approve ignored: %s is %s", s.Name, s.Status)
			}
		}
		return m, nil
	case keyApproveAlways:
		if s, ok := m.selectedSession(); ok {
			if approvable(s.Status) {
				tmux.RunTmux("send-keys", "-t", tmux.PaneTarget(s.SessionName, s.WindowIndex, s.PaneIndex), "Down", "Enter")
				log.Info("approved always %s (status=%s)", s.Name, s.Status)
			} else {
				log.Info("approve-always ignored: %s is %s", s.Name, s.Status)
			}
		}
		return m, nil
	case keyUnload:
		if s, ok := m.selectedSession(); ok {
			markDead(s, m.statesDir)
			tmux.RunTmux("kill-pane", "-t", tmux.PaneTarget(s.SessionName, s.WindowIndex, s.PaneIndex))
			log.Info("unloaded pane %s", s.Name)
			return m, doScan(m.statesDir)
		}
		return m, nil
	case keyKillSession:
		if s, ok := m.selectedSession(); ok {
			markDead(s, m.statesDir)
			tmux.RunTmux("kill-session", "-t", s.SessionName)
			log.Info("killed session %s", s.Name)
			return m, doScan(m.statesDir)
		}
		return m, nil
	case keyToggleSaved:
		m.showSaved = !m.showSaved
		m.applyFilter()
		return m, nil
	case keyStar:
		if s, ok := m.selectedSession(); ok {
			// Auto-register if not in registry
			if m.registry.Find(s.Name) == nil {
				m.registry.Register(s.Name, s.Path, string(s.AgentType))
			}
			starred := m.registry.ToggleStar(s.Name)
			if starred {
				log.Info("starred %s", s.Name)
			} else {
				log.Info("unstarred %s", s.Name)
			}
			// Refresh registry after toggle
			m.registry = state.NewRegistry(state.DefaultRegistryPath())
		}
		return m, nil
	case keyCycleSort:
		switch m.sortMode {
		case SortByStatus:
			m.sortMode = SortByName
		case SortByName:
			m.sortMode = SortByAgent
		case SortByAgent:
			m.sortMode = SortByStatus
		}
		// Clear the grid snapshot so the new sort order takes effect, then
		// applyFilter re-snapshots and the grid freezes on that order.
		m.gridOrder = nil
		m.applyFilter()
		log.Info("sort mode: %d", m.sortMode)
		return m, nil
	case keyRename:
		if s, ok := m.selectedSession(); ok {
			m.renameMode = true
			m.renameSession = s
			m.searchInput.SetValue(s.Name)
			m.searchInput.CursorEnd()
		}
		return m, nil
	case keyNewWorktree:
		if s, ok := m.selectedSession(); ok {
			m.worktreeMode = true
			m.worktreeOn = s
			m.searchInput.SetValue("")
		}
		return m, nil
	case keyNewSession:
		m.result = Result{Action: ActionNew}
		return m, tea.Quit
	case keyQuickProto:
		m.result = Result{Action: ActionQuickProto}
		return m, tea.Quit
	case keyInlinePrompt:
		if s, ok := m.selectedSession(); ok {
			m.promptMode = true
			m.promptTarget = s
			m.promptInput.SetValue("")
			m.promptInput.Focus()
		}
		return m, nil
	case keyEditPrompt:
		if s, ok := m.selectedSession(); ok {
			m.promptTarget = s
			return m, m.openEditorPrompt()
		}
		return m, nil
	case keyEditConfig:
		return m, m.openConfigEditor()
	default:
		// All other keys go to search input
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.applyFilter()
		return m, cmd
	}

	return m, nil
}

// handleWorktreeKey handles key input while naming a new worktree. The name is
// typed into the search input, the same field rename mode borrows.
func (m Model) handleWorktreeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEscape:
		m.worktreeMode = false
		m.searchInput.SetValue("")
		return m, nil
	case keyEnter:
		name := strings.TrimSpace(m.searchInput.Value())
		m.worktreeMode = false
		m.searchInput.SetValue("")
		if name == "" {
			return m, nil
		}
		// The agent inherits from the session the worktree is spawned off, so
		// a repo full of Claude panes keeps getting Claude panes.
		sessName, err := session.CreateWorktree(m.worktreeOn.Path, name, string(m.worktreeOn.AgentType))
		if err != nil {
			log.Info("worktree %q failed: %v", name, err)
			return m, nil
		}
		log.Info("created worktree %s in session %s", name, sessName)
		return m, doScan(m.statesDir)
	default:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
}

// handleRenameKey handles key input during rename mode.
func (m Model) handleRenameKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case keyEscape:
		// Cancel rename
		m.renameMode = false
		m.searchInput.SetValue("")
		return m, nil
	case keyEnter:
		// Finish rename
		newName := strings.TrimSpace(m.searchInput.Value())
		m.renameMode = false
		m.searchInput.SetValue("")

		if newName == "" || newName == m.renameSession.Name {
			return m, nil
		}

		oldName := m.renameSession.SessionName

		// Check if name already exists
		existing := tmux.RunTmux("list-sessions", "-F", "#{session_name}")
		for _, line := range strings.Split(existing, "\n") {
			if strings.TrimSpace(line) == newName {
				log.Info("rename failed: %s already exists", newName)
				return m, nil
			}
		}

		// Count agents in the same tmux session (multi-agent check)
		count := 0
		for _, s := range m.sessions {
			if s.SessionName == oldName {
				count++
			}
		}

		if count > 1 {
			// Rename just the window
			target := fmt.Sprintf("%s:%d", oldName, m.renameSession.WindowIndex)
			tmux.RunTmux("rename-window", "-t", target, newName)
			log.Info("renamed window %s -> %s", target, newName)
		} else {
			// Rename the tmux session
			tmux.RunTmux("rename-session", "-t", oldName, newName)
			log.Info("renamed session %s -> %s", oldName, newName)

			// Update registry
			if existing := m.registry.Find(oldName); existing != nil {
				path := existing.Path
				agent := existing.Agent
				m.registry.Remove(oldName)
				m.registry.Register(newName, path, agent)
			}
		}

		return m, doScan(m.statesDir)
	default:
		// Forward to search input for text editing
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
}

// handlePromptKey handles key input during prompt mode.
func (m Model) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case keyEscape:
		m.promptMode = false
		return m, nil
	case keyEnter:
		text := strings.TrimSpace(m.promptInput.Value())
		if text != "" {
			sendPromptToPane(m.promptTarget, text)
			log.Info("prompt sent to %s", m.promptTarget.Name)
		}
		m.promptMode = false
		return m, nil
	default:
		var cmd tea.Cmd
		m.promptInput, cmd = m.promptInput.Update(msg)
		return m, cmd
	}
}

// sendPromptToPane sends text to a session's tmux pane.
func sendPromptToPane(s models.Session, text string) {
	target := tmux.PaneTarget(s.SessionName, s.WindowIndex, s.PaneIndex)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i < len(lines)-1 {
			// Intermediate lines: send text then Enter separately
			tmux.RunTmux("send-keys", "-t", target, line, "")
			tmux.RunTmux("send-keys", "-t", target, "Enter", "")
		} else {
			// Last line: send with Enter to execute
			tmux.RunTmux("send-keys", "-t", target, line, "Enter")
		}
	}
}

// editorDoneMsg is sent when the editor process completes.
type editorDoneMsg struct {
	path string
	err  error
}

// configEditDoneMsg is sent when the config editor process completes.
type configEditDoneMsg struct {
	err error
}

func resolveEditor() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	return "vi"
}

// openEditorPrompt opens $EDITOR with a temp file for composing a prompt.
func (m Model) openEditorPrompt() tea.Cmd {
	editor := resolveEditor()

	// Create temp file
	tmpFile, err := os.CreateTemp("", "nagare-prompt-*.md")
	if err != nil {
		log.Error("editor prompt: %v", err)
		return nil
	}
	tmpFile.Close()
	tmpPath := tmpFile.Name()

	c := exec.Command(editor, tmpPath)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorDoneMsg{path: tmpPath, err: err}
	})
}

// openConfigEditor opens $EDITOR with the config file.
func (m Model) openConfigEditor() tea.Cmd {
	editor := resolveEditor()

	cfgPath := config.DefaultPath()
	// Ensure file exists
	os.MkdirAll(filepath.Dir(cfgPath), 0755)
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		// Write defaults
		cfg := config.Default()
		config.Save(cfg)
	}

	c := exec.Command(editor, cfgPath)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return configEditDoneMsg{err: err}
	})
}

// --- Commands ---

func doScan(statesDir string) tea.Cmd {
	return func() tea.Msg {
		paneStates := state.LoadStatesByPaneID(statesDir)
		cwdStates := state.LoadAllStates(statesDir)
		sessions := tmux.ScanSessions(paneStates, cwdStates)
		return SessionsUpdatedMsg(sessions)
	}
}

func doPreviewTick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return tickPreviewMsg{} })
}

func (m Model) doPreview() tea.Cmd {
	if len(m.filtered) == 0 {
		return func() tea.Msg { return PreviewUpdatedMsg("") }
	}

	// Grid view: capture all visible sessions in background
	if m.viewMode == GridView {
		sessions := make([]models.Session, len(m.filtered))
		copy(sessions, m.filtered)
		return func() tea.Msg {
			previews := make(map[string]string)
			for _, s := range sessions {
				target := tmux.PaneTarget(s.SessionName, s.WindowIndex, s.PaneIndex)
				previews[target] = CapturePreview(s.SessionName, s.WindowIndex, s.PaneIndex)
			}
			return gridPreviewsMsg(previews)
		}
	}

	// List view: capture only the selected session
	s := m.filtered[m.cursor]
	return func() tea.Msg {
		content := CapturePreview(s.SessionName, s.WindowIndex, s.PaneIndex)
		return PreviewUpdatedMsg(content)
	}
}

// --- Saved session merging ---

// mergeSavedSessions appends registry sessions that aren't currently running.
func (m *Model) mergeSavedSessions() {
	active := make(map[string]bool)
	for _, s := range m.sessions {
		active[s.SessionName] = true
	}
	for _, rs := range m.registry.ListAll() {
		if active[rs.Name] {
			continue
		}
		m.sessions = append(m.sessions, models.Session{
			Name:        rs.Name,
			SessionName: rs.Name,
			Path:        rs.Path,
			Status:      models.StatusSaved,
			AgentType:   models.AgentType(rs.Agent),
		})
	}
}

func (m *Model) countSaved() int {
	n := 0
	for _, s := range m.sessions {
		if s.Status == models.StatusSaved {
			n++
		}
	}
	return n
}

// --- Filtering & sorting ---

func (m *Model) applyFilter() {
	// Remember which session the cursor points at so we can restore the
	// selection after rebuilding the filtered list. Without this, a background
	// scan (every 2s) re-sorts and the cursor index silently slides onto a
	// different session — making Ctrl+y approvals land on (or miss) the
	// wrong target.
	prevKey := ""
	if s, ok := m.selectedSession(); ok {
		prevKey = sessionKey(s)
	}

	// Start with visible sessions (hide saved unless toggled)
	visible := m.sessions
	if !m.showSaved {
		visible = make([]models.Session, 0, len(m.sessions))
		for _, s := range m.sessions {
			if s.Status != models.StatusSaved {
				visible = append(visible, s)
			}
		}
	}

	query := m.searchInput.Value()
	queryChanged := query != m.lastQuery
	m.lastQuery = query

	if query != "" {
		// Build search targets: "name path" for each session
		targets := make([]string, len(visible))
		for i, s := range visible {
			targets[i] = s.Name + " " + s.Path
		}

		matches := fuzzy.Find(query, targets)
		m.filtered = make([]models.Session, len(matches))
		for i, match := range matches {
			m.filtered[i] = visible[match.Index]
		}
	} else if m.viewMode == GridView && len(m.gridOrder) > 0 {
		// Grid view with a live snapshot: preserve cell positions so scans
		// don't shuffle the grid under the user's cursor. A fresh snapshot
		// is taken on Tab-into-grid and on Ctrl+o sort cycles.
		m.applyGridOrder(visible)
	} else {
		m.filtered = make([]models.Session, len(visible))
		copy(m.filtered, visible)
		m.sortFiltered()
		if m.viewMode == GridView {
			m.snapshotGridOrder()
		}
	}

	// Cursor resolution: if the user just changed the query, jump to the top
	// (best fuzzy match). Otherwise, follow the previously-selected session to
	// its new index so background scans don't drift the selection.
	if queryChanged {
		m.cursor = 0
	} else if prevKey != "" {
		for i, s := range m.filtered {
			if sessionKey(s) == prevKey {
				m.cursor = i
				break
			}
		}
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

// sortFiltered orders m.filtered into the order it is displayed in. Sessions
// sharing a tmux session name are kept contiguous so the list view can put a
// single header above them, and each group takes the position of its most
// urgent member — a waiting worktree lifts its whole repo rather than being
// buried inside a quiet group.
//
// A lone session is a group of one and is its own representative, so a list
// with nothing to group sorts exactly as a flat list always did.
func (m *Model) sortFiltered() {
	// Pre-build starred set to avoid per-comparison registry lookups
	starred := make(map[string]bool)
	for _, s := range m.filtered {
		starred[s.Name] = m.isStarred(s.Name)
	}

	// less is the comparator the flat list has always used: stars first, then
	// the active sort mode.
	less := func(a, b models.Session) bool {
		if sa, sb := starred[a.Name], starred[b.Name]; sa != sb {
			return sa
		}
		switch m.sortMode {
		case SortByName:
			return a.Name < b.Name
		case SortByAgent:
			return a.AgentType < b.AgentType
		default: // SortByStatus
			return statusOrder(a.Status) < statusOrder(b.Status)
		}
	}

	// Urgency is independent of the active sort mode: a waiting child has to
	// lift its group even when sorting by name.
	moreUrgent := func(a, b models.Session) bool {
		if sa, sb := starred[a.Name], starred[b.Name]; sa != sb {
			return sa
		}
		return statusOrder(a.Status) < statusOrder(b.Status)
	}

	var keys []string
	groups := make(map[string][]models.Session)
	for _, s := range m.filtered {
		key := groupKeyOf(s)
		if _, seen := groups[key]; !seen {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], s)
	}

	reps := make(map[string]models.Session, len(groups))
	for key, members := range groups {
		sort.SliceStable(members, func(i, j int) bool { return less(members[i], members[j]) })
		groups[key] = members

		rep := members[0]
		for _, s := range members[1:] {
			if moreUrgent(s, rep) {
				rep = s
			}
		}
		reps[key] = rep
	}

	sort.SliceStable(keys, func(i, j int) bool {
		a, b := reps[keys[i]], reps[keys[j]]
		if less(a, b) {
			return true
		}
		if less(b, a) {
			return false
		}
		return keys[i] < keys[j] // stable tie-break so the list never jitters
	})

	ordered := make([]models.Session, 0, len(m.filtered))
	for _, key := range keys {
		ordered = append(ordered, groups[key]...)
	}
	m.filtered = ordered
}

func statusOrder(s models.SessionStatus) int {
	switch s {
	case models.StatusWaitingInput:
		return 0
	case models.StatusRunning:
		return 1
	case models.StatusIdle:
		return 2
	case models.StatusDead:
		return 3
	case models.StatusSaved:
		return 4
	default:
		return 5
	}
}

// --- View rendering ---

// renderStats renders the header line above the search box. Each count is
// tinted with the same color as its status dot, so the eye can read "is
// anything waiting on me?" from color alone without parsing the numbers.
// Zero counts are omitted rather than rendered as "0 waiting" noise.
func (m Model) renderStats(live, waiting, running, saved int) string {
	c := theme.Current().Colors

	shown := live
	if m.showSaved {
		shown += saved
	}

	parts := []string{
		lipgloss.NewStyle().Foreground(c.Foreground).Render(fmt.Sprintf("%d sessions", shown)),
	}
	count := func(n int, label string, status models.SessionStatus) {
		if n == 0 {
			return
		}
		styled := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(status)))
		parts = append(parts, styled.Render(fmt.Sprintf("%d %s", n, label)))
	}
	count(waiting, "waiting", models.StatusWaitingInput)
	count(running, "running", models.StatusRunning)
	if saved > 0 && !m.showSaved {
		parts = append(parts, mutedStyle().Render(fmt.Sprintf("%d saved", saved)))
	}

	sep := mutedStyle().Render(" · ")
	return " " + strings.Join(parts, sep)
}

func (m Model) viewLeft(outerWidth, outerHeight int) string {
	innerWidth := outerWidth - 4
	if innerWidth < 10 {
		innerWidth = 10
	}

	var b strings.Builder

	// Dashboard stats. Counts describe what the list actually shows: saved
	// sessions are hidden unless toggled, so folding them into "N sessions"
	// made the header disagree with the rows underneath it (34 sessions,
	// three visible). Saved sessions get their own muted count instead.
	live, waiting, running, saved := 0, 0, 0, 0
	for _, s := range m.sessions {
		switch s.Status {
		case models.StatusSaved:
			saved++
		case models.StatusWaitingInput:
			waiting++
		case models.StatusRunning:
			running++
		}
		if s.Status != models.StatusSaved {
			live++
		}
	}
	b.WriteString(m.renderStats(live, waiting, running, saved))
	b.WriteString("\n\n")

	// Search input (always active)
	if m.renameMode {
		m.searchInput.Prompt = " Rename: "
	} else if m.worktreeMode {
		m.searchInput.Prompt = " New worktree: "
	} else {
		m.searchInput.Prompt = " > "
	}
	b.WriteString(m.searchInput.View())
	b.WriteString("\n\n")

	// Session list.
	// In lipgloss v2, Width/Height are the TOTAL rendered size — border and
	// padding included. (v1 excluded the border, which is why the old
	// Height(outerHeight-2) calls here left the panel two rows short of the
	// terminal and bled the backdrop through at the bottom.)
	// Content area = outerHeight - border(2) - vertical padding(2).
	// Content above the list: stats (1) + blank (1) + search (1) + blank (1).
	listHeight := outerHeight - 4 - 4
	if listHeight < 1 {
		listHeight = 1
	}

	if m.viewMode == ListView {
		b.WriteString(m.renderListView(innerWidth, listHeight))
	} else {
		b.WriteString(m.renderGridView(innerWidth, listHeight))
	}

	// The list is where keystrokes land, so it uses the accent-bordered
	// primary panel. Other panels (preview, details, empty states) stay
	// on the quieter Border color.
	return fitBox(primaryPanelStyle(), outerWidth, outerHeight).
		Render(b.String())
}

func (m Model) renderListView(width, height int) string {
	if len(m.filtered) == 0 {
		return mutedStyle().Render("  No sessions found")
	}

	// Rows carry group headers alongside sessions, so scrolling works in row
	// space while the cursor keeps indexing sessions.
	rows := buildRows(m.filtered)
	cursorRow := 0
	for i, r := range rows {
		if r.SessionIdx == m.cursor {
			cursorRow = i
			break
		}
	}

	start := 0
	if cursorRow >= height {
		start = cursorRow - height + 1
	}
	end := start + height
	if end > len(rows) {
		end = len(rows)
	}

	c := theme.Current().Colors

	var lines []string
	for ri := start; ri < end; ri++ {
		row := rows[ri]
		if row.SessionIdx < 0 {
			lines = append(lines, m.renderGroupHeader(row, width))
			continue
		}

		i := row.SessionIdx
		s := m.filtered[i]
		dot := statusDot(s.Status, m.pulseOn)
		badge := lipgloss.NewStyle().
			Foreground(lipgloss.Color(models.AgentColor(s.AgentType))).
			Background(lipgloss.Color(models.AgentBgColor(s.AgentType))).
			Padding(0, 1).
			Render(models.AgentLabel(s.AgentType))

		star := ""
		if m.isStarred(s.Name) {
			star = "★ "
		}
		starStyled := lipgloss.NewStyle().Foreground(c.Warning).Render(star)

		// Row layout: a left cluster (status dot + name) and a right cluster
		// (star + agent badge) pinned to the right edge. Right-aligning the
		// badges lines them up in a clean gutter instead of letting them
		// float at ragged offsets that vary with each name's length — and it
		// hands every spare column to the name. The star sits inside the
		// cluster ahead of the badge so a starred row does not shunt its
		// badge out of the shared column.
		right := starStyled + badge
		// A grouped child is indented under its header and shows only its own
		// name — the repo is on the header, so the prefix is not repeated.
		prefix := ""
		if row.Glyph != "" {
			prefix = row.Glyph + " "
		}
		// Columns the row spends on anything that is not the name: leading
		// space, the dot, the space after it, the tree prefix, the right
		// cluster, at least one space separating name from badge, and a
		// trailing gutter column.
		fixed := 1 + lipgloss.Width(dot) + 1 + lipgloss.Width(prefix) + 1 + lipgloss.Width(right) + rowGutter
		maxName := width - fixed
		if maxName < minNameWidth {
			maxName = minNameWidth
		}
		name := truncate(row.Label, maxName)

		// Selection: tint the row background (crush / lazygit / gh-dash
		// convention) — no caret or gutter rune. Bold the text for an
		// extra hierarchy cue. The tint color is per-theme (SelBg), so
		// every theme controls exactly how loud its selection reads.
		rowBg := c.Background
		nameStyle := lipgloss.NewStyle().Foreground(c.Foreground)
		if i == m.cursor {
			rowBg = c.SelBg
			nameStyle = nameStyle.Foreground(c.Foreground).Bold(true)
		}
		// Highlight matches against the label actually shown: a query that only
		// hit the repo portion has nothing to mark on the child, and the header
		// above carries that text.
		nameStyled := renderNameWithMatches(name, row.Label, m.searchInput.Value(), nameStyle, c.Accent)
		prefixStyled := mutedStyle().Render(prefix)

		gap := width - 1 - lipgloss.Width(dot) - 1 - lipgloss.Width(prefixStyled) - lipgloss.Width(nameStyled) - lipgloss.Width(right) - rowGutter
		if gap < 1 {
			gap = 1
		}
		content := fmt.Sprintf(" %s %s%s%s%s", dot, prefixStyled, nameStyled, strings.Repeat(" ", gap), right)
		line := lipgloss.NewStyle().
			Background(rowBg).
			Width(width).
			Render(content)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// renderGroupHeader renders the repo line above a group of sessions. It carries
// no status dot and no agent badge — those belong to the sessions underneath —
// just the repo name and how many sessions it holds.
func (m Model) renderGroupHeader(row listRow, width int) string {
	c := theme.Current().Colors

	count := mutedStyle().Render(fmt.Sprintf("%d sessions", row.Count))
	fixed := 1 + 1 + lipgloss.Width(count) + rowGutter
	maxName := width - fixed
	if maxName < minNameWidth {
		maxName = minNameWidth
	}
	name := lipgloss.NewStyle().
		Foreground(c.Secondary).
		Bold(true).
		Render(truncate(row.Group, maxName))

	gap := width - 1 - lipgloss.Width(name) - lipgloss.Width(count) - rowGutter
	if gap < 1 {
		gap = 1
	}
	content := fmt.Sprintf(" %s%s%s", name, strings.Repeat(" ", gap), count)
	return lipgloss.NewStyle().Background(c.Background).Width(width).Render(content)
}

func (m Model) renderGridView(width, height int) string {
	if len(m.filtered) == 0 {
		return mutedStyle().Render("  No sessions found")
	}

	cols := 2
	cellWidth := (width - 2) / cols
	if cellWidth < 15 {
		cols = 1
		cellWidth = width - 2
	}

	c := theme.Current().Colors

	var rows []string
	for i := 0; i < len(m.filtered); i += cols {
		var cells []string
		for j := 0; j < cols && i+j < len(m.filtered); j++ {
			idx := i + j
			s := m.filtered[idx]
			dot := statusDot(s.Status, m.pulseOn)
			// Leading space, dot, separating space, trailing gutter.
			maxLen := cellWidth - 3 - rowGutter
			if maxLen < minNameWidth {
				maxLen = minNameWidth
			}
			name := truncate(s.Name, maxLen)
			// Selection: tint the cell background; same palette slot as the
			// list view so the two modes read consistently.
			cellBg := c.Background
			nameStyle := lipgloss.NewStyle().Foreground(c.Foreground)
			if idx == m.cursor {
				cellBg = c.SelBg
				nameStyle = nameStyle.Foreground(c.Foreground).Bold(true)
			}
			nameStyled := renderNameWithMatches(name, s.Name, m.searchInput.Value(), nameStyle, c.Accent)
			content := fmt.Sprintf(" %s %s", dot, nameStyled)
			cell := lipgloss.NewStyle().
				Background(cellBg).
				Width(cellWidth).
				Render(content)
			cells = append(cells, cell)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	return strings.Join(rows, "\n")
}

func (m Model) viewRight(outerWidth, outerHeight int) string {
	if len(m.filtered) == 0 {
		return fitBox(panelStyle(), outerWidth, outerHeight).
			Render(mutedStyle().Render("No session selected"))
	}

	innerWidth := outerWidth - 4
	if innerWidth < 10 {
		innerWidth = 10
	}

	s := m.filtered[m.cursor]
	c := theme.Current().Colors

	// Detail section
	label := lipgloss.NewStyle().Foreground(c.Muted)
	val := lipgloss.NewStyle().Foreground(c.Foreground)
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(s.Status)))
	agentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(models.AgentColor(s.AgentType))).
		Background(lipgloss.Color(models.AgentBgColor(s.AgentType))).
		Padding(0, 1)

	// Build info column
	var info strings.Builder
	info.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Path  "), val.Render(s.Path)))
	info.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Agent "), agentStyle.Render(models.AgentLabel(s.AgentType))))
	info.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Status"), statusStyle.Render(models.StatusLabel(s.Status))))

	if s.Details.GitBranch != "" {
		info.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Branch"), val.Render(s.Details.GitBranch)))
	}
	// Name the repo a worktree belongs to — the display name only carries the
	// worktree, so without this a worktree pane never says what it forked from.
	if s.Details.RepoName != "" && s.Details.Worktree != "" {
		repo := fmt.Sprintf("%s (worktree %s)", s.Details.RepoName, s.Details.Worktree)
		info.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Repo  "), val.Render(repo)))
	}
	if s.Details.LastActivity != "" {
		info.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Active"), val.Render(formatTimeAgo(s.Details.LastActivity))))
	}
	if s.LastMessage != "" {
		msg := s.LastMessage
		maxLen := innerWidth - 30
		if maxLen > 0 && len(msg) > maxLen {
			msg = msg[:maxLen] + "..."
		}
		info.WriteString(fmt.Sprintf("  %s  %s\n", label.Render("Last  "), mutedStyle().Render(msg)))
	}

	// Combine art on the left with info on the right
	var detail strings.Builder
	art := renderAgentArt(s.AgentType)
	if art != "" && innerWidth > 40 {
		artWidth := lipgloss.Width(art)
		infoWidth := innerWidth - artWidth - 2 // 2 for gap
		infoBlock := lipgloss.NewStyle().
			Width(infoWidth).
			Background(c.Background).
			Render(titleStyle().Render(s.Name) + "\n\n" + info.String())
		gap := lipgloss.NewStyle().
			Width(2).
			Background(c.Background).
			Render("")
		detail.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, art, gap, infoBlock))
	} else {
		detail.WriteString(titleStyle().Render(s.Name))
		detail.WriteString("\n\n")
		detail.WriteString(info.String())
	}

	// Size detail panel to fit its content exactly. lipgloss v2 Height is the
	// total rendered height, so it must cover the content plus this style's
	// vertical padding (2) and border (2).
	detailContent := detail.String()
	detailLines := strings.Count(detailContent, "\n") + 1
	detailOuter := detailLines + 4
	if detailOuter > outerHeight/2 {
		// Cap at half the panel and clamp content to fit.
		detailOuter = outerHeight / 2
		maxContent := detailOuter - 4
		if maxContent < 1 {
			maxContent = 1
		}
		ls := strings.Split(detailContent, "\n")
		if len(ls) > maxContent {
			detailContent = strings.Join(ls[:maxContent], "\n")
		}
	}
	if detailOuter < 6 {
		detailOuter = 6
	}

	detailStr := fitBox(panelStyle(), outerWidth, detailOuter).
		Render(detailContent)

	// Preview section: gets the remaining height.
	// inner = previewOuter - border(2), no vertical padding on previewPanelStyle.
	previewOuter := outerHeight - detailOuter
	if previewOuter < 5 {
		previewOuter = 5
	}

	previewContent := m.preview
	if previewContent == "" {
		previewContent = mutedStyle().Render("No preview available")
	} else {
		maxLines := previewOuter - 2
		if maxLines < 1 {
			maxLines = 1
		}
		lines := strings.Split(previewContent, "\n")
		// Trim trailing empty lines
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		// Take the bottom portion
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
		}
		for i, line := range lines {
			if ansi.StringWidth(line) > innerWidth {
				lines[i] = ansi.Truncate(line, innerWidth, "")
			}
		}
		previewContent = strings.Join(lines, "\n")
	}

	previewStr := fitBox(previewPanelStyle(), outerWidth, previewOuter).
		Render(previewContent)

	return lipgloss.JoinVertical(lipgloss.Left, detailStr, previewStr)
}

// --- Grid view ---

func gridColumns(count int) int {
	if count <= 2 {
		return 1
	}
	if count <= 4 {
		return 2
	}
	return 3
}

func (m Model) viewGrid(totalWidth, totalHeight int) string {
	c := theme.Current().Colors

	if len(m.filtered) == 0 {
		return fitBox(panelStyle(), totalWidth, totalHeight).
			Render(mutedStyle().Render("No sessions found"))
	}

	// Search bar at top (1 line + 1 blank line = 2 lines)
	searchBar := m.searchInput.View()

	cols := gridColumns(len(m.filtered))
	cellWidth := totalWidth / cols
	numRows := (len(m.filtered) + cols - 1) / cols
	cellHeight := (totalHeight - 2 - numRows) / numRows // search + row gaps
	if cellHeight < 8 {
		cellHeight = 8
	}

	// Build rows of cells
	var rows []string
	for i := 0; i < len(m.filtered); i += cols {
		var cells []string
		for j := 0; j < cols && i+j < len(m.filtered); j++ {
			idx := i + j
			s := m.filtered[idx]

			// Header: status dot + name + agent badge
			dot := statusDot(s.Status, m.pulseOn)
			statusLabel := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(s.Status))).Render(models.StatusLabel(s.Status))
			agentBadge := lipgloss.NewStyle().
				Foreground(lipgloss.Color(models.AgentColor(s.AgentType))).
				Background(lipgloss.Color(models.AgentBgColor(s.AgentType))).
				Padding(0, 1).
				Render(models.AgentLabel(s.AgentType))

			header := fmt.Sprintf(" %s %s %s  %s", dot, s.Name, agentBadge, statusLabel)

			// Meta line: path + git branch
			meta := mutedStyle().Render(fmt.Sprintf("   %s", s.Path))
			if s.Details.GitBranch != "" {
				meta += mutedStyle().Render(fmt.Sprintf("  (%s)", s.Details.GitBranch))
			}

			// Separator between header and preview
			innerWidth := cellWidth - 6 // borders + padding
			if innerWidth < 10 {
				innerWidth = 10
			}

			// Small agent art floated to the right of the header
			art := renderAgentArtSmall(s.AgentType)
			artWidth := lipgloss.Width(art)
			topBlock := header + "\n" + meta
			if art != "" && innerWidth > 30 {
				textWidth := innerWidth - artWidth - 1
				textCol := lipgloss.NewStyle().Width(textWidth).Background(c.Background).Render(topBlock)
				gap := lipgloss.NewStyle().Width(1).Background(c.Background).Render("")
				topBlock = lipgloss.JoinHorizontal(lipgloss.Top, textCol, gap, art)
			}

			separator := lipgloss.NewStyle().Foreground(c.Border).Render(strings.Repeat("─", innerWidth))

			// Preview: capture pane content for this session
			previewHeight := cellHeight - 7
			if previewHeight < 1 {
				previewHeight = 1
			}

			preview := m.getGridPreview(s, innerWidth, previewHeight)

			content := topBlock + "\n" + separator + "\n" + preview

			// Border color: bright for selected, muted for others
			borderColor := c.Border
			if idx == m.cursor {
				borderColor = c.Primary
			}

			cellStyle := lipgloss.NewStyle().
				Background(c.Background).
				Foreground(c.Foreground).
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(borderColor).
				BorderBackground(c.Background).
				Padding(1)
			cell := fitBox(cellStyle, cellWidth, cellHeight).Render(content)

			cells = append(cells, cell)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}

	grid := strings.Join(rows, "\n")
	result := " " + searchBar + "\n" + grid
	// Pad to totalHeight so the help bar (appended by View) lands at the bottom.
	if pad := totalHeight - (strings.Count(result, "\n") + 1); pad > 0 {
		result += strings.Repeat("\n", pad)
	}
	return result
}

func (m Model) getGridPreview(s models.Session, width, height int) string {
	target := tmux.PaneTarget(s.SessionName, s.WindowIndex, s.PaneIndex)
	content := m.gridPreviews[target]
	if content == "" {
		return mutedStyle().Render("Loading...")
	}

	lines := strings.Split(content, "\n")
	// Trim trailing blank lines so the last real content lands at the bottom.
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	// Take the bottom portion so the last line of the stream is visible.
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	for i, line := range lines {
		if ansi.StringWidth(line) > width {
			lines[i] = ansi.Truncate(line, width, "")
		}
	}
	return strings.Join(lines, "\n")
}

// formatTimeAgo converts an ISO 8601 timestamp to a human-readable relative time.
func formatTimeAgo(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh %dm ago", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
