package steerlineintegration

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/erranliu/SteerLine/internal/execution"
	api "github.com/erranliu/SteerLine/internal/product/api"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/steerlineapp"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func corruptCandidateOperation(t *testing.T, f appCandidateFixture, operationID string) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(f.f.config.RunDir, "steerline.db"))
	require(t, err)
	defer db.Close()
	result, err := db.ExecContext(f.ctx, `UPDATE operations SET payload=? WHERE project_identity=? AND operation_id=?`, []byte("{"), f.f.config.Identity(), operationID)
	require(t, err)
	changed, err := result.RowsAffected()
	require(t, err)
	if changed != 1 {
		t.Fatalf("expected to corrupt candidate operation %q once; updated %d rows", operationID, changed)
	}
}

func TestTM07ScopedActivityAndWorkflowReaders(t *testing.T) {
	f := newAppCandidateFixture(t)
	a, _ := f.start(t, "")
	b, optionsB := f.start(t, "")
	if a.WorkflowRunID == b.WorkflowRunID || a.ExecutionID == b.ExecutionID {
		t.Fatal("fixture did not create independent candidates")
	}
	corruptCandidateOperation(t, f, optionsB.RequestID)
	reader, err := steerlineapp.OpenNativeTaskReader(f.ctx, &f.f.config)
	require(t, err)
	defer reader.Close()
	query := productquery.NewService(reader, f.f.config.Identity())
	workflow, err := query.Workflow(f.ctx, a.WorkflowRunID)
	require(t, err)
	if len(workflow.WorkflowRuns) != 1 || workflow.WorkflowRuns[0].WorkflowRunID != a.WorkflowRunID || len(workflow.Executions) != 1 || workflow.Executions[0].ExecutionID != a.ExecutionID {
		t.Fatalf("workflow reader escaped the selected workflow identity: %+v", workflow)
	}
	activity, err := query.ReadActivity(f.ctx, a.WorkflowRunID, a.ExecutionID, a.AttemptID, api.ActivityCursor{})
	require(t, err)
	if activity.Cursor.RunID != a.WorkflowRunID || activity.Cursor.ExecutionID != a.ExecutionID || activity.Cursor.AttemptID != a.AttemptID || len(activity.Diagnostics) == 0 {
		t.Fatalf("activity reader failed to return the selected empty event page: %+v", activity)
	}
}

func TestTM07CandidateCommandUsesTaskScopedSnapshot(t *testing.T) {
	f := newAppCandidateFixture(t)
	a, _ := f.start(t, "")
	b, optionsB := f.start(t, "")
	corruptCandidateOperation(t, f, optionsB.RequestID)
	third, _ := f.start(t, "")
	if third.WorkflowRunID == a.WorkflowRunID || third.WorkflowRunID == b.WorkflowRunID {
		t.Fatal("candidate command reused a workflow identity")
	}
	snapshot, err := f.store.ReadTaskCandidateSnapshot(f.ctx, f.f.config.Identity(), "901")
	require(t, err)
	if snapshot.Graph.Authority.ProjectIdentity != f.f.config.Identity() || snapshot.TaskID != "901" || len(snapshot.WorkflowRuns) != 3 || len(snapshot.Executions) != 3 {
		t.Fatalf("candidate command snapshot did not retain the full selected task pre-value: workflows=%d executions=%d task=%s authority=%+v", len(snapshot.WorkflowRuns), len(snapshot.Executions), snapshot.TaskID, snapshot.Graph.Authority)
	}
	for _, workflow := range snapshot.WorkflowRuns {
		if workflow.TaskID != "901" {
			t.Fatalf("task candidate snapshot returned a foreign workflow: %+v", workflow)
		}
	}
	commandStore := reflect.TypeOf((*execution.CommandStore)(nil)).Elem()
	if _, ok := commandStore.MethodByName("Query"); ok {
		t.Fatal("CommandStore still exposes a project-wide execution query")
	}
	method, ok := commandStore.MethodByName("ReadTaskCandidateSnapshot")
	if !ok || method.Type.Out(0).Name() != "TaskCandidateSnapshot" {
		t.Fatal("CommandStore does not expose its task candidate pre-value")
	}
	workflowReader := reflect.TypeOf((*storageports.WorkflowReader)(nil)).Elem()
	activityReader := reflect.TypeOf((*storageports.ActivityReader)(nil)).Elem()
	workflowMethod, workflowOK := workflowReader.MethodByName("ReadWorkflow")
	activityMethod, activityOK := activityReader.MethodByName("ReadActivityView")
	if !workflowOK || workflowMethod.Type.Out(0).Name() != "WorkflowSnapshot" || !activityOK || activityMethod.Type.Out(0).Name() != "ActivityBinding" {
		t.Fatal("product query ports do not expose their identity-scoped values")
	}
}
