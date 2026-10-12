package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestTaskDeliveryCommitPRFlow(t *testing.T) {
	for _, change := range []string{"observations", "worktree", "repository", "branch", "base", "generation"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			store, project, tasks := nativeAuditStore(t)
			task := tasks[0]
			task.Status, task.Claimant, task.Version = "in_progress", "operator", task.Version+1
			at := time.Now().UTC()
			workflow := executionmodel.WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "pr-flow", TaskID: task.TaskID, WorkflowVersion: executionmodel.Version, FlowRef: executionmodel.DevelopmentFlow, StepID: executionmodel.StepAwaitMerge, Status: "waiting_external", SelectedResult: "execution", PRURL: "https://example.invalid/Org/Repo/pull/2", CreatedAt: at, UpdatedAt: at}
			execution := executionmodel.ExecutionRecord{ProjectIdentity: project, ExecutionID: workflow.SelectedResult, WorkflowRunID: workflow.WorkflowRunID, Status: "idle", SessionID: "original-session", Generation: "original-generation", Workspace: executionmodel.WorkspaceEvidence{Repository: "repository", Worktree: "worktree", Branch: "task-2", Base: strings.Repeat("a", 40), Head: strings.Repeat("b", 40)}, CreatedAt: at, UpdatedAt: at}
			bindingData, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{ImplementationKey: "storage-fixture", CodecVersion: 1, Purpose: executionbackend.ImplementationResourcePurpose, ProjectID: project, ExecutionID: execution.ExecutionID, AttemptID: "attempt", Generation: execution.Generation, ConfigDigest: "fixture-config"}, []byte(`{"session":"original-session"}`))
			require(t, err)
			execution.ImplementationData = []executionbackend.ImplementationDataEnvelope{bindingData}
			require(t, store.transactSQLite(ctx, func(tx *sqliteTx) error {
				if err := tx.UpdateNativeTaskCAS(task, tasks[0].Version); err != nil {
					return err
				}
				if err := tx.PutWorkflowRun(workflow); err != nil {
					return err
				}
				return tx.PutExecution(execution)
			}))
			otherPR := workflow
			otherPR.PRURL += "-other"
			otherPR.PRHistory = []string{workflow.PRURL}
			requireNativeAuditCode(t, store.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.PutWorkflowRun(otherPR) }), "immutable_task")
			graph, err := store.TaskGraphSnapshot(ctx, project)
			require(t, err)
			contract, err := store.TaskContract(ctx, project, task)
			require(t, err)
			binding := executionmodel.CandidateBindings([]executionmodel.WorkflowRunRecord{workflow}, []executionmodel.ExecutionRecord{execution}, task.TaskID)
			input := taskmodel.Result{ProjectIdentity: project, TaskID: task.TaskID, ContractID: contract.ContractID, ContractDigest: contract.Digest, Claimant: task.Claimant, ExpectedTaskVersion: task.Version, ExpectedGraphRevision: graph.Authority.GraphRevision, ExpectedSetVersion: binding, ResultID: workflow.WorkflowRunID, RequestID: "delivery:" + workflow.WorkflowRunID, Conclusion: "delivered"}
			declaration, err := taskmodel.DeclareResult(task, contract, input, at)
			require(t, err)
			proof := executionmodel.TaskDeliveryEvidence{PRInstance: "https://example.invalid", URL: workflow.PRURL, Repository: "Org/Repo", PRNumber: 2, BaseBranch: "main", State: "merged", Merged: true, MergedAt: at, MergeCommit: strings.Repeat("c", 40)}
			delivery := executionmodel.TaskDeliveryRecord{ProjectIdentity: project, TaskID: task.TaskID, SelectedWorkflowRunID: workflow.WorkflowRunID, PRInstance: proof.PRInstance, PRRepository: proof.Repository, PRNumber: proof.PRNumber, PRURL: proof.URL, MergeCommit: proof.MergeCommit, MergedAt: at, RecordedAt: at, Evidence: mustJSON(proof)}
			commit := storageports.ResultCommit{Task: declaration, AuthorityVersion: graph.Authority.AuthorityVersion, Workflow: workflow, Execution: execution, Delivery: delivery}

			// Simulate a write after provider verification, before the result transaction.
			current := execution
			switch change {
			case "observations":
				current.Workspace.Head, current.Workspace.Dirty, current.Workspace.VerifiedAt = strings.Repeat("d", 40), true, at.Add(time.Minute)
			case "worktree":
				current.Workspace.Worktree += "-other"
			case "repository":
				current.Workspace.Repository += "-other"
			case "branch":
				current.Workspace.Branch += "-other"
			case "base":
				current.Workspace.Base = strings.Repeat("e", 40)
			case "generation":
				current.Generation += "-other"
			}
			require(t, store.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.PutExecution(current) }))
			_, err = store.CommitTaskResult(ctx, commit)
			if change != "observations" {
				requireNativeAuditCode(t, err, "immutable_task")
				var count int
				require(t, store.db.QueryRow(`SELECT COUNT(*) FROM task_deliveries`).Scan(&count))
				if count != 0 {
					t.Fatal("changed identity published a delivery")
				}
				return
			}
			require(t, err)
			_, err = store.CommitTaskResult(ctx, commit)
			require(t, err)
			var count int
			require(t, store.db.QueryRow(`SELECT COUNT(*) FROM task_deliveries`).Scan(&count))
			if count != 1 {
				t.Fatalf("delivery replay wrote %d deliveries", count)
			}
		})
	}
}
