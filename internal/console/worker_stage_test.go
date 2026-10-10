package console

import (
	api "github.com/erranliu/SteerLine/internal/product/api"
	"strings"
	"testing"
)

func TestWorkerStagePresentation(t *testing.T) {
	run := &api.CandidateSummary{FlowRef: "development/v2", WorkflowStage: "implement.report", WorkflowStatus: "blocked", ReportToolPolicy: "frozen_native_permissions; fact_comparison_only", StageResults: []api.StageResultSummary{{StepID: "implement.execute", Succeeded: false, Failure: "implementation failed", Final: "Original compiler failure details", FinalVerified: true, FinalSource: "native.final_answer"}, {StepID: "implement.report", Succeeded: true, FinalVerified: true, FinalSource: "native.final_answer"}}}
	text := strings.Join(workerStagePresentation(run), "\n")
	for _, want := range []string{"implement.report", "blocked", "执行失败，报告已生成", "Original compiler failure details", "无法保证拦截全部外部写入"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	run.WorkflowStage, run.WorkflowStatus = "await_merge", "waiting_external"
	run.WorkflowStatus = "waiting_external"
	run.StageResults = []api.StageResultSummary{{StepID: "publish", Succeeded: true, FinalVerified: true, FinalSource: "native.final_answer"}}
	text = strings.Join(workerStagePresentation(run), "\n")
	if strings.Contains(text, "报告已生成") || !waitingForMerge(run) || waitingForDecision(run) {
		t.Fatal("publish invented a report or waiting merge required continuation", text)
	}
}
