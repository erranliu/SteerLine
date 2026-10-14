package console

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/muesli/termenv"
)

func TestWorkerStatusRawFieldsAndFixedLabels(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	if got := workerValue("workflow_status", "future失败", true); got != "future失败" {
		t.Fatal("unknown value acquired inferred color or label", got)
	}
	if got := workerValue("workflow_status", "running", true); !strings.Contains(got, "running（运行中）") {
		t.Fatal("raw value was replaced", got)
	}
	if strings.Contains(workerValue("workflow_stage", "running", true), "运行中") {
		t.Fatal("mapping crossed field namespaces")
	}
	if got := workerValue("workflow_status", "in_progress", true); got != "in_progress" {
		t.Fatal("foreign field value acquired a guessed style", got)
	}
	if got := workerValue("workflow_stage", "publish.report", true); !strings.Contains(got, "publish.report（发布报告）") {
		t.Fatal("publication report label missing", got)
	}
	m, _ := ccFixture(t, 1)
	run := m.selectedRow(0).run
	run.WorkflowStage, run.WorkflowStatus, run.ExecutionStatus = "implement.report", "future_state_with_a_very_long_name", "running"
	lines := candidateOverviewLines(m, m.projects[0], m.selectedRow(0), 28)
	plain := strings.ReplaceAll(strings.Join(lines, ""), " ", "")
	for _, raw := range []string{"implement.report", "task_status:"} {
		if !strings.Contains(plain, raw) {
			t.Fatal("raw field was truncated instead of wrapped", raw, lines)
		}
	}
	for _, hidden := range []string{run.WorkflowStatus, "execution_status:", "workflow_status:"} {
		if strings.Contains(plain, hidden) {
			t.Fatal("hidden status leaked into candidate overview", hidden, lines)
		}
	}
	m.detailExpanded = true
	detail := strings.Join(candidateDetailLines(m, m.projects[0], m.selectedRow(0)), "\n")
	for _, raw := range []string{run.WorkflowStatus, "execution_status:", "workflow_status:"} {
		if !strings.Contains(detail, raw) {
			t.Fatal("raw state disappeared from details", raw, detail)
		}
	}
	for _, line := range lines {
		if lipgloss.Width(line) > 28 {
			t.Fatal("wrapped field exceeded pane", line)
		}
	}
}

func TestWorkerStatusActivityPreservesFieldsAndFocus(t *testing.T) {
	m, _ := ccFixture(t, 1)
	m.width, m.height = 160, 40
	run := m.selectedRow(0).run
	run.WorkflowStage, run.WorkflowStatus = "implement.report", "running"
	m, _ = m.refreshRecentActivity(0)
	state := m.projects[0]
	state.lowerScroll = 2
	m.projects[0].lowerScroll = 2
	raw := map[string]any{"from": map[string]any{"workflow_stage": "implement.execute", "workflow_status": "running", "execution_status": "running"}, "to": map[string]any{"workflow_stage": "implement.report", "workflow_status": "queued", "execution_status": "succeeded", "input_version": 2}}
	event := api.ActivityEvent{Kind: "worker_state", Seq: 1, TS: time.Now(), Meta: raw, Title: "do not infer state from this title"}
	batch := api.LogBatch{Cursor: api.ActivityCursor{RunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, Initialized: true}, Activity: []api.ActivityEvent{event}}
	msg := recentActivityMsg{state.ref.Key, state.ref.Identity, state.activityKey, state.activityGeneration, batch, nil}
	next, _ := m.Update(msg)
	m = next.(Model)
	next, _ = m.Update(msg)
	m = next.(Model)
	if len(m.projects[0].workerTransitions) != 1 || m.projects[0].lowerScroll != 2 || m.projects[0].focus != state.focus {
		t.Fatal("repeated activity duplicated changes or moved focus")
	}
	text := strings.Join(feedbackLines(m, m.projects[0], m.selectedRow(0)), "\n")
	for _, want := range []string{"问题：", "流程与状态", "implement.execute → implement.report", "当前状态：running", "当前流程："} {
		if !strings.Contains(text, want) {
			t.Fatal("question or original transition fields were hidden", want, text)
		}
	}
	if strings.Contains(text, event.Title) {
		t.Fatal("display inferred state from prose")
	}
	if strings.Contains(text, "execution_status:") || strings.Contains(text, "input_version:") || strings.Contains(text, "running → queued") {
		t.Fatal("technical changes leaked into default timeline", text)
	}
	m.detailExpanded = true
	if raw := strings.Join(candidateDetailLines(m, m.projects[0], m.selectedRow(0)), "\n"); !strings.Contains(raw, "execution_status:") || !strings.Contains(raw, "input_version:") {
		t.Fatal("raw fields disappeared from details", raw)
	}
	m.detailExpanded = false
	malformed := msg
	malformed.batch.Activity = []api.ActivityEvent{{Kind: "worker_state", Seq: 2, Meta: map[string]any{"to": "invalid fields"}}}
	next, _ = m.Update(malformed)
	m = next.(Model)
	if !strings.Contains(m.projects[0].activityError, "seq=2") || len(m.projects[0].workerTransitions) != 0 {
		t.Fatal("malformed state transition disappeared without a diagnostic")
	}
	next, _ = m.Update(msg)
	m = next.(Model)
	m.projects[0].activityError = "original activity source gap"
	if !strings.Contains(strings.Join(feedbackLines(m, m.projects[0], m.selectedRow(0)), "\n"), "original activity source gap") {
		t.Fatal("pending question hid the activity gap")
	}
	run.Decision = nil
	run.WorkflowStage, run.WorkflowStatus, run.ExecutionStatus = "delivered", "completed", "succeeded"
	if !strings.Contains(strings.Join(feedbackLines(m, m.projects[0], m.selectedRow(0)), "\n"), "流程与状态") {
		t.Fatal("terminal state hid transition history")
	}
	run.AttemptID = "another-attempt"
	m, _ = m.refreshRecentActivity(0)
	next, _ = m.Update(msg)
	m = next.(Model)
	if len(m.projects[0].workerTransitions) != 0 {
		t.Fatal("old Attempt transition reached new view")
	}
}

