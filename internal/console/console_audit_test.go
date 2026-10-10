package console

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestPS1AuditRejectedAnswerDoesNotHideEditorBehindReader(t *testing.T) {
	m, c := ccFixture(t, 1)
	m, _ = ccKey(m, runeKey('a'))
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("被拒绝后保留的草稿")})
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyF2})
	if cmd == nil {
		t.Fatal("answer was not submitted")
	}
	var action *actionState
	for _, a := range m.projects[0].actions {
		action = a
	}
	m, _ = ccKey(m, runeKey('f'))
	receipt := &api.CommandReceipt{CommandID: action.RequestID, Kind: "answer_decision", TargetID: action.DecisionID, ProjectIdentity: m.projects[0].ref.Identity, Status: "rejected", ResultCode: "answer_conflict"}
	next, _ := m.Update(actionReceiptMsg{projectKey: m.projects[0].ref.Key, identity: m.projects[0].ref.Identity, requestID: action.RequestID, command: receipt})
	m = next.(Model)
	if m.answer != nil || !m.detailExpanded {
		t.Fatal("asynchronous rejection opened an invisible editor or displaced reader")
	}
	_, retry := ccKey(m, tea.KeyMsg{Type: tea.KeyF2})
	if retry != nil || c.decisionCalls != 0 {
		t.Fatal("reader F2 resubmitted an invisible answer")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = ccKey(m, runeKey('a'))
	if m.answer == nil || string(m.answer.note) != "被拒绝后保留的草稿" {
		t.Fatal("rejected draft was not available when returning to the main view")
	}
}

func TestPS1AuditNewQuestionResetsActivityScroll(t *testing.T) {
	m, _ := ccFixture(t, 1)
	run := m.selectedRow(0).run
	decision := run.Decision
	run.Decision = nil
	m, _ = m.refreshRecentActivity(0)
	m.projects[0].lowerScroll = 4
	run.Decision = decision
	m, _ = m.refreshRecentActivity(0)
	if m.projects[0].lowerScroll != 0 {
		t.Fatal("new question inherited activity scroll and hid its heading")
	}
}

func TestPS1AuditNarrowListFocusCannotSubmitInvisibleAnswer(t *testing.T) {
	m, c := ccFixture(t, 1)
	m.width, m.height = 55, 22
	m, _ = ccKey(m, runeKey('a'))
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.projects[0].focus != 0 || strings.Contains(m.View(), "提交预览") {
		t.Fatal("expected narrow list view")
	}
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyF2})
	if cmd != nil || c.decisionCalls != 0 || m.answer == nil || !strings.Contains(m.View(), "提交预览") {
		t.Fatal("F2 in list view must expose the original draft before allowing a submission")
	}
	runID, decisionID := m.answer.runID, m.answer.decisionID
	m, cmd = ccKey(m, tea.KeyMsg{Type: tea.KeyF2})
	if cmd == nil || m.answer != nil {
		t.Fatal("visible preview could not submit")
	}
	cmd()
	if c.decisionCalls != 1 || c.decisionAnswer.RunID != runID || c.decisionAnswer.DecisionID != decisionID {
		t.Fatal("preview changed the original answer target or submitted more than once")
	}
}

func TestPS1AuditBlockedNarrowEditorKeepsQuestionVisible(t *testing.T) {
	m, _ := ccFixture(t, 1)
	m.width, m.height = 55, 22
	run := m.selectedRow(0).run
	run.EnvironmentBlocker = &api.EnvironmentBlocker{}
	run.EnvironmentBlocker.Fact.Reason = "权限待核验"
	m, _ = ccKey(m, runeKey('a'))
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF2})
	for _, want := range []string{"问题：", "未提交", "提交预览", "F2"} {
		if !strings.Contains(m.View(), want) {
			t.Fatal("blocked editor lost", want, m.View())
		}
	}
}

func TestPS1AuditOverviewKeepsBlockerAndPRVisible(t *testing.T) {
	m, _ := ccFixture(t, 2)
	run := m.selectedRow(0).run
	url := "https://example.test/pr/1"
	run.PR.URL = &url
	run.EnvironmentBlocker = &api.EnvironmentBlocker{}
	run.EnvironmentBlocker.Fact.Reason = "权限待核验"
	for _, size := range [][2]int{{160, 24}, {120, 22}} {
		m.width, m.height = size[0], size[1]
		view := m.View()
		for _, want := range []string{"执行方案", "权限待核验", "PR："} {
			if !strings.Contains(view, want) {
				t.Fatalf("%dx%d overview lost %s: %s", size[0], size[1], want, view)
			}
		}
	}
}
