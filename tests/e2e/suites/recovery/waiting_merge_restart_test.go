//go:build e2e

package recovery

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"testing"
)

func TestWaitingMergeRestart(t *testing.T) {
	env := testenv.NewScenario(t, "recovery.waiting-merge-restart")
	p := env.NewProject("recovery")
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	before := p.WaitPhase("waiting_pr_merge")
	p.AssertFullAccess(before)
	p.Record("before-restart", before)
	if before.Helper.Sessions != 1 || before.Helper.Dispatches != 1 || before.Helper.PR.State != "open" {
		t.Fatal("business turn was not completed with open PR")
	}
	p.RestartService()
	after := p.WaitPhase("waiting_pr_merge")
	p.AssertFullAccess(after)
	p.Record("after-restart", after)
	first, second := before.Domain.Executions[0], after.Domain.Executions[0]
	if before.Page.ProjectID != after.Page.ProjectID || before.Page.DatabaseID != after.Page.DatabaseID || before.Domain.WorkflowRuns[0].WorkflowRunID != after.Domain.WorkflowRuns[0].WorkflowRunID || first.ExecutionID != second.ExecutionID || first.SessionID != second.SessionID || first.Workspace.Worktree != second.Workspace.Worktree || first.Workspace.Branch != second.Workspace.Branch || len(before.Domain.Attempts) != len(after.Domain.Attempts) {
		t.Fatal("restart replaced original task/database/execution/workspace identity")
	}
	if after.Helper.Sessions != before.Helper.Sessions || after.Helper.Dispatches != before.Helper.Dispatches {
		t.Fatal("restart duplicated native session/dispatch")
	}
	for _, task := range after.Page.Tasks {
		if task.TaskID == p.Tasks["b"].ID && (task.DependencyReady || len(task.WorkflowRuns) > 0) {
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
			if task.Status != "closed" || task.Delivery == nil || task.Delivery.SelectedWorkflowRunID != before.Domain.WorkflowRuns[0].WorkflowRunID {
				t.Fatal("restart lost original workflow delivery")
			}
			delivery, _ = json.Marshal(task.Delivery)
		case p.Tasks["b"].ID:
			if !task.DependencyReady || len(task.WorkflowRuns) > 0 || task.Status != "open" {
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
	if len(again.Domain.WorkflowRuns) != 1 || again.Helper.Sessions != 1 || again.Helper.Dispatches != 1 {
		t.Fatal("repeated reconciliation duplicated execution")
	}
	p.StopService()
}
