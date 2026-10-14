package console

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestFlowStateTimelineShowsOnlyFlowOnStepChangeAndHidesTechnicalNoise(t *testing.T) {
	at := time.Date(2026, 10, 10, 6, 52, 38, 0, time.UTC)
	changes := []workerTransition{{Time: at, From: workerTransitionFields{WorkflowStage: "implement.report", WorkflowStatus: "running"}, To: workerTransitionFields{WorkflowStage: "publish", WorkflowStatus: "queued"}}}
	for i := 0; i < 20; i++ {
		changes = append(changes, workerTransition{Time: at.Add(time.Duration(i+1) * time.Second), From: workerTransitionFields{WorkflowStage: "publish", WorkflowStatus: "queued", ResourceStatus: "idle", CurrentTurnID: "old", InputVersion: 2}, To: workerTransitionFields{WorkflowStage: "publish", WorkflowStatus: "queued", ResourceStatus: "pending", CurrentTurnID: fmt.Sprint(i), InputVersion: 3}})
	}
	lines := workerTransitionLines(changes)
	if len(lines) != 2 || !strings.Contains(lines[1], "流程  implement.report → publish") || strings.Contains(lines[1], "running") || strings.Contains(lines[1], "queued") {
		t.Fatal("step change displayed status or technical noise displaced flow", lines)
	}
	for _, field := range []string{"resource_status", "current_turn_id", "input_version", "execution_status"} {
		if strings.Contains(strings.Join(lines, "\n"), field) {
			t.Fatal("technical field leaked", field, lines)
		}
	}
	if raw := strings.Join(workerRawTransitionLines(changes), "\n"); !strings.Contains(raw, "current_turn_id:") || !strings.Contains(raw, "resource_status:") || !strings.Contains(raw, "input_version:") {
		t.Fatal("raw changes lost", raw)
	}
	changes = append(changes, workerTransition{Time: at.Add(time.Minute), From: workerTransitionFields{WorkflowStage: "publish", WorkflowStatus: "running"}, To: workerTransitionFields{WorkflowStage: "publish", WorkflowStatus: "blocked"}, Detail: "result_invalid: invalid JSON"})
	lines = workerTransitionLines(changes)
	if !strings.Contains(lines[1], "running → blocked") || !strings.Contains(lines[2], "原因：result_invalid") || strings.Contains(lines[1], "publish → publish") {
		t.Fatal("unchanged step repeated or blocker lost", lines)
	}
}

func TestFlowStateTimelineCountsVisibleMessagesAndKeepsCurrentSnapshot(t *testing.T) {
	var changes []workerTransition
	for i := 0; i < 6; i++ {
		changes = append(changes, workerTransition{From: workerTransitionFields{WorkflowStage: fmt.Sprintf("step-%d", i), WorkflowStatus: "running"}, To: workerTransitionFields{WorkflowStage: "publish", WorkflowStatus: "queued"}, Detail: fmt.Sprintf("reason-%d", i)})
	}
	lines := workerTransitionLines(changes)
	if len(lines) != 11 || !strings.Contains(lines[9], "step-1 → publish") || strings.Contains(strings.Join(lines, "\n"), "step-0") {
		t.Fatal("limit did not retain the latest five flow changes with reasons", lines)
	}
	candidate := &api.CandidateSummary{WorkflowStage: "publish", WorkflowStatus: "queued"}
	if !diagnosticInWorkerTimeline(changes, candidate, "reason-1") || diagnosticInWorkerTimeline(changes, candidate, "reason-0") {
		t.Fatal("diagnostic visibility disagreed with the five displayed flow changes")
	}
	m, _ := ccFixture(t, 1)
	run := m.selectedRow(0).run
	run.WorkflowStage, run.WorkflowStatus = "publish", "future_status"
	m.projects[0].workerTransitions = []workerTransition{{From: workerTransitionFields{WorkflowStage: "publish", WorkflowStatus: "future_status", CurrentTurnID: "old"}, To: workerTransitionFields{WorkflowStage: "publish", WorkflowStatus: "future_status", CurrentTurnID: "new"}}}
	text := strings.Join(feedbackLines(m, m.projects[0], m.selectedRow(0)), "\n")
	if !strings.Contains(text, "当前流程：publish") || !strings.Contains(text, "当前状态：future_status") || strings.Contains(text, "current_turn_id") || strings.Contains(text, "流程与状态") {
		t.Fatal("current snapshot missing or technical event presented as a change", text)
	}
}

