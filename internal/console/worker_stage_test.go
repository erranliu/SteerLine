package console

import (
	api "github.com/erranliu/SteerLine/internal/product/api"
	"strings"
	"testing"
)

func TestWorkerStagePresentation(t *testing.T) {
	run := &api.CandidateSummary{FlowRef: "development/v1", StepID: "implement.report", BusinessStatus: "blocked", ReportToolPolicy: "frozen_native_permissions; fact_comparison_only", StageResults: []api.StageResultSummary{{StepID: "implement.execute", Succeeded: false, Failure: "implementation failed"}, {StepID: "implement.report", Succeeded: true, FinalSource: "native.final_answer"}}}
	text := strings.Join(workerStagePresentation(run), "\n")
	for _, want := range []string{"implement.report", "blocked", "执行失败，报告已生成", "无法保证拦截全部外部写入"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	run.StepID, run.BusinessStatus = "await_merge", "waiting_external"
	run.FinishTaskStatus = "waiting_pr_merge"
	run.StageResults = []api.StageResultSummary{{StepID: "publish", Succeeded: true, FinalSource: "native.final_answer"}}
	text = strings.Join(workerStagePresentation(run), "\n")
	if strings.Contains(text, "报告已生成") || !waitingForMerge(run) || waitingForDecision(run) {
		t.Fatal("publish invented a report or waiting merge required continuation", text)
	}
}
