package console

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/muesli/termenv"
)

func TestCPWInlineChoicesFreezeTargetAndCancelOnSwitch(t *testing.T) {
	for _, change := range []string{"task", "graph", "profile", "missing"} {
		t.Run(change, func(t *testing.T) {
			m, c := ccFixture(t, 2)
			m, _ = ccKey(m, runeKey('b'))
			if m.profileChoice == nil || m.detailExpanded || !strings.Contains(m.View(), "任务列表") || !strings.Contains(m.View(), "Enter 确认") {
				t.Fatal("choice left main workspace", m.View())
			}
			switch change {
			case "task":
				m.selectedRow(0).native.Version++
			case "graph":
				m.projects[0].snapshot.NativeTasks.GraphRevision++
			case "profile":
				m.projects[0].ref.Presentation.Profiles[0].Digest = "changed"
			case "missing":
				m.projects[0].snapshot.NativeTasks = nil
			}
			m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil || c.calls != 0 || m.profileChoice != nil || !strings.Contains(m.status, "已变化") {
				t.Fatal("frozen choice rebound", m.status)
			}
		})
	}
	m, c := ccFixture(t, 2)
	m, _ = ccKey(m, runeKey('z'))
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.confirm == nil || m.projects[0].focus != 1 {
		t.Fatal("list Enter confirmed hidden operation")
	}
	m, cmd = ccKey(m, runeKey('y'))
	if cmd != nil || m.confirm == nil {
		t.Fatal("y confirmed operation")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF7})
	if m.confirm != nil || c.calls != 0 || m.selectedRow(0).run.RunID != "candidate-1" {
		t.Fatal("switch retained old confirmation")
	}
}

func TestCPWReadersDoNotDispatchHiddenActions(t *testing.T) {
	for _, entry := range []rune{'d', 'f', 'v'} {
		m, c := ccFixture(t, 2)
		m, _ = ccKey(m, runeKey(entry))
		for _, key := range []tea.KeyMsg{runeKey('s'), runeKey('b'), runeKey('z'), runeKey('a'), runeKey('u'), runeKey('x'), runeKey('m'), {Type: tea.KeyCtrlE}, {Type: tea.KeyCtrlO}, {Type: tea.KeyF2}, {Type: tea.KeyF7}, {Type: tea.KeyTab}} {
			m, cmd := ccKey(m, key)
			if cmd != nil || !m.detailExpanded || m.answer != nil || m.confirm != nil || m.profileChoice != nil || c.calls != 0 || c.decisionCalls != 0 || m.selectedRow(0).run.RunID != "candidate-0" {
				t.Fatal("reader dispatched", entry, key.String())
			}
		}
	}
	m, _ := ccFixture(t, 2)
	m, _ = ccKey(m, runeKey('b'))
	choice := m.profileChoice
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF8})
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyF2}, {Type: tea.KeyF7}, runeKey('y')} {
		m, cmd := ccKey(m, key)
		if cmd != nil || !m.operationDetails || m.profileChoice != choice {
			t.Fatal("F8 confirmed or changed target")
		}
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.operationDetails || m.profileChoice != choice || m.projects[0].focus != 1 {
		t.Fatal("F8 lost original mode")
	}
}

func TestCPWNoInvisibleSubmitIncludingStop(t *testing.T) {
	for _, action := range []rune{'a', 'b', 'z', 'x'} {
		m, c := ccFixture(t, 1)
		m.width, m.height = 120, 8
		m, _ = ccKey(m, runeKey(action))
		key := tea.KeyMsg{Type: tea.KeyEnter}
		if action == 'a' {
			key.Type = tea.KeyF2
		}
		m, cmd := ccKey(m, key)
		if cmd != nil || c.calls != 0 || c.decisionCalls != 0 || !strings.Contains(m.status, "窗口过小") {
			t.Fatal("invisible operation sent", action, m.status)
		}
	}
}