func TestTaskListUsesBlockerFactsAndAlignedSingleRows(t *testing.T) {
	m, _ := ccFixture(t, 1)
	s := m.projects[0]
	rows := []consoleRow{
		{kind: rowNative, taskID: "84", title: "中文任务🙂", native: &api.TaskDetail{TaskSummary: api.TaskSummary{Status: "open"}, UnmetPredecessors: []api.DependencySummary{{TaskID: "83", Status: "open", Blocker: "predecessor_not_delivered"}}}},
		{taskID: "85", title: "Another task", native: &api.TaskDetail{TaskSummary: api.TaskSummary{Status: "open"}}, kind: rowReady},
		{kind: rowNative, taskID: "86", title: "长标题" + strings.Repeat("中文", 40), native: &api.TaskDetail{TaskSummary: api.TaskSummary{Status: "open"}}},
		{kind: rowNative, taskID: "87", title: "Unknown state", native: &api.TaskDetail{TaskSummary: api.TaskSummary{Status: "future_blocked"}}},
	}
	if taskRowHint(m, s, rows[0]) != "阻塞" || taskRowBlocked(rows[1]) || taskRowBlocked(rows[2]) || taskRowBlocked(rows[3]) {
		t.Fatal("open or unknown state inferred blocked")
	}
	for _, width := range []int{26, 38, 60} {
		idWidth, statusWidth, countWidth := taskListWidths(width)
		for _, row := range rows {
			lines := compactTaskRow(m, s, row, width)
			if len(lines) != 1 || lipgloss.Width(lines[0]) > width-2 {
				t.Fatal("row wrapped or exceeded viewport", width, lines)
			}
			if !strings.HasPrefix(lines[0], padDisplay(row.taskID, idWidth)+" ") {
				t.Fatal("identity column drifted", lines)
			}
			if width == 60 {
				prefix, _, ok := strings.Cut(lines[0], strings.Split(row.title, " ")[0])
				if !ok && row.taskID != "86" {
					t.Fatal("title disappeared", lines)
				}
				if ok && lipgloss.Width(prefix) != idWidth+statusWidth+countWidth+3 {
					t.Fatal("title column drifted", lines)
				}
			}
		}
	}
	text := strings.Join(candidateOverviewLines(m, s, &rows[0], 80), "\n")
	if !strings.Contains(text, "task_status:      open") || !strings.Contains(text, "依赖阻塞：Task 83 · predecessor_not_delivered") {
		t.Fatal("blocked overview lost original facts", text)
	}
	missing := rows[0]
	native := *missing.native
	native.UnmetPredecessors = []api.DependencySummary{{TaskID: "82", Status: "open"}}
	missing.native = &native
	if detail := strings.Join(taskBlockerLines(missing), "\n"); !taskRowBlocked(missing) || !strings.Contains(detail, "Task 82 · 未提供阻塞原因 · status=open") {
		t.Fatal("missing reason hid an explicit unmet predecessor", detail)
	}
	m.projects[0].snapshot.Tasks = nil
	m.projects[0].snapshot.Observation.Page.Runs = nil
	m.projects[0].snapshot.NativeTasks.Tasks = []api.TaskDetail{*rows[0].native, *rows[2].native}
	m.projects[0].snapshot.NativeTasks.Tasks[0].TaskID = "84"
	m.projects[0].snapshot.NativeTasks.Tasks[1].TaskID = "86"
	m.projects[0].filter = 7
	if blocked := m.rows(0); len(blocked) != 1 || blocked[0].taskID != "84" {
		t.Fatal("blocked filter excluded native blockers or included plain open", blocked)
	}
	if filterName(7) != "blocked" || filterIndex("blocked") != 7 {
		t.Fatal("blocked filter identity drifted")
	}
	active := rows[0]
	active.run = &api.CandidateSummary{TaskStatus: "in_progress", WorkflowStatus: "running"}
	if taskRowBlocked(active) || len(taskBlockerLines(active)) != 0 {
		t.Fatal("old admission blockers replaced an active worker state")
	}
}

