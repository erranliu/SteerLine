package console

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCPWFinalExitedInspectionCannotOverwriteStatus(t *testing.T) {
	m, _ := ccFixture(t, 1)
	m, _ = ccKey(m, runeKey('u'))
	s := m.projects[0]
	r := m.selectedRow(0).run
	late := recoveryInspectionMsg{projectKey: s.ref.Key, identity: s.ref.Identity, runID: r.RunID, generation: s.recoveryGeneration, err: errors.New("late inspection failure")}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	m.status = "当前主界面提示"
	next, _ := m.Update(late)
	m = next.(Model)
	if m.status != "当前主界面提示" || m.projects[0].recoveryBusy {
		t.Fatal("exit did not cancel original inspection")
	}
}

func TestCPWFinalSingleCandidateCyclePreservesEditor(t *testing.T) {
	m, _ := ccFixture(t, 1)
	m, _ = ccKey(m, runeKey('a'))
	editor := m.answer
	focus := m.projects[0].focus
	for _, key := range []tea.KeyMsg{{Type: tea.KeyF6}, {Type: tea.KeyF7}} {
		m, _ = ccKey(m, key)
		if m.answer != editor || m.projects[0].focus != focus {
			t.Fatal("no-op candidate change closed editor")
		}
	}
}

func TestCPWFinalHelpCannotDiscardUnpersistedEditor(t *testing.T) {
	m, _ := ccFixture(t, 1)
	m, _ = ccKey(m, runeKey('a'))
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	dir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(dir, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	m.profile.dir = dir
	m, cmd := ccKey(m, runeKey('?'))
	if cmd != nil || m.answer == nil || m.help || m.answer.error == "" {
		t.Fatal("help dismissed editor after save failure")
	}
}

func TestCPWFinalMultiRuneMenuTextKeepsEditor(t *testing.T) {
	m, _ := ccFixture(t, 1)
	m, _ = ccKey(m, runeKey('a'))
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	editor := m.answer
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s b")})
	if cmd != nil || m.answer != editor || m.profileChoice != nil {
		t.Fatal("partial shortcut match changed mode")
	}
}
