package projection

import (
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func LatestWorkflow(workflows []contracts.WorkflowRunRecord, taskID string) *contracts.WorkflowRunRecord {
	var latest *contracts.WorkflowRunRecord
	for i := range workflows {
		workflow := &workflows[i]
		if workflow.TaskID != taskID {
			continue
		}
		if latest == nil || workflow.CreatedAt.After(latest.CreatedAt) || workflow.CreatedAt.Equal(latest.CreatedAt) && workflow.WorkflowRunID > latest.WorkflowRunID {
			latest = workflow
		}
	}
	return latest
}

func LatestExecution(executions []contracts.ExecutionRecord, workflowRunID string) *contracts.ExecutionRecord {
	var latest *contracts.ExecutionRecord
	for i := range executions {
		execution := &executions[i]
		if execution.WorkflowRunID != workflowRunID {
			continue
		}
		if latest == nil || execution.CreatedAt.After(latest.CreatedAt) || execution.CreatedAt.Equal(latest.CreatedAt) && execution.ExecutionID > latest.ExecutionID {
			latest = execution
		}
	}
	return latest
}

func CurrentAttempt(attempts []contracts.StepAttemptRecord, execution *contracts.ExecutionRecord) *contracts.StepAttemptRecord {
	if execution == nil || execution.CurrentStep == "" {
		return nil
	}
	var latest *contracts.StepAttemptRecord
	for i := range attempts {
		attempt := &attempts[i]
		if attempt.ExecutionID != execution.ExecutionID || attempt.StepID != execution.CurrentStep {
			continue
		}
		if latest == nil || attempt.AttemptNumber > latest.AttemptNumber {
			latest = attempt
		}
	}
	return latest
}

func finishStatus(task contracts.TaskRecord, workflow contracts.WorkflowRunRecord) string {
	switch {
	case task.Status == "closed" && workflow.Phase == "blocked":
		return "blocked"
	case task.Status == "closed" || workflow.Phase == "completed":
		return "completed"
	case workflow.Phase == "waiting_pr_merge":
		return "waiting_pr_merge"
	case workflow.Phase == "waiting_decision":
		return "waiting_decision"
	case workflow.Phase == "blocked" || task.Status == "discarded":
		return "blocked"
	default:
		return "pending"
	}
}

func HostForExecution(backend, protocol string) string {
	switch {
	case backend == string(executionapi.BackendLocal) && (protocol == string(executionapi.ProtocolLocalProcess) || protocol == string(executionapi.ProtocolLocalCodexAppServer)):
		return "local"
	case backend == string(executionapi.BackendOrca) && protocol == string(executionapi.ProtocolOrcaTerminalV2):
		return "orca"
	case backend == string(executionapi.BackendOrca) && protocol == string(executionapi.ProtocolOrcaSupervised):
		return "orca_supervised"
	default:
		return ""
	}
}