func TestWorkerStatusSingleRowsKeepSelectionVisible(t *testing.T) {
	m, _ := ccFixture(t, 1)
	s := &m.projects[0]
	s.snapshot.Observation.Page.Runs = nil
	s.snapshot.NativeTasks.Tasks = nil
	for i := 0; i < 5; i++ {
		run := &api.CandidateSummary{TaskID: fmt.Sprint(i), RunID: "run-" + fmt.Sprint(i), Title: "title", TaskStatus: "in_progress", WorkflowStage: "implement.report", WorkflowStatus: "waiting_external", ExecutionStatus: "running"}
		s.snapshot.Observation.Page.Runs = append(s.snapshot.Observation.Page.Runs, run)
		s.snapshot.NativeTasks.Tasks = append(s.snapshot.NativeTasks.Tasks, api.TaskDetail{TaskSummary: api.TaskSummary{TaskID: run.TaskID, Status: run.TaskStatus}})
	}
	s.selectedKey = "task:3"
	lines := renderRows(m, *s, 26, 7)
	text := strings.Join(lines, "\n")
	if len(lines) > 7 || !strings.Contains(text, "▸ 3") || strings.Contains(text, "waiting_external") {
		t.Fatal("list lost selected row or leaked worker details", lines)
	}
}

func TestWorkerStatusCompactOverviewDoesNotRepeatDiagnostics(t *testing.T) {
	m, _ := ccFixture(t, 1)
	run := m.selectedRow(0).run
	run.WorkflowStage, run.WorkflowStatus = "publish", "blocked"
	run.StageDiagnostic = "original publishing failure"
	run.Error = &run.StageDiagnostic
	overview := strings.Join(candidateOverviewLines(m, m.projects[0], m.selectedRow(0), 80), "\n")
	feedback := strings.Join(feedbackLines(m, m.projects[0], m.selectedRow(0)), "\n")
	if strings.Count(overview, "original publishing failure") != 1 || strings.Contains(feedback, "original publishing failure") || strings.Count(overview+feedback, "下一步：") != 1 {
		t.Fatal("compact panes duplicated diagnostic or next action")
	}
	for _, size := range [][2]int{{55, 22}, {80, 24}, {160, 40}} {
		m.width, m.height = size[0], size[1]
		m.projects[0].focus = 1
		view := m.View()
		for _, want := range []string{"workflow_stage:", "publish", "task_status:"} {
			if !strings.Contains(view, want) {
				t.Fatal("main surface hid current raw state", size, want, view)
			}
		}
		for _, hidden := range []string{"workflow_status:", "execution_status:"} {
			if strings.Contains(view, hidden) {
				t.Fatal("hidden raw status leaked into main surface", size, hidden, view)
			}
		}
	}
}
