package console

import (
	"strings"
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestStateNamingMergeWaitUsesStageAndTerminalFacts(t *testing.T) {
	for _, tc := range []struct {
		name, stage, workflow, task string
		merge                       bool
	}{
		{"merge", "await_merge", "waiting_external", "in_progress", true},
		{"other external wait", "publish", "waiting_external", "in_progress", false},
		{"input", "await_merge", "waiting_input", "in_progress", false},
		{"abandoned", "await_merge", "abandoned", "in_progress", false},
		{"completed", "await_merge", "completed", "in_progress", false},
		{"closed loser", "await_merge", "waiting_external", "closed", false},
		{"discarded", "await_merge", "waiting_external", "discarded", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := &api.CandidateSummary{WorkflowStage: tc.stage, WorkflowStatus: tc.workflow, TaskStatus: tc.task, ExecutionStatus: "succeeded"}
			if got := waitingForMerge(run); got != tc.merge || filterAllows(4, rowRun, run) != tc.merge {
				t.Fatalf("incorrect merge phase/filter: %+v", run)
			}
		})
	}
	if !runNeedsAttention(&api.CandidateSummary{TaskStatus: "discarded", ExecutionStatus: "succeeded"}) {
		t.Fatal("discarded task lost its resource attention after derived status removal")
	}
}

func TestStateNamingClosedInputRemainsHistory(t *testing.T) {
	m, _ := ccFixture(t, 1)
	row := m.selectedRow(0)
	row.run.TaskStatus, row.run.WorkflowStatus = "closed", "waiting_input"
	row.run.Decision = nil
	if waitingForDecision(row.run) || strings.Contains(strings.Join(renderCandidates(m, m.projects[0], row), "\n"), "待决策/冲突") || strings.Contains(strings.Join(candidateDetailLines(m, m.projects[0], row), "\n"), "当前问题尚未同步") {
		t.Fatal("closed candidate history became a pending input action")
	}
}

func TestStateNamingAttentionUsesTheAbnormalAxis(t *testing.T) {
	for _, tc := range []struct {
		name, task, workflow, execution, want string
	}{
		{"workflow blocked after success", "in_progress", "blocked", "succeeded", "需处理 · blocked"},
		{"discarded after success", "discarded", "waiting_external", "succeeded", "需处理 · discarded"},
		{"execution failed during merge wait", "in_progress", "waiting_external", "failed", "需处理 · failed"},
		{"unknown execution with workflow blocked", "in_progress", "blocked", "unknown", "需处理 · unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := &api.CandidateSummary{TaskStatus: tc.task, WorkflowStage: "await_merge", WorkflowStatus: tc.workflow, ExecutionStatus: tc.execution}
			if got := runStatus(run); got != tc.want || !runNeedsAttention(run) || !filterAllows(5, rowRun, run) {
				t.Fatalf("attention names the successful axis: got=%q run=%+v", got, run)
			}
		})
	}
}
