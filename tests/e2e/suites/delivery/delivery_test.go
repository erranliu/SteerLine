//go:build e2e

package delivery

import (
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestBasicDelivery(t *testing.T) {
	s := testenv.NewScenario(t, "delivery.basic")
	p := s.NewProject("delivery")
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	open := p.WaitPhase("waiting_pr_merge")
	p.Record("open", open)
	if open.Helper.Dispatches != 1 || open.Helper.Sessions != 1 {
		t.Fatal("expected one original execution")
	}
	p.Merge()
	done := p.WaitPhase("completed")
	p.Record("completed", done)
	for _, task := range done.Page.Tasks {
		if task.TaskID == p.Tasks["a"].ID && (task.Status != "closed" || task.Delivery == nil || task.Delivery.SelectedWorkflowRunID != open.Domain.WorkflowRuns[0].WorkflowRunID) {
			t.Fatal("delivery missing")
		}
		if task.TaskID == p.Tasks["b"].ID && (!task.DependencyReady || len(task.WorkflowRuns) != 0) {
			t.Fatal("successor not ready and unstarted")
		}
	}
	p.StopService()
}
