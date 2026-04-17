package picker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	result        Result
	promptMode    bool
	promptTarget  models.Session
	promptInput   textinput.Model
	lastQuery     string   // previous search query, to detect query changes in applyFilter
	gridOrder     []string // frozen display order for grid view (session keys); nil = not yet snapshotted
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
	pi.Width = 60

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

// isStarred returns whether a session is starred in the registry.
func (m Model) isStarred(name string) bool {
	s := m.registry.Find(name)
	return s != nil && s.Starred
}

// Result returns the picker's result (action to take after quitting).
func (m Model) Result() Result {
	return m.result
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		doScan(m.statesDir),
		doPreviewTick(),
	)
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

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	// Render base content
	contentHeight := m.height
	if m.showHelpBar {
		contentHeight = m.height - 2 // help bar can wrap to 2 lines
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
		base = base + "\n" + helpBar(m.width)
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
	log.Debug("key: %q type=%d", key, msg.Type)

	// Theme picker intercepts all keys when open
	if m.showThemePick {
		return m.handleThemePickKey(key)
	}

	// Rename mode intercepts keys before normal handling
	if m.renameMode {
		return m.handleRenameKey(msg)
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
		hookStates := state.LoadAllStates(statesDir)
		sessions := tmux.ScanSessions(hookStates)
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

func (m *Model) sortFiltered() {
	// Pre-build starred set to avoid per-comparison registry lookups
	starred := make(map[string]bool)
	for _, s := range m.filtered {
		starred[s.Name] = m.isStarred(s.Name)
	}

	sort.SliceStable(m.filtered, func(i, j int) bool {
		si := starred[m.filtered[i].Name]
		sj := starred[m.filtered[j].Name]
		if si != sj {
			return si
		}
		switch m.sortMode {
		case SortByName:
			return m.filtered[i].Name < m.filtered[j].Name
		case SortByAgent:
			return m.filtered[i].AgentType < m.filtered[j].AgentType
		default: // SortByStatus
			return statusOrder(m.filtered[i].Status) < statusOrder(m.filtered[j].Status)
		}
	})
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

func (m Model) viewLeft(outerWidth, outerHeight int) string {
	innerWidth := outerWidth - 4
	if innerWidth < 10 {
		innerWidth = 10
	}

	var b strings.Builder

	// Dashboard stats
	total := len(m.sessions)
	waiting := 0
	running := 0
	for _, s := range m.sessions {
		switch s.Status {
		case models.StatusWaitingInput:
			waiting++
		case models.StatusRunning:
			running++
		}
	}
	b.WriteString(mutedStyle().Render(fmt.Sprintf(" %d sessions | %d waiting | %d running", total, waiting, running)))
	b.WriteString("\n\n")

	// Search input (always active)
	if m.renameMode {
		m.searchInput.Prompt = " Rename: "
	} else {
		m.searchInput.Prompt = " > "
	}
	b.WriteString(m.searchInput.View())
	b.WriteString("\n\n")

	// Session list
	// lipgloss Height(H) = post-padding, pre-border height; outer = H+2.
	// Pass Height(outerHeight-2) so outer panel = outerHeight.
	// Inner content = (outerHeight-2) - vertical_padding(2) = outerHeight-4.
	// Content above list: stats (1) + blank (1) + search (1) + blank (1) = 4
	listHeight := outerHeight - 8
	if listHeight < 1 {
		listHeight = 1
	}

	if m.viewMode == ListView {
		b.WriteString(m.renderListView(innerWidth, listHeight))
	} else {
		b.WriteString(m.renderGridView(innerWidth, listHeight))
	}

	return panelStyle().
		Width(outerWidth - 2).
		Height(outerHeight - 2).
		Render(b.String())
}

func (m Model) renderListView(width, height int) string {
	if len(m.filtered) == 0 {
		return mutedStyle().Render("  No sessions found")
	}

	start := 0
	if m.cursor >= height {
		start = m.cursor - height + 1
	}
	end := start + height
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	c := theme.Current().Colors

	var lines []string
	for i := start; i < end; i++ {
		s := m.filtered[i]
		dot := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(s.Status))).Render("●")
		badge := lipgloss.NewStyle().
			Foreground(lipgloss.Color(models.AgentColor(s.AgentType))).
			Background(lipgloss.Color(models.AgentBgColor(s.AgentType))).
			Padding(0, 1).
			Render(models.AgentLabel(s.AgentType))

		name := s.Name
		maxName := width - 20
		if maxName < 5 {
			maxName = 5
		}
		if utf8.RuneCountInString(name) > maxName {
			runes := []rune(name)
			name = string(runes[:maxName]) + "..."
		}

		star := ""
		if m.isStarred(s.Name) {
			star = " ★"
		}

		var line string
		if i == m.cursor {
			cursor := lipgloss.NewStyle().Foreground(c.Primary).Bold(true).Render(">")
			nameStyled := lipgloss.NewStyle().Foreground(c.Primary).Bold(true).Render(name)
			starStyled := lipgloss.NewStyle().Foreground(c.Warning).Render(star)
			content := fmt.Sprintf(" %s %s %s %s%s", cursor, dot, nameStyled, badge, starStyled)
			line = lipgloss.NewStyle().
				Background(c.Background).
				PaddingLeft(1).
				Width(width).
				Render(content)
		} else {
			nameStyled := lipgloss.NewStyle().Foreground(c.Foreground).Render(name)
			starStyled := lipgloss.NewStyle().Foreground(c.Warning).Render(star)
			content := fmt.Sprintf(" %s %s %s%s", dot, nameStyled, badge, starStyled)
			line = lipgloss.NewStyle().
				Background(c.Background).
				Foreground(c.Foreground).
				PaddingLeft(2).
				Width(width).
				Render(content)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
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
			dot := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(s.Status))).Render("●")
			name := s.Name
			maxLen := cellWidth - 6
			if utf8.RuneCountInString(name) > maxLen {
				runes := []rune(name)
				name = string(runes[:maxLen]) + ".."
			}
			content := fmt.Sprintf(" %s %s", dot, name)
			var cell string
			if idx == m.cursor {
				cell = lipgloss.NewStyle().
					Background(c.Primary).Foreground(c.Background).Bold(true).
					Width(cellWidth).Render(content)
			} else {
				cell = lipgloss.NewStyle().
					Background(c.Background).Foreground(c.Foreground).
					Width(cellWidth).Render(content)
			}
			cells = append(cells, cell)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	return strings.Join(rows, "\n")
}

func (m Model) viewRight(outerWidth, outerHeight int) string {
	if len(m.filtered) == 0 {
		return panelStyle().
			Width(outerWidth - 2).
			Height(outerHeight - 2).
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

	// Size detail panel to fit its content exactly.
	// lipgloss outer = Height+2 (border), inner = Height - padding_v(2) = Height-2.
	// So outer = content_lines + 4 (padding 2 + border 2).
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

	detailStr := panelStyle().
		Width(outerWidth - 2).
		Height(detailOuter - 2).
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

	previewStr := previewPanelStyle().
		Width(outerWidth - 2).
		Height(previewOuter - 2).
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
		return panelStyle().
			Width(totalWidth - 2).
			Height(totalHeight - 2).
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
			dot := lipgloss.NewStyle().Foreground(lipgloss.Color(models.StatusColor(s.Status))).Render("●")
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

			cell := lipgloss.NewStyle().
				Background(c.Background).
				Foreground(c.Foreground).
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(borderColor).
				BorderBackground(c.Background).
				Width(cellWidth - 2).
				Height(cellHeight - 2).
				Padding(1).
				Render(content)

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