func TestCPWDraftSaveFailurePreventsModeSwitch(t *testing.T) {
	m, _ := ccFixture(t, 2)
	m, _ = ccKey(m, runeKey('a'))
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("保留中文草稿")})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
	dir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(dir, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	m.profile.dir = dir
	m, cmd := ccKey(m, runeKey('b'))
	if cmd != nil || m.answer == nil || string(m.answer.note) != "保留中文草稿" || m.profileChoice != nil || m.answer.error == "" {
		t.Fatal("lost draft on failed mode switch")
	}
}

func TestCPWCurrentActivityWinsOverOldFinalAndReceipts(t *testing.T) {
	m, _ := ccFixture(t, 1)
	r := m.selectedRow(0).run
	r.Decision = nil
	r.WorkflowStatus = "running"
	r.ExecutionStatus = "running"
	r.WorkflowStage, r.WorkflowStatus = "implement.execute", "running"
	r.CurrentTurnID = "new-turn"
	e := finalFixture(m, 1, "old-turn", "old", "上一轮结果")
	s := &m.projects[0]
	s.final.previewEvent = &e
	s.final.previewText = "上一轮结果"
	s.recentActivity = []activitySummary{{Title: "本轮正在运行"}}
	s.actions["old"] = &actionState{RequestID: "old", TaskID: r.TaskID, RunID: r.RunID, Action: "answer_decision", Resolved: true, Status: "applied", Message: "旧答案已保存"}
	view := strings.Join(feedbackLines(m, *s, m.selectedRow(0)), "\n")
	if !strings.Contains(view, "当前流程：implement.execute") || !strings.Contains(view, "当前状态：running") || strings.Contains(view, "本轮正在运行") || strings.Contains(view, "上一轮结果") {
		t.Fatal(view)
	}
	r.TaskStatus = "closed"
	r.EndedAt = &s.lastRefresh
	r.ExecutionStatus = "failed"
	r.WorkflowStatus = "blocked"
	if taskAttention(*s, r.TaskID) != "" {
		t.Fatal("historical failure became actionable")
	}
	s.candidates[r.TaskID] = "missing-run"
	if !strings.Contains(m.View(), "原候选暂不可读取") || m.selectedRow(0).run != nil {
		t.Fatal("missing pin replaced")
	}
}

func TestCPWFinalPreviewBoundedAndHistoricalReaderIndependent(t *testing.T) {
	m, _ := ccFixture(t, 2)
	r := m.selectedRow(0).run
	r.Decision = nil
	r.CurrentTurnID = "turn"
	r.WorkflowStage, r.WorkflowStatus = "await_merge", "waiting_external"
	m, _ = m.refreshFinalReplies(0)
	text := strings.Repeat("中文正文\n", 20000)
	query := m.projects[0].client.(*testClient).query
	query.bodies = map[string]string{"final": text}
	e := finalFixture(m, 1, "turn", "final", text)
	next, cmd := m.Update(finalResult(m, e))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("preview not requested")
	}
	late := cmd()
	next, _ = m.Update(late)
	m = next.(Model)
	if len([]rune(m.projects[0].final.previewText)) > 800 || len(strings.Split(m.projects[0].final.previewText, "\n")) > 6 || m.projects[0].final.bodyRef != nil || !strings.Contains(m.View(), "最终回复预览") {
		t.Fatal("unbounded preview or shared full body", m.View())
	}
	r.Summary = new(string)
	*r.Summary = "验证通过"
	if strings.Contains(m.View(), "验证通过") {
		t.Fatal("unqualified summary promoted")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF7})
	next, _ = m.Update(late)
	m = next.(Model)
	if m.projects[0].final.previewText != "" {
		t.Fatal("late preview crossed candidate")
	}
}

func TestCPWFocusFramesFitAndColorHasMonochromeEquivalent(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	profile := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(profile)
	for _, colors := range []termenv.Profile{termenv.Ascii, termenv.ANSI256} {
		lipgloss.SetColorProfile(colors)
		for _, size := range [][2]int{{120, 32}, {100, 28}, {80, 24}, {120, 18}} {
			m, _ := ccFixture(t, 2)
			m.width, m.height = size[0], size[1]
			m, _ = ccKey(m, runeKey('a'))
			m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
			view := m.View()
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size[0] {
					t.Fatal("width overflow", size, lipgloss.Width(line), line)
				}
			}
			if len(strings.Split(view, "\n")) > size[1] || strings.Contains(view, "输入焦点") || !strings.Contains(view, "╔") || !strings.Contains(view, "▶ ┃") || !strings.Contains(view, "提交预览") {
				t.Fatal("focus/preview lost", size, view)
			}
		}
	}
	if strings.Contains(stateText("未知\x1b[2J"), "\x1b[2J") {
		t.Fatal("unsafe state escaped cleaning")
	}
}

