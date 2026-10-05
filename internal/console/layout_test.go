package console

import (
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestTaskSelectionAndTwoFocusRegions(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	snapshot := readySnapshot(ref, now)
	snapshot.Tasks.Tasks = append(snapshot.Tasks.Tasks, api.TaskCandidate{TaskID: "task-C", Title: "Build C"})
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.width, m.height = 120, 40
	view := m.View()
	for _, want := range []string{"任务列表 [焦点]", "▸", "详情", "任务动态 · 活动", "────────"} {
		if !strings.Contains(view, want) {
			t.Fatalf("stacked layout lost %q:\n%s", want, view)
		}
	}
	selected := m.projects[0].selectedKey
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.projects[0].focus != 1 || !strings.Contains(m.View(), "任务动态 [焦点]") || !strings.Contains(m.View(), "▸") {
		t.Fatal("activity focus hid the selected task")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.projects[0].selectedKey != selected {
		t.Fatal("activity scrolling changed the selected task")
	}
	if !m.projects[0].view.Follow {
		t.Fatal("scrolling an empty activity pane disabled follow mode")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.projects[0].focus != 0 || m.projects[0].selectedKey == selected {
		t.Fatal("list focus did not move task selection")
	}
}

func TestProjectSwitchKeepsCurrentIdentityVisibleAndBackgroundNoticeIsolated(t *testing.T) {
	now := time.Date(2026, 9, 26, 1, 0, 0, 0, time.UTC)
	refs := []ProjectRef{testProject("a-long-project-name"), testProject("SteerlineTest"), testProject("merge-h-308-code"), testProject("slot2")}
	snapshots := make([]ProjectSnapshot, len(refs))
	for i := range refs {
		snapshots[i] = readySnapshot(refs[i], now)
	}
	m := modelForTest(refs, snapshots, &testControlClient{}, now)
	m.width = 80
	for i := range refs {
		strip := renderProjectStrip(m)
		if !strings.Contains(strip, refs[i].Name) || !strings.Contains(strip, "项目 "+string(rune('1'+i))+"/4") {
			t.Fatalf("current project lost after switch %d: %q", i, strip)
		}
		if i < len(refs)-1 {
			updated, _ := m.moveProject(1)
			m = updated.(Model)
		}
	}
	m.projects[0].selectedKey = "task:removed"
	m.projects[0].refreshID = 9
	m.status = ""
	updated, _ := m.Update(projectMsg{key: refs[0].Key, identity: refs[0].Identity, requestID: 9, snapshot: readySnapshot(refs[0], now)})
	m = updated.(Model)
	if m.selected != 3 || m.status != "" || !strings.Contains(renderProjectStrip(m), "slot2") {
		t.Fatalf("background refresh changed active project: selected=%d status=%q strip=%q", m.selected, m.status, renderProjectStrip(m))
	}
}

func TestColumnsAlignWithChineseAndLongTaskTitles(t *testing.T) {
	left := []string{"中文", styleSelected.Render("短")}
	right := []string{"详情 A", "详情 B"}
	for _, line := range strings.Split(joinLines(left, right, 24, 2), "\n") {
		before, _, ok := strings.Cut(line, "│")
		if !ok || lipgloss.Width(before) != 24 {
			t.Fatalf("detail column drifted: %q", line)
		}
	}
	for _, width := range []int{34, 80} {
		row := renderTaskListRow("n308-28", strings.Repeat("中文任务", 20), "已完成", "失败（历史）", width)
		if lipgloss.Width(row) > width || !strings.Contains(row, "已完成") || !strings.Contains(row, "n308-28") || !strings.Contains(row, "失败") {
			t.Fatalf("task row lost columns at %d columns: %q", width, row)
		}
	}
}

func TestCompletedTaskShowsHistoricalExecutionInFourColumns(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("steer")
	snapshot := readySnapshot(ref, now)
	snapshot.Tasks = nil
	run := &api.CandidateSummary{TaskID: "3", RunID: "run-3", Title: "人工决策与恢复闭环", TaskStatus: "closed", ExecutionStatus: "failed", FinishTaskStatus: "blocked", EndedAt: &now}
	snapshot.Observation.Page.Runs = []*api.CandidateSummary{run}
	snapshot.NativeTasks = &api.TaskPage{Tasks: []api.TaskDetail{{TaskSummary: api.TaskSummary{TaskID: "3", Title: run.Title, Status: "closed"}}}}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.projects[0].selectedKey = "task:3"
	m.width = 100
	rows := m.rows(0)
	if len(rows) != 1 || rowTaskStatus(rows[0]) != "已完成" || rowExecutionStatus(rows[0]) != "失败（历史）" {
		t.Fatalf("closed task columns: %+v", rows)
	}
	list := strings.Join(renderRows(m, m.projects[0], listPanelWidth(m), 12), "\n")
	for _, want := range []string{"任务编号", "任务状态", "执行状态", "任务名称", "已完成", "失败（历史）", run.Title} {
		if !strings.Contains(list, want) {
			t.Fatalf("four-column list lost %q:\n%s", want, list)
		}
	}
	header := ""
	for _, line := range strings.Split(list, "\n") {
		if strings.Contains(line, "任务编号") {
			header = line
			break
		}
	}
	if !(strings.Index(header, "任务编号") < strings.Index(header, "任务状态") && strings.Index(header, "任务状态") < strings.Index(header, "执行状态") && strings.Index(header, "执行状态") < strings.Index(header, "任务名称")) {
		t.Fatalf("list columns are out of order: %q", header)
	}
	detail := strings.Join(renderDetail(m, m.projects[0], &rows[0], 80, 80), "\n")
	for _, want := range []string{"任务状态：已完成", "执行状态：失败（历史）", "需要你操作：无"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("completed task detail lost %q:\n%s", want, detail)
		}
	}
	if listPanelWidth(m) < 55 || activityPanelWidth(m) < 40 {
		t.Fatalf("panel widths do not fit four columns: list=%d detail=%d", listPanelWidth(m), activityPanelWidth(m))
	}
	_, _, _, _, _, attention := projectCounts(m.projects[0], now)
	if attention != 0 || filterAllows(5, rowRun, run) || !filterAllows(6, rowRun, run) || taskNextAction(run) != "无" {
		t.Fatalf("settled historical failure still needs action: attention=%d", attention)
	}
	if run.ExecutionStatus != "failed" || run.FinishTaskStatus != "blocked" {
		t.Fatal("historical execution facts were rewritten")
	}
}

func TestTaskPreviewKeepsOneIdentityAndShowsFixedExecutionPlan(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("native")
	ref.Presentation.TaskAuthority = "native"
	snapshot := readySnapshot(ref, now)
	snapshot.Tasks = nil
	run := &api.CandidateSummary{TaskID: "102", DomainTaskID: "102", RunID: "run-102", Title: "进行中的任务", TaskStatus: "in_progress", ExecutionStatus: "running"}
	snapshot.Observation.Page.Runs = []*api.CandidateSummary{run}
	snapshot.NativeTasks = &api.TaskPage{Tasks: []api.TaskDetail{{TaskSummary: api.TaskSummary{TaskID: "102", Title: run.Title, Status: "in_progress"}, Successors: []string{"104"}}}}
	fixed := api.ExecutionProfileSummary{ID: "orca-local", Name: "本机执行", Host: "orca", Agent: "codex", ImplementationDisplay: []string{"Model：gpt-6-luna", "Reasoning effort：xhigh"}}
	snapshot.History = &api.HistorySummary{WorkflowRuns: []api.WorkflowSummary{{WorkflowRunID: run.RunID, ExecutionProfile: &fixed}}}
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.projects[0].selectedKey = "task:" + run.TaskID
	row := m.selectedRow(0)
	if row == nil || row.run == nil {
		t.Fatal("running task was not selected")
	}
	preview := strings.Join(renderDetailPreview(m, m.projects[0], row, 120, 100), "\n")
	for _, want := range []string{"执行方案：orca · codex · Model：gpt-6-luna · Reasoning effort：xhigh", "任务关系", "后继任务：104"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("task preview lost %q: %s", want, preview)
		}
	}
	for _, hidden := range []string{"任务状态：", "执行状态：", "run_id：", "中心只读快照"} {
		if strings.Contains(preview, hidden) {
			t.Fatalf("task preview repeated %q: %s", hidden, preview)
		}
	}
	full := strings.Join(renderDetail(m, m.projects[0], row, 120, 100), "\n")
	if strings.Count(full, "任务 102 · 进行中的任务") != 1 || !strings.Contains(full, "任务状态：进行中") || !strings.Contains(full, "执行状态：执行中") || !strings.Contains(full, "run_id：run-102") {
		t.Fatalf("full detail lost facts or repeated task identity: %s", full)
	}
}

func TestOpenTaskDisplaysAsBlockedWithoutChangingPendingExecution(t *testing.T) {
	if displayTaskStatus("open") != "阻塞" || displayExecutionStatus(&api.CandidateSummary{ExecutionStatus: "pending"}) != "待开始" {
		t.Fatal("task and execution statuses were conflated")
	}
}

func TestPendingDecisionPreviewHidesRunIdentity(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("decision")
	snapshot, run := decisionSnapshot(ref, now)
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{snapshot}, &testControlClient{}, now)
	m.projects[0].selectedKey = "task:" + run.TaskID
	row := m.selectedRow(0)
	preview := strings.Join(renderDetailPreview(m, m.projects[0], row, 120, 200), "\n")
	full := strings.Join(renderDetail(m, m.projects[0], row, 120, 200), "\n")
	if strings.Contains(preview, "run_id：") || !strings.Contains(preview, "问题：") || !strings.Contains(full, "run_id："+run.RunID) {
		t.Fatalf("decision preview/full identity mismatch: preview=%s full=%s", preview, full)
	}
}

