package host

import (
	"context"
	"encoding/json"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	execution "github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func taskBatchFixture(t *testing.T, c execution.Config, commit string, tasks []taskmodel.TaskBatchTask, edges []taskmodel.TaskBatchEdge) []byte {
	t.Helper()
	if edges == nil {
		edges = []taskmodel.TaskBatchEdge{}
	}
	for i := range tasks {
		if tasks[i].Contract.ContractID == "" {
			tasks[i].Contract = nativeContract(t, c, commit, "batch-contract-"+strconvID(tasks[i].TaskID))
		}
	}
	b, err := json.Marshal(taskmodel.TaskBatchInput{SchemaVersion: taskmodel.BatchSchemaVersion, ProjectID: c.Identity(), Tasks: tasks, Dependencies: edges})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func strconvID(v int64) string { return strconv.FormatInt(v, 10) }

func openDAGFixture(t *testing.T) (execution.Config, string, *sqliteFixture) {
	t.Helper()
	c, commit := nativeTaskFixture(t)
	reader, err := openHostFixtureNativeProject(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := openSQLiteFixture(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return c, commit, store
}

func batchTasks(ids ...int64) []taskmodel.TaskBatchTask {
	out := make([]taskmodel.TaskBatchTask, len(ids))
	for i, id := range ids {
		priority := 2
		out[i] = taskmodel.TaskBatchTask{LocalKey: "k" + strconvID(id), TaskID: id, Title: "Task " + strconvID(id), Type: "task", Priority: &priority}
	}
	return out
}

func hasTaskIssue(issues []taskmodel.TaskBatchIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type fixedTaskDeliveryVerifier struct {
	info    executionmodel.PRInfo
	calls   int
	err     error
	onQuery func()
}

func (v *fixedTaskDeliveryVerifier) Query(ctx context.Context, url string) (executionmodel.PRInfo, error) {
	v.calls++
	if url != v.info.URL {
		return executionmodel.PRInfo{}, publication.ErrPRIdentity
	}
	if v.onQuery != nil {
		v.onQuery()
	}
	return v.info, v.err
}

func seedPublishedForDiscard(t *testing.T, ctx context.Context, c execution.Config, store *sqliteFixture, taskID, commit, prURL string) {
	t.Helper()
	if c.PR.Kind == "gitea" {
		settings := neutralProfileSettings(c)
		c.PublicationPolicy.Publisher = "worker"
		setNeutralProfile(t, &c, settings)
	}
	dependencies := hostFixtureRuntime(c)
	require(t, c.BindPublicationTarget(ctx, dependencies.ProcessHost))
	profile := hostFixtureCatalogFor(c).AvailableExecutionProfiles()[0]
	fixed := profile.Freeze(dependencies.Directory, c.Identity())
	if !neutralVerifyFixedExecutionProfile(fixed) {
		t.Fatal("published discard fixture has no valid frozen profile")
	}
	page, err := (productquery.NewService(store, c.Identity())).ShowNativeTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	task := page.Tasks[0].TaskRecord
	created := time.Now().UTC()
	runID := "wf-" + taskID
	executionID := "exec-" + taskID
	workspace := executionmodel.WorkspaceEvidence{Repository: c.Repository, Worktree: "tree-" + taskID, Branch: "fixture/" + taskID, Base: commit, Head: commit, VerifiedAt: created}
	run := executionmodel.WorkflowRunRecord{ProjectIdentity: c.Identity(), WorkflowRunID: runID, TaskID: taskID, WorkflowVersion: executionmodel.Version, Status: "waiting_external", SelectedResult: executionID, PRURL: prURL, PRState: "open", CreatedAt: created, UpdatedAt: created}
	run.ExecutionProfile, run.ConfigDigest, run.Repository, run.Branch = &fixed, fixed.AcceptedConfigVersion, c.Repository, c.Branch
	execution := executionmodel.ExecutionRecord{ProjectIdentity: c.Identity(), ExecutionID: executionID, CurrentAttemptID: "attempt-" + taskID, WorkflowRunID: runID, Backend: string(executionbackend.BackendLocal), Protocol: string(neutralProtocol), Agent: "fixture", Status: "idle", CurrentStep: "worker", Workspace: workspace, CreatedAt: created, UpdatedAt: created}
	execution.ConfigurationDigest = fixed.Digest
	setNeutralBinding(t, &execution, "fixture-endpoint", "fixture-generation", workerFlowMockSession)
	execution.Process = &executionbackend.ProcessIdentity{Kind: "fixture", RuntimeID: "fixture-runtime", Incarnation: "original"}

	task.Status = "in_progress"
	task.Claimant = "worker"
	task.Version++
	task.UpdatedAt = created
	if err = store.transactRecords(ctx, func(domainTx *fixtureTransaction) error {
		tx := domainTx
		if e := tx.PutWorkflowRun(run); e != nil {
			return e
		}
		if e := tx.PutExecution(execution); e != nil {
			return e
		}
		return tx.UpdateNativeTaskCAS(task, 1)
	}); err != nil {
		t.Fatal(err)
	}
}
