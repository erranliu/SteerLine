//go:build e2e

package smoke

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestDeliverySmoke(t *testing.T) {
	env := testenv.NewScenario(t, "smoke.delivery")
	p := env.NewProject("delivery")
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	open := p.WaitPhase("waiting_pr_merge")
	p.Record("open", open)
	if len(open.Page.Tasks) != 2 || open.Helper.Sessions != 1 || open.Helper.Dispatches != 1 || open.Helper.PR.State != "open" {
		t.Fatalf("incomplete controlled execution: %+v", open)
	}
	for _, task := range open.Page.Tasks {
		if task.TaskID == p.Tasks["b"].ID && (task.DependencyReady || len(task.WorkflowRuns) > 0) {
			t.Fatal("successor started or became ready before merge")
		}
	}
	if open.Domain.Executions[0].Workspace.Worktree == "" || open.Domain.Executions[0].SessionID != open.Helper.Session {
		t.Fatal("execution did not bind assigned workspace/session")
	}
	p.Merge()
	closed := p.WaitPhase("completed")
	p.Record("completed", closed)
	var delivery []byte
	for _, task := range closed.Page.Tasks {
		switch task.TaskID {
		case p.Tasks["a"].ID:
			if task.Status != "closed" || task.Delivery == nil || task.Delivery.SelectedWorkflowRunID != open.Domain.WorkflowRuns[0].WorkflowRunID {
				t.Fatal("missing delivery on original workflow")
			}
			delivery, _ = json.Marshal(task.Delivery)
		case p.Tasks["b"].ID:
			if !task.DependencyReady || len(task.WorkflowRuns) > 0 || task.Status != "open" {
				t.Fatal("successor not ready and unstarted")
			}
		}
	}
	if len(delivery) == 0 {
		t.Fatal("missing A delivery")
	}
	// Explicit product queries after delivery must preserve the same delivery fact.
	again, err := p.Snapshot()
	env.Must(err)
	p.Record("rechecked", again)
	for _, task := range again.Page.Tasks {
		if task.TaskID == p.Tasks["a"].ID {
			actual, _ := json.Marshal(task.Delivery)
			if string(actual) != string(delivery) {
				t.Fatal("delivery changed during repeated read")
			}
		}
	}
	if len(again.Domain.WorkflowRuns) != 1 || again.Helper.Dispatches != 1 {
		t.Fatal("duplicate execution after delivery")
	}
	p.StopService()
}
