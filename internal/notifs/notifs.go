package notifs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nemke/nagare-go/internal/config"
	"github.com/nemke/nagare-go/internal/notifications"
	"github.com/nemke/nagare-go/internal/session"
	"github.com/nemke/nagare-go/internal/theme"
)

// Model is the Bubble Tea model for the notification center TUI.
type Model struct {
	store       *notifications.Store
	items       []notifications.Notification
	cursor      int
	tab         int // 0 = notifications, 1 = settings
	width       int
	height      int
	cfg         config.NagareConfig
	settingsCur int // cursor in settings tab
	editInput   textinput.Model
	editMode    bool
	editField   string
}

// New creates a new notification center model.
func New() Model {
	cfg, _ := config.Load()
	store := notifications.NewStore(notifications.DefaultStorePath())

	ti := textinput.New()
	ti.CharLimit = 10

	return Model{
		store:     store,
		items:     store.ListAll(),
		cfg:       cfg,
		editInput: ti,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
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
	key := msg.String()

	// Edit mode for int settings
	if m.editMode {
		switch key {
		case "esc":
			m.editMode = false
			m.editInput.SetValue("")
			return m, nil
		case "enter":
			m.saveIntSetting()
			m.editMode = false
			m.editInput.SetValue("")
			return m, nil
		default:
			var cmd tea.Cmd
			m.editInput, cmd = m.editInput.Update(msg)
			return m, cmd
		}
	}

	switch key {
	case "esc", "q":
		return m, tea.Quit
	case "1":
		m.tab = 0
		m.cursor = 0
		return m, nil
	case "2":
		m.tab = 1
		m.cursor = 0
		return m, nil
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down":
		maxItems := m.maxItemsForTab()
		if m.cursor < maxItems-1 {
			m.cursor++
		}
		return m, nil
	case "d":
		if m.tab == 0 && len(m.items) > 0 && m.cursor < len(m.items) {
			m.store.Dismiss(m.items[m.cursor].ID)
			m.items = m.store.ListAll()
			if m.cursor >= len(m.items) && m.cursor > 0 {
				m.cursor--
			}
		}
		return m, nil
	case "D":
		if m.tab == 0 {
			m.store.DismissAll()
			m.items = m.store.ListAll()
			m.cursor = 0
		}
		return m, nil
	case "enter":
		if m.tab == 0 {
			return m.handleNotificationEnter()
		}
		return m.handleSettingsEnter()
	}

	return m, nil
}

func (m Model) maxItemsForTab() int {
	if m.tab == 0 {
		return len(m.items)
	}
	return 14 // 12 settings + 2 separators
}

func (m Model) handleNotificationEnter() (tea.Model, tea.Cmd) {
	if len(m.items) == 0 || m.cursor >= len(m.items) {
		return m, nil
	}

	item := m.items[m.cursor]
	m.store.MarkRead(item.ID)
	m.items = m.store.ListAll()

	// Jump to session
	session.SwitchToSession(item.SessionName)
	return m, tea.Quit
}

func (m Model) handleSettingsEnter() (tea.Model, tea.Cmd) {
	item := m.getSettingsItem(m.cursor)
	if item == nil {
		return m, nil
	}

	if item.isBool {
		// Toggle bool setting
		*item.boolPtr = !*item.boolPtr
		m.saveConfig()
	} else {
		// Start editing int
		m.editMode = true
		m.editField = item.field
		m.editInput.SetValue(fmt.Sprintf("%d", *item.intPtr))
		m.editInput.Focus()
	}

	return m, nil
}

type settingsItem struct {
	label   string
	isBool  bool
	boolPtr *bool
	intPtr  *int
	field   string
}

func (m Model) getSettingsItem(idx int) *settingsItem {
	cfg := &m.cfg
	ni := &cfg.Notifications.NeedsInput
	tc := &cfg.Notifications.TaskComplete

	items := []settingsItem{
		{"Notifications enabled", true, &cfg.Notifications.Enabled, nil, "enabled"},
		{"", false, nil, nil, ""}, // separator
		{"Toast notification", true, &ni.Toast, nil, "needs_input.toast"},
		{"Bell", true, &ni.Bell, nil, "needs_input.bell"},
		{"OS notification", true, &ni.OsNotify, nil, "needs_input.os_notify"},
		{"Popup notification", true, &ni.Popup, nil, "needs_input.popup"},
		{"Popup timeout", false, nil, &ni.PopupTimeout, "needs_input.popup_timeout"},
		{"", false, nil, nil, ""}, // separator
		{"Toast notification", true, &tc.Toast, nil, "task_complete.toast"},
		{"Bell", true, &tc.Bell, nil, "task_complete.bell"},
		{"OS notification", true, &tc.OsNotify, nil, "task_complete.os_notify"},
		{"Popup notification", true, &tc.Popup, nil, "task_complete.popup"},
		{"Popup timeout", false, nil, &tc.PopupTimeout, "task_complete.popup_timeout"},
		{"Min working seconds", false, nil, &tc.MinWorkingSeconds, "task_complete.min_working_seconds"},
	}

	if idx < 0 || idx >= len(items) {
		return nil
	}
	return &items[idx]
}

func (m *Model) saveIntSetting() {
	val, err := strconv.Atoi(strings.TrimSpace(m.editInput.Value()))
	if err != nil || val < 0 {
		return
	}

	item := m.getSettingsItem(m.cursor)
	if item != nil && item.intPtr != nil {
		*item.intPtr = val
		m.saveConfig()
	}
}

func (m Model) saveConfig() {
	if err := config.Save(m.cfg); err != nil {
		// Log error silently
		_ = err
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	c := theme.Current().Colors

	var b strings.Builder

	// Tab bar
	b.WriteString(m.renderTabBar())
	b.WriteString("\n")

	// Content
	contentHeight := m.height - 3 // tab bar + hint bar + padding
	if m.tab == 0 {
		b.WriteString(m.renderNotifications(contentHeight))
	} else {
		b.WriteString(m.renderSettings(contentHeight))
	}

	// Hint bar at bottom
	b.WriteString("\n")
	b.WriteString(m.renderHintBar())

	return lipgloss.NewStyle().
		Background(c.Background).
		Foreground(c.Foreground).
		Width(m.width).
		Height(m.height).
		Render(b.String())
}

func (m Model) renderTabBar() string {
	c := theme.Current().Colors

	tab1Style := lipgloss.NewStyle().
		Foreground(c.Foreground).
		Background(c.Background)
	tab2Style := lipgloss.NewStyle().
		Foreground(c.Foreground).
		Background(c.Background)

	if m.tab == 0 {
		tab1Style = tab1Style.Foreground(c.Background).Background(c.Primary).Bold(true)
	} else {
		tab2Style = tab2Style.Foreground(c.Background).Background(c.Primary).Bold(true)
	}

	tab1 := tab1Style.Padding(0, 2).Render("[1] Notifications")
	tab2 := tab2Style.Padding(0, 2).Render("[2] Settings")

	return lipgloss.JoinHorizontal(lipgloss.Top, tab1, "  ", tab2)
}

func (m Model) renderNotifications(height int) string {
	if len(m.items) == 0 {
		return lipgloss.NewStyle().
			Foreground(theme.Current().Colors.Muted).
			Render("  No notifications")
	}

	listHeight := height - 2
	if listHeight < 1 {
		listHeight = 1
	}

	start := 0
	if m.cursor >= listHeight {
		start = m.cursor - listHeight + 1
	}
	end := start + listHeight
	if end > len(m.items) {
		end = len(m.items)
	}

	var lines []string
	for i := start; i < end; i++ {
		item := m.items[i]
		line := m.renderNotificationItem(item, i == m.cursor)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderNotificationItem(item notifications.Notification, selected bool) string {
	c := theme.Current().Colors

	// Read dot
	readDot := " "
	if !item.Read {
		readDot = lipgloss.NewStyle().Foreground(c.Primary).Render("●")
	}

	// Icon
	icon := "⏳"
	if strings.Contains(item.Message, "finished") || strings.Contains(item.Message, "✅") {
		icon = "✅"
	}

	// Timestamp (first 19 chars: YYYY-MM-DD HH:MM:SS)
	ts := item.Timestamp
	if len(ts) > 19 {
		ts = ts[:19]
	}

	// Format lines
	line1 := fmt.Sprintf(" %s %s %s  %s", readDot, icon, item.SessionName, item.Message)
	line2 := fmt.Sprintf("    %s", lipgloss.NewStyle().Foreground(c.Muted).Render(ts))

	if selected {
		// Highlight both lines
		line1 = lipgloss.NewStyle().
			Background(c.Primary).
			Foreground(c.Background).
			Bold(true).
			Render(line1)
		line2 = lipgloss.NewStyle().
			Background(c.Primary).
			Foreground(c.Background).
			Render(line2)
	}

	return line1 + "\n" + line2
}

func (m Model) renderSettings(height int) string {
	c := theme.Current().Colors
	sectionHeader := lipgloss.NewStyle().Foreground(c.Accent).Bold(true)

	var lines []string

	lines = append(lines, sectionHeader.Render("  Master"))

	for i := 0; i < 14; i++ {
		item := m.getSettingsItem(i)
		if item == nil {
			continue
		}

		// Separators + section headers
		if item.label == "" {
			lines = append(lines, "")
			if i == 1 {
				lines = append(lines, sectionHeader.Render("  Needs Input"))
			}
			if i == 7 {
				lines = append(lines, sectionHeader.Render("  Task Complete"))
			}
			continue
		}

		line := m.renderSettingsItem(item, i == m.cursor)
		lines = append(lines, line)
	}

	// Add edit input if in edit mode
	if m.editMode {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().
			Foreground(c.Primary).
			Render("  Edit: "+m.editInput.View()))
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderSettingsItem(item *settingsItem, selected bool) string {
	c := theme.Current().Colors

	var line string
	if item.isBool {
		check := "[ ]"
		if *item.boolPtr {
			check = "[x]"
		}
		line = fmt.Sprintf("  %s %s", check, item.label)
	} else {
		line = fmt.Sprintf("  %s: %d", item.label, *item.intPtr)
	}

	if selected {
		line = lipgloss.NewStyle().
			Background(c.Primary).
			Foreground(c.Background).
			Bold(true).
			Render("  " + line)
	}

	return line
}

func (m Model) renderHintBar() string {
	c := theme.Current().Colors
	key := lipgloss.NewStyle().Foreground(c.Accent).Bold(true)
	sep := lipgloss.NewStyle().Foreground(c.Muted).Render(" │ ")

	var parts []string
	if m.tab == 0 {
		parts = append(parts, key.Render("Enter")+" Jump")
		parts = append(parts, key.Render("d")+" Dismiss")
		parts = append(parts, key.Render("D")+" Dismiss All")
	} else {
		parts = append(parts, key.Render("Enter")+" Toggle/Edit")
	}
	parts = append(parts, key.Render("1/2")+" Tab")
	parts = append(parts, key.Render("Esc")+" Quit")

	bar := strings.Join(parts, sep)
	return lipgloss.NewStyle().
		Foreground(c.Muted).
		Background(c.Background).
		Width(m.width).
		Padding(0, 1).
		Render(bar)
}