func TestClosedTaskWithUncertainExecutionStillNeedsAttention(t *testing.T) {
	now := time.Now().UTC()
	run := &api.CandidateSummary{TaskStatus: "closed", ExecutionStatus: "unknown", FinishTaskStatus: "blocked"}
	if !runNeedsAttention(run) || !filterAllows(5, rowRun, run) || filterAllows(6, rowRun, run) {
		t.Fatal("unknown execution disappeared from attention")
	}
	run.ExecutionStatus, run.EndedAt = "failed", nil
	if !runNeedsAttention(run) {
		t.Fatal("unterminated failed execution was treated as history")
	}
	run.EndedAt = &now
	run.Error = stringPointer("unresolved external resource")
	if !runNeedsAttention(run) {
		t.Fatal("failed execution with an unresolved error was treated as history")
	}
}

func TestNativeTaskDetailKeepsBlockersWithoutPlanOrHashes(t *testing.T) {
	task := api.TaskDetail{
		TaskSummary:  api.TaskSummary{TaskID: "12", Title: "中文任务", Status: "open"},
		Predecessors: []api.DependencySummary{{TaskID: "9", Status: "discarded", Blocker: "discarded_predecessor"}},
	}
	task.SpecPath = "docs/spec/task.spec.md"
	detail := strings.Join(renderNativeTaskDetail(projectState{}, task), "\n")
	for _, want := range []string{"中文任务", "discarded_predecessor", "docs/spec/task.spec.md"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail lost %q: %q", want, detail)
		}
	}
	for _, hidden := range []string{"task.plan.md", "secret-document-hash", "sha256", "version 0"} {
		if strings.Contains(detail, hidden) {
			t.Fatalf("detail exposed %q: %q", hidden, detail)
		}
	}
}

