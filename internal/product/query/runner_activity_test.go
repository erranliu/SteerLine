package query

import (
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestRunnerActivityTransitionDisplay(t *testing.T) {
	for _, tc := range []struct {
		name string
		from runnercontract.WorkflowState
		to   runnercontract.WorkflowState
		want string
	}{
		{"initial", runnercontract.WorkflowState{}, runnercontract.WorkflowState{Stage: "implement.execute", Status: "running"}, "Workflow 开始：implement.execute（执行中）"},
		{"workflow only", runnercontract.WorkflowState{Stage: "implement.execute", Status: "running"}, runnercontract.WorkflowState{Stage: "implement.report", Status: "running"}, "Workflow 切换：implement.execute → implement.report（执行中）"},
		{"workflow and status", runnercontract.WorkflowState{Stage: "implement.report", Status: "running"}, runnercontract.WorkflowState{Stage: "audit.execute", Status: "blocked", Diagnostic: "HTTP 401"}, "Workflow 切换：implement.report → audit.execute（已阻塞）"},
		{"status only", runnercontract.WorkflowState{Stage: "implement.report", Status: "running"}, runnercontract.WorkflowState{Stage: "implement.report", Status: "waiting_external"}, "状态切换：执行中 → 等待外部事实"},
		{"diagnostic only", runnercontract.WorkflowState{Stage: "publish", Status: "blocked", Diagnostic: "old"}, runnercontract.WorkflowState{Stage: "publish", Status: "blocked", Diagnostic: "new"}, "诊断信息更新"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := api.ActivityEvent{Seq: 42, ItemID: "original-event"}
			got := runnerWorkflowActivity(original, runnercontract.WorkflowTransition{From: tc.from, To: tc.to})
			if got.Title != tc.want {
				t.Fatalf("title = %q, want %q", got.Title, tc.want)
			}
			if got.Seq != original.Seq || got.ItemID != original.ItemID || got.Status != tc.to.Status || got.Detail != tc.to.Diagnostic || got.Meta["from"] != tc.from || got.Meta["to"] != tc.to {
				t.Fatalf("display changed original event facts: %+v", got)
			}
		})
	}
}
