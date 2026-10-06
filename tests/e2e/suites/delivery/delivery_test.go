//go:build e2e

package delivery

import (
	"fmt"
	"testing"

	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
)

func TestBasicDelivery(t *testing.T) {
	s := testenv.NewScenario(t, "delivery.basic")
	p := s.NewProject("delivery")
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	open := p.WaitPhase("waiting_pr_merge")
	p.AssertFullAccess(open)
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
	if len(done.Domain.Deliveries) != 1 || done.Domain.Deliveries[0].SelectedWorkflowRunID != open.Domain.WorkflowRuns[0].WorkflowRunID || done.Domain.Deliveries[0].MergeCommit == "" {
		t.Fatalf("delivery is not one verified result from the original workflow: %+v", done.Domain.Deliveries)
	}
	deliveryMerge := done.Domain.Deliveries[0].MergeCommit

	// The snapshot above proves B became ready and stayed unstarted after A's
	// unique Delivery. Starting B then proves its real execution base is A's
	// recorded merge commit, not a separately sampled or reconstructed guess.
	// Dispatch is already active after A's original claim; do not submit a
	// second resume_dispatch request while starting the newly ready task.
	s.Must(p.Driver.Control(s.Context, s.Run.Helper, "start_task", p.Tasks["b"].ID))
	var successor testenv.Snapshot
	s.Wait("successor base follows delivered merge", func() (bool, error) {
		var err error
		successor, err = p.Snapshot()
		if err != nil {
			return false, err
		}
		for _, workflow := range successor.Domain.WorkflowRuns {
			if workflow.TaskID != p.Tasks["b"].ID {
				continue
			}
			for _, execution := range successor.Domain.Executions {
				if execution.WorkflowRunID != workflow.WorkflowRunID {
					continue
				}
				if execution.Workspace.Base == "" {
					return false, nil
				}
				if execution.Workspace.Base != deliveryMerge {
					return false, fmt.Errorf("successor base %s differs from delivery merge %s", execution.Workspace.Base, deliveryMerge)
				}
				return true, nil
			}
		}
		return false, nil
	})
	if len(successor.Domain.Deliveries) != 1 || successor.Domain.Deliveries[0].MergeCommit != deliveryMerge {
		t.Fatalf("starting the ready successor changed the original Delivery: %+v", successor.Domain.Deliveries)
	}
	p.Record("successor-base", successor)
	p.StopService()
}