func TestBackgroundDecisionRejectionDoesNotOpenAnotherProjectsDraft(t *testing.T) {
	now := time.Now().UTC()
	a, b := testProject("a"), testProject("b")
	m := modelForTest([]ProjectRef{a, b}, []ProjectSnapshot{readySnapshot(a, now), readySnapshot(b, now)}, &testControlClient{}, now)
	m.projects[0].actions["decision-req"] = &actionState{RequestID: "decision-req", Action: "answer_decision", RunID: "run-a", DecisionID: "decision-a"}
	m.selected = 1
	updated, _ := m.Update(actionSubmittedMsg{projectKey: a.Key, identity: a.Identity, requestID: "decision-req", err: errors.New("stale_revision")})
	m = updated.(Model)
	if m.answer != nil || m.status != "" || m.selected != 1 || m.projects[0].actions["decision-req"].Status != "rejected" {
		t.Fatalf("background answer changed current project UI: selected=%d status=%q dialog=%+v", m.selected, m.status, m.answer)
	}
}

func TestHelpAndCompactLayoutFitNarrowAndWideScreens(t *testing.T) {
	now := time.Now().UTC()
	ref := testProject("alpha")
	m := modelForTest([]ProjectRef{ref}, []ProjectSnapshot{readySnapshot(ref, now)}, &testControlClient{}, now)
	for _, width := range []int{80, 120} {
		m.width, m.height = width, 24
		for _, help := range []bool{false, true} {
			m.help = help
			view := m.View()
			lines := strings.Split(view, "\n")
			if len(lines) != 24 {
				t.Fatalf("view has %d lines at width %d", len(lines), width)
			}
			for _, line := range lines {
				if lipgloss.Width(line) > width {
					t.Fatalf("view exceeds %d columns: %q", width, line)
				}
			}
		}
	}
}
