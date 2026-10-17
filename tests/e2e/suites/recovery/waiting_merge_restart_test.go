//go:build e2e

package recovery

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
)

func TestWaitingMergeRestart(t *testing.T) {
	t.Parallel()
	env := testenv.NewScenario(t, "recovery.waiting-merge-restart")
	p := env.NewProject("recovery")
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	before := p.WaitPhase("waiting_pr_merge")
	p.AssertRunnerBinding(before)
	p.Record("before-restart", before)
	if before.Helper.PR.State != "open" {
		t.Fatal("business turn was not completed with open PR")
	}
	requests, _ := json.Marshal(p.CallerRequests())
	p.RestartService()
	after := p.WaitPhase("waiting_pr_merge")
	p.AssertRunnerBinding(after)
	p.Record("after-restart", after)
	first, second := before.Runner.Candidates[0], after.Runner.Candidates[0]
	if first.Projection == nil || second.Projection == nil {
		t.Fatal("missing public Run projection")
	}
	if before.Page.ProjectID != after.Page.ProjectID || before.Page.DatabaseID != after.Page.DatabaseID || first.ID != second.ID || first.RunID != second.RunID || first.Submit.RequestID != second.Submit.RequestID {
		t.Fatal("restart replaced original caller/Run/Attempt/session/workspace")
	}

	afterRequests, _ := json.Marshal(p.CallerRequests())
	if string(requests) != string(afterRequests) {
		t.Fatal("SteerLine restart submitted new business input")
	}
	for _, task := range after.Page.Tasks {
		if task.TaskID == p.Tasks["b"].ID && (task.DependencyReady || len(task.Candidates) > 0) {
			t.Fatal("restart released or started blocked successor")
		}
	}
	p.Merge()
	closed := p.WaitPhase("completed")
	p.Record("completed", closed)
	var delivery []byte
	for _, task := range closed.Page.Tasks {
		switch task.TaskID {
		case p.Tasks["a"].ID:
			if task.Status != "closed" || task.Delivery == nil || task.Delivery.SelectedWorkflowRunID != before.Runner.Candidates[0].ID {
				t.Fatal("restart lost original workflow delivery")
			}
			delivery, _ = json.Marshal(task.Delivery)
		case p.Tasks["b"].ID:
			if !task.DependencyReady || len(task.Candidates) > 0 || task.Status != "open" {
				t.Fatal("successor not ready and unstarted")
			}
		}
	}
	if len(delivery) == 0 {
		t.Fatal("missing delivery")
	}
	again, err := p.Snapshot()
	env.Must(err)
	p.Record("rechecked", again)
	for _, task := range again.Page.Tasks {
		if task.TaskID == p.Tasks["a"].ID {
			actual, _ := json.Marshal(task.Delivery)
			if string(actual) != string(delivery) {
				t.Fatal("delivery changed during reconciliation")
			}
		}
	}
	if len(again.Runner.Candidates) != 1 {
		t.Fatal("repeated reconciliation duplicated execution")
	}
	p.WaitStoppedCandidates()
	p.StopService()
}
