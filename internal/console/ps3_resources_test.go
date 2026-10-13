package console

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestPS3RetainedResourceDetailUsesSavedNativeLocation(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("retained")
	snapshot := readySnapshot(ref, now)
	run := &api.CandidateSummary{TaskID: "3", RunID: "run-3", TaskStatus: "discarded", ExecutionStatus: "active", WorkflowStatus: "blocked", Backend: "orca-remote", Protocol: "remote-v1", Target: api.TargetSummary{ID: "original-target", WorktreeID: "original-native-tree", Branch: "original-branch"}, PR: api.PRSummary{State: "unknown"}, PRHistory: []string{"https://example.invalid/pull/3"}}
	snapshot.Tasks = nil
	snapshot.SelectedNative = nil
	snapshot.Observation.Page.Runs = []*api.CandidateSummary{run}
	snapshot.NativeTasks = &api.TaskPage{Tasks: []api.TaskDetail{{TaskSummary: api.TaskSummary{TaskID: "3", Status: "discarded"}, StopPending: []string{"execution_stop_manual:original"}}}}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.detailExpanded = true
	rows := m.rows(0)
	if len(rows) != 1 {
		t.Fatal("historical candidate missing")
	}
	lines := strings.Join(candidateDetailLines(m, m.projects[0], &rows[0]), "\n")
	for _, want := range []string{"保留，需人工处置", "未确认当前存在", "采样于", "original-native-tree", "original-branch", "original-target", "active", "待核实原执行停止", "https://example.invalid/pull/3"} {
		if !strings.Contains(lines, want) {
			t.Fatalf("lost %s: %s", want, lines)
		}
	}
	if strings.Contains(lines, "已清理") {
		t.Fatal("historical location was described as cleaned")
	}
}

func TestPS3DiscardedCandidateRejectsStaleQuestionAndMergeHints(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("discarded")
	snapshot := readySnapshot(ref, now)
	run := &api.CandidateSummary{TaskID: "3", RunID: "run-3", TaskStatus: "discarded", ExecutionStatus: "active", WorkflowStatus: "waiting_input", Decision: &api.DecisionSummary{ID: "decision-3", Status: "pending", Version: 1}, PR: api.PRSummary{State: "merged"}}
	snapshot.Tasks = nil
	snapshot.Observation.Page.Runs = []*api.CandidateSummary{run}
	snapshot.Connection.Snapshot.Page = snapshot.Observation.Page
	snapshot.DecisionReasons = map[string]string{run.RunID: "提交后发送原答案"}
	snapshot.NativeTasks = &api.TaskPage{Tasks: []api.TaskDetail{{TaskSummary: api.TaskSummary{TaskID: "3", Status: "discarded"}}}}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.projects[0].selectedKey = "task:3"
	rows := m.rows(0)
	if len(rows) != 1 {
		t.Fatal("candidate missing")
	}
	state := m.projects[0]
	if strings.HasPrefix(decisionAnswerAvailability(state, &rows[0]), "提交后") {
		t.Fatal("stale answer eligibility reused")
	}
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.answer != nil {
		t.Fatal("discarded task opened an answer editor")
	}
	if !strings.Contains(nextAction(m, state, &rows[0]), "冲突") || strings.Contains(taskNextAction(run), "回答") {
		t.Fatal("discarded task suggested business input")
	}
	run.WorkflowStage, run.WorkflowStatus = "await_merge", "waiting_external"
	if strings.Contains(nextAction(m, state, &rows[0]), "人工合并") {
		t.Fatal("discarded task suggested normal delivery")
	}
	candidates := strings.Join(renderCandidates(m, state, &rows[0]), "\n")
	if !strings.Contains(candidates, "冲突：已丢弃 Task") || strings.Contains(candidates, "交付待核验") {
		t.Fatal("late discarded merge was displayed as pending delivery")
	}
	if run.ExecutionStatus != "active" {
		t.Fatal("business discard invented resource stop")
	}
}

func TestPS3AuditFinalSubmitRejectsDiscardedTaskEvenWithUnchangedDecision(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("audit-submit")
	snapshot := readySnapshot(ref, now)
	run := &api.CandidateSummary{TaskID: "3", RunID: "run-3", TaskStatus: "in_progress", ExecutionStatus: "waiting_decision", WorkflowStatus: "waiting_input", ControlRevision: 7, Decision: &api.DecisionSummary{ID: "decision-3", Status: "pending", Version: 1, Request: api.DecisionRequest{Question: "original question"}}}
	snapshot.Tasks = nil
	snapshot.Observation.Page.Runs = []*api.CandidateSummary{run}
	snapshot.Connection.Snapshot.Page = snapshot.Observation.Page
	snapshot.DecisionReasons = map[string]string{run.RunID: "提交后发送原答案"}
	snapshot.NativeTasks = &api.TaskPage{Tasks: []api.TaskDetail{{TaskSummary: api.TaskSummary{TaskID: "3", Status: "in_progress"}}}}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.projects[0].selectedKey = "task:3"
	m.width = 100
	m.height = 30
	m, _ = ccKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.answer == nil {
		t.Fatal("original active question did not open")
	}
	m.answer.note = []rune("saved original draft")
	m.projects[0].snapshot.Connection.Snapshot.Page.Runs[0].TaskStatus = "discarded"
	next, cmd := m.submitDecisionAnswer()
	m = next.(Model)
	if cmd != nil || len(m.projects[0].actions) != 0 {
		t.Fatal("known discarded task produced an answer command")
	}
	if m.answer == nil || string(m.answer.note) != "saved original draft" {
		t.Fatal("blocked submit lost draft")
	}
}