func TestCPWDumbTerminalKeepsFocusAndSelectionReadable(t *testing.T) {
	t.Setenv("TERM", "dumb")
	m, _ := ccFixture(t, 1)
	view := m.View()
	if !strings.Contains(view, "+ > 任务列表") || !strings.Contains(view, "====") || !strings.Contains(view, "▸") {
		t.Fatal("ASCII focus or selection lost", view)
	}
}

func TestCPWRecoverySummaryIsBoundAndExpires(t *testing.T) {
	m, _ := ccFixture(t, 2)
	m, cmd := ccKey(m, runeKey('u'))
	if cmd == nil || m.detailExpanded || !m.projects[0].recoveryActive {
		t.Fatal("inspection left workspace")
	}
	s := &m.projects[0]
	r := m.selectedRow(0).run
	report := api.RecoveryInspection{RunID: r.RunID, TaskID: r.TaskID, RunRevision: r.ControlRevision, ServiceInstanceID: s.snapshot.Connection.Snapshot.Page.Service.InstanceID, ProcessCheck: "原进程未核实"}
	msg := recoveryInspectionMsg{projectKey: s.ref.Key, identity: s.ref.Identity, runID: r.RunID, generation: s.recoveryGeneration, report: report}
	next, _ := m.Update(msg)
	m = next.(Model)
	if !strings.Contains(m.View(), "原进程未核实") || recoveryStale(m.projects[0], m.selectedRow(0)) {
		t.Fatal("report missing")
	}
	r.ControlRevision++
	if !strings.Contains(m.View(), "报告已过期") {
		t.Fatal("old report proved current state")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF7})
	next, _ = m.Update(msg)
	m = next.(Model)
	if m.projects[0].recoveryReport != nil || m.projects[0].recoveryActive {
		t.Fatal("late inspection crossed candidate")
	}
}

func TestCPWRejectedAnswerNeverReopensEditor(t *testing.T) {
	m, _ := ccFixture(t, 1)
	m, _ = ccKey(m, runeKey('a'))
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyTab})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("保留的原答案")})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF2})
	var action *actionState
	for _, a := range m.projects[0].actions {
		action = a
	}
	s := &m.projects[0]
	receipt := api.CommandReceipt{CommandID: action.RequestID, Kind: action.Action, TargetID: action.DecisionID, ProjectIdentity: s.ref.Identity, Status: "rejected", ResultCode: "answer_conflict"}
	next, _ := m.Update(actionReceiptMsg{projectKey: s.ref.Key, identity: s.ref.Identity, requestID: action.RequestID, command: &receipt})
	m = next.(Model)
	if m.answer != nil || m.projects[0].focus != 1 || !strings.Contains(m.View(), "answer_conflict") {
		t.Fatal("rejection stole focus", m.View())
	}
	m, _ = ccKey(m, runeKey('a'))
	if m.answer == nil || string(m.answer.note) != "保留的原答案" {
		t.Fatal("explicit reopen lost draft")
	}
}

func TestCPWLongConfirmDoesNotHideItsConfirmation(t *testing.T) {
	m, _ := ccFixture(t, 1)
	m.width, m.height = 55, 22
	m.selectedRow(0).run.ExecutionProfile.Name = strings.Repeat("长方案", 1000)
	m, _ = ccKey(m, runeKey('z'))
	view := m.View()
	if m.confirm == nil || !m.operationVisible() || !strings.Contains(view, "Enter 确认退出实验") || !strings.Contains(view, "停止独立核实") {
		t.Fatal("long confirmation displaced critical fields", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatal("long confirm overflowed")
		}
	}
}
