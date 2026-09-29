package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// preflightWorkerFlowState is read-only and runs before workspace preparation.
// A v5 projection or workflow may represent a still-running external worker;
// it must not be converted or overwritten on startup.
func preflightWorkerFlowState(ctx context.Context, c Config) error {
	if c.WorkflowProtocol != WorkerFlowVersion {
		return nil
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	reader, err := OpenNativeTaskReader(ctx, &c)
	if err != nil {
		return err
	}
	defer reader.Close()
	payload, _, err := reader.LoadProjection(ctx, c.Identity())
	if err != nil {
		return err
	}
	if len(payload) != 0 {
		var runtime Runtime
		if err := decode(payload, &runtime); err != nil || runtime.SchemaVersion != WorkerFlowRuntimeSchema {
			return nativeErr("unsupported_schema", "worker-flow-v1 refuses the old runtime projection; preserve the original state and external session")
		}
	}
	snapshot, err := reader.Query(ctx, c.Identity())
	if err != nil {
		return err
	}
	for _, workflow := range snapshot.WorkflowRuns {
		if workflow.WorkflowVersion != WorkerFlowVersion {
			return nativeErr("unsupported_schema", "worker-flow-v1 refuses an old WorkflowRun; preserve the original state and external session")
		}
	}
	return nil
}

// persistWorkerFlow writes the authoritative v1 worker records. The Run is a
// compatibility projection for clients; it cannot supply a second phase or
// replace the original Execution/session binding.
func persistWorkerFlow(tx DomainTx, project string, runtime Runtime, run *Run) error {
	if run == nil || run.WorkflowVersion != WorkerFlowVersion {
		return errors.New("worker flow runtime has no matching run")
	}
	var task *TaskRecord
	for i := range runtime.Tasks {
		if runtime.Tasks[i].TaskID == run.DomainTaskID {
			task = &runtime.Tasks[i]
			break
		}
	}
	var workflow *WorkflowRunRecord
	for i := range runtime.WorkflowRuns {
		if runtime.WorkflowRuns[i].WorkflowRunID == run.RunID {
			workflow = &runtime.WorkflowRuns[i]
			break
		}
	}
	var execution *ExecutionRecord
	for i := range runtime.Executions {
		if runtime.Executions[i].ExecutionID == run.ExecutionID {
			execution = &runtime.Executions[i]
			break
		}
	}
	if task == nil || workflow == nil || execution == nil || task.TaskID != workflow.TaskID ||
		workflow.WorkflowVersion != WorkerFlowVersion || workflow.WorkflowRunID != execution.WorkflowRunID ||
		workflow.SelectedResult != execution.ExecutionID || run.TaskID != task.TaskID || run.DomainTaskID != task.TaskID ||
		run.SessionID != nil && execution.SessionID != "" && *run.SessionID != execution.SessionID {
		return errors.New("worker flow projection differs from fixed task, workflow, execution or session")
	}
	finish := map[string]string{"queued": "pending", "executing": "pending", "waiting_decision": "waiting_decision", "waiting_pr_merge": "waiting_pr_merge", "completed": "completed"}[workflow.Phase]
	if finish == "" || run.FinishTaskStatus != finish || workflow.Phase == "waiting_pr_merge" && workflow.PRURL == "" ||
		workflow.Phase == "waiting_decision" && run.Decision == nil || task.Status == "closed" && workflow.Phase != "completed" {
		return fmt.Errorf("worker flow projection disagrees with phase %q", workflow.Phase)
	}
	if task.Status == "closed" {
		// TaskDelivery already atomically persisted the completed workflow.
		return nil
	}
	if err := tx.PutWorkflowRun(*workflow); err != nil {
		return err
	}
	if err := tx.PutExecution(*execution); err != nil {
		return err
	}
	attempts := 0
	for _, attempt := range runtime.Attempts {
		if attempt.ExecutionID == execution.ExecutionID {
			attempts++
			if attempt.StepID != "worker" || attempt.AttemptID != run.AttemptID {
				return errors.New("worker flow has a second step attempt")
			}
			if err := tx.PutStepAttempt(attempt); err != nil {
				return err
			}
		}
	}
	if attempts != 1 {
		return errors.New("worker flow must retain exactly one stable worker attempt identity")
	}
	if run.Decision != nil {
		request, err := json.Marshal(run.Decision.Request)
		if err != nil {
			return err
		}
		var answer []byte
		if run.Decision.Status != "pending" {
			value := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: run.Decision.OptionID, Note: run.Decision.Note, AnsweredBy: run.Decision.AnsweredBy}
			if run.Decision.AnsweredAt != nil {
				value.AnsweredAt = *run.Decision.AnsweredAt
			}
			answer, err = json.Marshal(value)
			if err != nil {
				return err
			}
		}
		created := now()
		for _, old := range runtime.Decisions {
			if old.DecisionID == run.Decision.ID {
				created = old.CreatedAt
				break
			}
		}
		if err := tx.PutDecision(DecisionRecord{ProjectIdentity: project, DecisionID: run.Decision.ID, Scope: "execution", WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, QuestionID: run.Decision.ID, Request: request, Answer: answer, AnsweredBy: run.Decision.AnsweredBy, Status: run.Decision.Status, Version: run.Decision.Version, CreatedAt: created, UpdatedAt: now()}); err != nil {
			return err
		}
	}
	return nil
}
