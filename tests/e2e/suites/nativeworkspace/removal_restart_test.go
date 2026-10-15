//go:build e2e

package nativeworkspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"github.com/google/uuid"
)

func TestOrcaRemovalRestart(t *testing.T) {
	s := testenv.NewScenario(t, "workspace.orca-removal-restart")
	p := s.NewProject("workspace")
	p.UseNativeWorkspaceLayout()
	p.EnableNativeWorkspaceTools()
	p.CreateTasksWithIndependentTask()
	p.UseNativeDefaultWorktreeRoot()
	created := p.NativeCreateWorktree(strings.ReplaceAll(uuid.NewString(), "-", ""))
	root := filepath.Join(filepath.Dir(p.Database), "worktrees")
	if !strings.EqualFold(filepath.Clean(filepath.Dir(filepath.FromSlash(created.Path))), filepath.Clean(root)) {
		t.Fatalf("native workspace layout=%s expected parent=%s", created.Path, root)
	}
	p.StartService()
	p.Claim("a")
	before := p.WaitPhase("waiting_pr_merge")
	p.Record("before-removal", before)
	worker := before.Domain.Executions[0]
	if !strings.EqualFold(filepath.Clean(filepath.Dir(worker.Workspace.Worktree)), filepath.Clean(root)) || !strings.EqualFold(filepath.Clean(filepath.Dir(filepath.FromSlash(created.Path))), filepath.Clean(root)) {
		t.Fatal("scenario did not use the installed default worktree layout")
	}
	// Delete during the business phase, with the same project/service alive.
	// The controlled worker's workspace is independent of the native checkout.
	p.NativeRemoveWorktree(created)
	query := func(label string) testenv.Snapshot {
		var raw []byte
		s.Wait("public query "+label, func() (bool, error) {
			var queryErr error
			raw, queryErr = p.Driver.Run(s.Context, "task", "list", "-config", p.Config, "-limit", "100")
			return queryErr == nil, queryErr
		})
		var identity struct {
			ProjectID  string `json:"project_id"`
			DatabaseID string `json:"database_id"`
		}
		s.Must(json.Unmarshal(raw, &identity))
		if identity.ProjectID != before.Page.ProjectID || identity.DatabaseID != before.Page.DatabaseID {
			t.Fatal("native cleanup changed authoritative project/database identity")
		}
		var public api.ProjectSnapshot
		s.Wait("HTTP query "+label, func() (bool, error) {
			var err error
			public, err = driver.ReadHTTPProject(s.Context, filepath.Dir(p.Database), before.Domain.ProjectIdentity, "")
			if err != nil {
				return false, err
			}
			return public.Connection.Snapshot.Page != nil && public.Connection.Snapshot.Page.Service.Ready, nil
		})
		if public.NativeTasks == nil || public.NativeTasks.ProjectID != before.Page.ProjectID || public.NativeTasks.DatabaseID != before.Page.DatabaseID || public.Connection.Snapshot.Page == nil || public.Connection.Snapshot.Page.Service.InstanceID == "" {
			t.Fatal("HTTP query lost project/database/service identity")
		}
		original := before.Runtime.Runs[0]
		found := false
		for _, run := range public.Connection.Snapshot.Page.Runs {
			if run.RunID == original.RunID && run.ExecutionID == original.ExecutionID && run.AttemptID == original.AttemptID {
				found = true
			}
		}
		if !found {
			t.Fatal("HTTP query lost original execution after native cleanup/restart")
		}
		snapshot, err := p.Snapshot()
		s.Must(err)
		if snapshot.Runtime == nil || public.Connection.Snapshot.Page.Service.InstanceID != snapshot.Runtime.Service.InstanceID {
			t.Fatal("HTTP query does not belong to the current project service instance")
		}
		p.Record(label, snapshot)
		ready := false
		for _, task := range snapshot.Page.Tasks {
			if task.TaskID == p.Tasks["c"].ID {
				ready = task.Status == "open" && task.DependencyReady && len(task.WorkflowRuns) == 0
			}
		}
		if !ready {
			t.Fatal("native removal blocked or dispatched an unrelated ready task")
		}
		if len(snapshot.Domain.WorkflowRuns) != 1 || snapshot.Domain.WorkflowRuns[0].WorkflowRunID != worker.WorkflowRunID {
			t.Fatal("native removal replaced or duplicated task execution")
		}
		return snapshot
	}
	query("after-removal")
	p.RestartService()
	query("after-restart")
	s.Must(p.Driver.Control(s.Context, s.Run.Helper, "start_task", p.Tasks["c"].ID))
	s.Wait("unrelated task starts after native cleanup", func() (bool, error) {
		snapshot, err := p.Snapshot()
		if err != nil {
			return false, err
		}
		for _, execution := range snapshot.Domain.Executions {
			if execution.WorkflowRunID == worker.WorkflowRunID {
				continue
			}
			if execution.Workspace.Worktree == "" || execution.Status == "active" || execution.Status == "blocked" || execution.Status == "failed" {
				continue
			}
			for _, workflow := range snapshot.Domain.WorkflowRuns {
				if workflow.WorkflowRunID == execution.WorkflowRunID && workflow.TaskID == p.Tasks["c"].ID && workflow.Status == "waiting_external" {
					_, err := os.Stat(execution.Workspace.Worktree)
					if err == nil {
						p.Record("unrelated-task-started", snapshot)
						return true, nil
					}
				}
			}
		}
		return false, nil
	})
	p.StopService()
	s.VerifyNativeWorkspaceLifecycle()
}