func TestTaskHintCombinesAllCandidates(t *testing.T) {
	for _, tc := range []struct {
		name, stage, workflow, execution, task, want string
	}{
		{"merge", "await_merge", "waiting_external", "succeeded", "in_progress", "等待合并"},
		{"other external wait", "publish", "waiting_external", "succeeded", "in_progress", "等待外部"},
		{"decision", "implement.execute", "waiting_input", "succeeded", "in_progress", "待决策"},
		{"running", "implement.execute", "running", "running", "in_progress", "进行中"},
		{"queued", "implement.execute", "queued", "pending", "in_progress", "启动中"},
		{"blocked", "publish", "blocked", "succeeded", "in_progress", "阻塞"},
		{"failed", "await_merge", "waiting_external", "failed", "in_progress", "失败"},
		{"unknown", "await_merge", "waiting_external", "unknown", "in_progress", "待核验"},
		{"closed history", "await_merge", "waiting_external", "failed", "closed", "已关闭"},
		{"discarded", "await_merge", "waiting_external", "succeeded", "discarded", "已丢弃"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := ccFixture(t, 1)
			run := m.selectedRow(0).run
			run.WorkflowStage, run.WorkflowStatus, run.ExecutionStatus, run.TaskStatus = tc.stage, tc.workflow, tc.execution, tc.task
			run.Decision = nil
			if got := taskRowHint(m, m.projects[0], *m.selectedRow(0)); got != tc.want {
				t.Fatalf("task hint = %q, want %q", got, tc.want)
			}
		})
	}
	m, _ := ccFixture(t, 2)
	runs := m.projects[0].snapshot.Observation.Page.Runs
	runs[0].WorkflowStatus, runs[0].ExecutionStatus, runs[0].Decision = "blocked", "failed", nil
	runs[1].WorkflowStage, runs[1].WorkflowStatus, runs[1].ExecutionStatus = "await_merge", "waiting_external", "succeeded"
	for _, selected := range runs {
		m.projects[0].candidates["38"] = selected.RunID
		if got := taskRowHint(m, m.projects[0], *m.selectedRow(0)); got != "等待合并" {
			t.Fatal("selected or failed candidate hid a sibling ready for merge", selected.RunID, got)
		}
	}
	runs[1].PR.QueryError = new(string)
	*runs[1].PR.QueryError = "provider unavailable"
	if got := taskRowHint(m, m.projects[0], *m.selectedRow(0)); got != "待核验" {
		t.Fatal("unverified PR looked ready for merge", got)
	}
}

func TestFlowStateReadableSampleAcrossWindowSizes(t *testing.T) {
	m, _ := ccFixture(t, 1)
	run := m.selectedRow(0).run
	run.WorkflowStage, run.WorkflowStatus, run.ExecutionStatus = "publish", "blocked", "succeeded"
	run.Title = "Worker 流程重构"
	run.ResourceStatus, run.CurrentTurnID = "idle", "hidden-current-turn"
	run.StageDiagnostic = "result_invalid: invalid character 'P' looking for beginning of value"
	run.Decision.Request.Question = "Resolve the current step blocker"
	run.Decision.Request.Options = run.Decision.Request.Options[:2]
	s := &m.projects[0]
	for i := 0; i < 35; i++ {
		s.snapshot.NativeTasks.Tasks = append(s.snapshot.NativeTasks.Tasks, api.TaskDetail{TaskSummary: api.TaskSummary{TaskID: fmt.Sprint(100 + i), Title: fmt.Sprintf("任务 %d 中文标题示例", i), Status: "open"}, UnmetPredecessors: []api.DependencySummary{{TaskID: "84", Blocker: "predecessor_not_delivered"}}})
	}
	s.workerTransitions = []workerTransition{
		{Time: time.Now().Add(-time.Minute), From: workerTransitionFields{WorkflowStage: "implement.report", WorkflowStatus: "running"}, To: workerTransitionFields{WorkflowStage: "publish", WorkflowStatus: "queued"}},
		{Time: time.Now().Add(-30 * time.Second), From: workerTransitionFields{WorkflowStage: "publish", WorkflowStatus: "queued"}, To: workerTransitionFields{WorkflowStage: "publish", WorkflowStatus: "running"}},
		{Time: time.Now(), From: workerTransitionFields{WorkflowStage: "publish", WorkflowStatus: "running", ResourceStatus: "active"}, To: workerTransitionFields{WorkflowStage: "publish", WorkflowStatus: "blocked", ResourceStatus: "idle"}, Detail: run.StageDiagnostic},
	}
	for _, size := range [][2]int{{160, 40}, {120, 30}, {80, 24}} {
		m.width, m.height = size[0], size[1]
		if compactLayout(m) {
			s.focus = 1
		}
		view := m.View()
		if len(strings.Split(view, "\n")) > m.height {
			t.Fatal("sample overflowed window", size)
		}
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > m.width {
				t.Fatal("sample overflowed width", size, line)
			}
		}
		if strings.Contains(view, "hidden-current-turn") || strings.Contains(view, "resource_status:") {
			t.Fatal("technical fields leaked into sample", view)
		}
		if !strings.Contains(view, "当前流程：publish") || !strings.Contains(view, "当前状态：blocked") {
			t.Fatal("current flow/status hidden", size, view)
		}
		if size[0] >= 120 && (strings.Count(view, "invalid character 'P'") != 1 || !strings.Contains(view, "implement.report → publish")) {
			t.Fatal("duplicate diagnostic displaced readable history", size, view)
		}
		t.Logf("%dx%d\n%s", m.width, m.height, view)
	}
}
