package picker

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nemke/nagare-go/internal/models"
	"github.com/nemke/nagare-go/internal/state"
)

func noteSession(name string) models.Session {
	return models.Session{
		Name: name, SessionName: name, Path: "/tmp/" + name,
		Status: models.StatusIdle, AgentType: models.AgentClaude,
	}
}

func noteModel(t *testing.T, sessions []models.Session) Model {
	t.Helper()
	m := NewForTest()
	m.notes = state.NewNotes(filepath.Join(t.TempDir(), "notes.json"))
	return driveModel(t, m,
		tea.WindowSizeMsg{Width: 120, Height: 40},
		SessionsUpdatedMsg(sessions),
	)
}

func TestF5OpensNoteOverlay(t *testing.T) {
	m := noteModel(t, []models.Session{noteSession("alpha")})

	m = driveModel(t, m, tea.KeyPressMsg{Code: tea.KeyF5})

	if !m.noteMode {
		t.Fatal("F5 did not open the note overlay")
	}
	if m.noteTarget.Name != "alpha" {
		t.Errorf("note target = %q, want alpha", m.noteTarget.Name)
	}
}

func TestF5DoesNothingWithEmptyList(t *testing.T) {
	m := noteModel(t, nil)

	m = driveModel(t, m, tea.KeyPressMsg{Code: tea.KeyF5})

	if m.noteMode {
		t.Error("F5 opened a note overlay with nothing selected")
	}
}

func TestNoteEnterSavesAndEscCancels(t *testing.T) {
	m := noteModel(t, []models.Session{noteSession("alpha")})

	m = driveModel(t, m, tea.KeyPressMsg{Code: tea.KeyF5})
	m = typeString(t, m, "waiting on API keys")
	m = driveModel(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.noteMode {
		t.Fatal("Enter did not close the note overlay")
	}
	if got := m.noteFor("alpha"); got != "waiting on API keys" {
		t.Errorf("saved note = %q, want %q", got, "waiting on API keys")
	}

	m = driveModel(t, m, tea.KeyPressMsg{Code: tea.KeyF5})
	m.noteInput.SetValue("should not stick")
	m = driveModel(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})

	if m.noteMode {
		t.Fatal("Esc did not close the note overlay")
	}
	if got := m.noteFor("alpha"); got != "waiting on API keys" {
		t.Errorf("note after cancel = %q, want the previously saved text", got)
	}
}

func TestEmptyNoteClears(t *testing.T) {
	m := noteModel(t, []models.Session{noteSession("alpha")})
	if err := m.notes.Set("alpha", "scratch"); err != nil {
		t.Fatal(err)
	}

	m = driveModel(t, m, tea.KeyPressMsg{Code: tea.KeyF5})
	m.noteInput.SetValue("")
	m = driveModel(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if got := m.noteFor("alpha"); got != "" {
		t.Errorf("note after empty save = %q, want empty", got)
	}
}

func TestNoteAppearsInDetails(t *testing.T) {
	m := noteModel(t, []models.Session{noteSession("alpha"), noteSession("beta")})
	if err := m.notes.Set("alpha", "waiting on API keys"); err != nil {
		t.Fatal(err)
	}

	for i, s := range m.filtered {
		if s.Name == "alpha" {
			m.cursor = i
			break
		}
	}

	text := ansi.Strip(m.View().Content)
	if !strings.Contains(text, "waiting on API keys") {
		t.Errorf("details missing note\n%s", text)
	}
	if !strings.Contains(text, "Note") {
		t.Errorf("details missing Note label\n%s", text)
	}

	for i, s := range m.filtered {
		if s.Name == "beta" {
			m.cursor = i
			break
		}
	}
	other := ansi.Strip(m.View().Content)
	if strings.Contains(other, "waiting on API keys") {
		t.Error("alpha's note leaked into beta's details")
	}
}

func TestNotePrefillsExistingText(t *testing.T) {
	m := noteModel(t, []models.Session{noteSession("alpha")})
	if err := m.notes.Set("alpha", "already here"); err != nil {
		t.Fatal(err)
	}

	m = driveModel(t, m, tea.KeyPressMsg{Code: tea.KeyF5})

	if got := m.noteInput.Value(); got != "already here" {
		t.Errorf("prefill = %q, want %q", got, "already here")
	}
}

func TestNoteOverlayIsNotClickDismissable(t *testing.T) {
	m := noteModel(t, []models.Session{noteSession("alpha")})
	m.noteMode = true
	m.noteTarget = m.filtered[0]

	_, hits := m.view()
	if hits.dialog.Empty() {
		t.Fatal("note overlay was not drawn")
	}
	if hits.dismissable {
		t.Error("note overlay is click-dismissable; a half-typed note should need Esc or Enter")
	}
}
