package console

import (
	"errors"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func stopConsoleFixture(t *testing.T, count int) (Model, *testControlClient) {
	m, c := ccFixture(t, count)
	p := m.projects[0].snapshot.Observation.Page
	p.Service.Capabilities = append(p.Service.Capabilities, "stop_candidate")
	for i, r := range p.Runs {
		r.CandidateNumber = i + 1
	}
	m.projects[0].candidates["38"] = p.Runs[0].RunID
	return m, c
}

func TestCandidateStopDraftFailureUnknownResultAndWrongTarget(t *testing.T) {
	m, c := stopConsoleFixture(t, 2)
	m, _ = ccKey(m, runeKey('a'))
	if m.answer == nil {
		t.Fatal("answer draft did not open")
	}
	m.answer.note = []rune("original answer")
	blocked := t.TempDir() + "/blocked"
	if err := os.WriteFile(blocked, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	m.profile.dir = blocked
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF9})
	if m.confirm != nil || m.answer == nil || string(m.answer.note) != "original answer" || c.calls != 0 {
		t.Fatal("stop bypassed failed draft persistence")
	}
	m, c = stopConsoleFixture(t, 2)
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF9})
	id := m.confirm.requestID
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal(m.status)
	}
	a := m.projects[0].actions[id]
	next, followup := m.Update(actionSubmittedMsg{projectKey: "cc", identity: m.projects[0].ref.Identity, instance: a.TargetInstance, requestID: id, err: errors.New("response lost")})
	m = next.(Model)
	if followup == nil || a.Resolved || len(m.projects[0].actions) != 1 || a.Payload.RunID != "candidate-0" {
		t.Fatal("unknown result replaced/resolved original request")
	}
	// A receipt for another candidate must not switch, stop or retarget it.
	next, _ = m.Update(actionReceiptMsg{projectKey: "cc", identity: m.projects[0].ref.Identity, requestID: id, receipt: &api.ControlReceipt{RequestID: id, InstanceID: a.TargetInstance, Action: "stop_candidate", RunID: "candidate-1", Status: "applied"}})
	m = next.(Model)
	if m.selectedRow(0).run.RunID != "candidate-0" || a.RunID != "candidate-0" || a.StopSelectionPending || c.calls != 0 {
		t.Fatal("wrong target receipt changed candidate identity")
	}
}

func TestCandidateStopConfirmationAndSelection(t *testing.T) {
	m, c := stopConsoleFixture(t, 3)
	m.projects[0].final.bodyText = "old candidate reply"
	m.projects[0].recentActivity = []activitySummary{{Title: "old activity"}}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF9})
	if m.confirm == nil || !strings.Contains(strings.Join(confirmRequired(m, m.projects[0]), "\n"), "worker 可能继续运行") {
		t.Fatal("missing candidate/resource confirmation")
	}
	id, target := m.confirm.requestID, m.confirm.payload.RunID
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if c.calls != 0 || m.confirm != nil {
		t.Fatal("cancel submitted a stop")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF9})
	id = m.confirm.requestID
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("stop confirmation did not submit", m.status)
	}
	if m.selectedRow(0).run.RunID != target || m.selectedRow(0).run.WorkflowStatus == "abandoned" {
		t.Fatal("submission optimistically stopped/switched")
	}
	view, err := readViewFile(m.profile.dir + "/view.json")
	if err != nil || view.Projects[m.projects[0].ref.Key].Actions[id].Payload.RunID != target {
		t.Fatal("fixed request was not saved", err)
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if c.request.Action != "stop_candidate" || c.request.RequestID != id {
		t.Fatal("wrong stop identity")
	}
	a := m.projects[0].actions[id]
	next, _ = m.Update(actionReceiptMsg{projectKey: "cc", identity: m.projects[0].ref.Identity, requestID: id, receipt: &api.ControlReceipt{RequestID: id, InstanceID: a.TargetInstance, Action: "stop_candidate", Status: "applied", RunID: target}})
	m = next.(Model)
	if m.selectedRow(0).run.RunID != target {
		t.Fatal("receipt guessed a survivor before the snapshot")
	}
	for _, r := range m.projects[0].snapshot.Observation.Page.Runs {
		if r.RunID == target {
			r.WorkflowStatus = "abandoned"
			r.CandidateStopReason = "manual"
		}
	}
	m.selectAfterCandidateStop(0)
	if m.selectedRow(0).run.CandidateNumber != 3 || m.projects[0].final.bodyText != "" || len(m.projects[0].recentActivity) != 0 {
		t.Fatal("stop did not select highest remaining candidate and clear content")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF6})
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF6})
	if m.selectedRow(0).run.RunID != target || pendingDecision(m.selectedRow(0).run) || !strings.Contains(candidateHumanStatus(m.projects[0], m.selectedRow(0).run), "已停止参与") {
		t.Fatal("stopped history not read-only")
	}
	row := m.selectedRow(0)
	row.run.CandidateStoppedBy = "operator\x1b[2J\nforged line"
	lines := renderCandidates(m, m.projects[0], row)
	for _, line := range lines {
		if strings.Contains(line, "\x1b[2J") || strings.Contains(line, "operator\n") || strings.Contains(line, target) && strings.Contains(line, "参与实验") {
			t.Fatal("stopped candidate rendered unsafe or contradictory status", line)
		}
	}
}

func TestCandidateStopLastPlaceholderAndLateReceipt(t *testing.T) {
	m, _ := stopConsoleFixture(t, 1)
	m.projects[0].actions["pending"] = &actionState{RequestID: "pending", Action: "start_candidate", TaskID: "38", CandidateNumber: 2, Status: "submitting"}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF9})
	if m.confirm != nil || !strings.Contains(m.status, "只剩一个") {
		t.Fatal("local placeholder bypassed last-participant protection", m.status)
	}
	m.projects[0].candidates["38"] = "pending"
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF9})
	if m.confirm != nil || !strings.Contains(m.status, "服务器确认") {
		t.Fatal("placeholder could stop", m.status)
	}
	m, _ = stopConsoleFixture(t, 3)
	m.projects[0].snapshot.NativeTasks.Tasks[0].Status = "closed"
	m.projects[0].snapshot.Observation.Page.Control.DispatchMode = "auto"
	m.projects[0].snapshot.Observation.Page.Control.DispatchPaused = true
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyF9})
	if m.confirm == nil {
		t.Fatal("Task/mode gated candidate stop", m.status)
	}
	id, target := m.confirm.requestID, m.confirm.payload.RunID
	m, cmd := ccKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal(m.status)
	}
	a := m.projects[0].actions[id]
	m.projects[0].candidates["38"] = "candidate-1"
	next, _ := m.Update(actionReceiptMsg{projectKey: "cc", identity: m.projects[0].ref.Identity, requestID: id, receipt: &api.ControlReceipt{RequestID: id, Action: "stop_candidate", InstanceID: a.TargetInstance, RunID: target, Status: "applied"}})
	m = next.(Model)
	m.projects[0].snapshot.Observation.Page.Runs[0].WorkflowStatus = "abandoned"
	m.selectAfterCandidateStop(0)
	if m.selectedRow(0).run.RunID != "candidate-1" {
		t.Fatal("late stop receipt stole selection")
	}
}
