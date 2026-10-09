package reconstruction

import (
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func LatestWorkflow(workflows []executionmodel.WorkflowRunRecord, taskID string) *executionmodel.WorkflowRunRecord {
	var latest *executionmodel.WorkflowRunRecord
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

func LatestExecution(executions []executionmodel.ExecutionRecord, workflowRunID string) *executionmodel.ExecutionRecord {
	var latest *executionmodel.ExecutionRecord
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

func CurrentAttempt(attempts []executionmodel.StepAttemptRecord, execution *executionmodel.ExecutionRecord) *executionmodel.StepAttemptRecord {
	if execution == nil || execution.CurrentStep == "" {
		return nil
	}
	var latest *executionmodel.StepAttemptRecord
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

func finishStatus(task taskmodel.TaskRecord, workflow executionmodel.WorkflowRunRecord) string {
	switch {
	case workflow.Abandonment != nil:
		return "abandoned"
	case task.Status == "closed" && workflow.Status == "blocked":
		return "blocked"
	case workflow.Status == "completed":
		return "completed"
	case task.Status == "closed":
		return "not_selected"
	case workflow.Status == "waiting_external":
		return "waiting_pr_merge"
	case workflow.Status == "waiting_input":
		return "waiting_decision"
	case workflow.Status == "blocked" || task.Status == "discarded":
		return "blocked"
	default:
		return "pending"
	}
}
