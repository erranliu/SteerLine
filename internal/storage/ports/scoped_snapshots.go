package ports

import (
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

// TaskCandidateSnapshot combines the complete DAG pre-value with the full
// workflow and selected execution history for one task.
type TaskCandidateSnapshot struct {
	Graph           TaskGraphSnapshot
	ProjectIdentity string
	TaskID          string
	WorkflowRuns    []executionmodel.WorkflowRunRecord
	Executions      []executionmodel.ExecutionRecord
}

// WorkflowSnapshot contains only the persisted facts shown for one workflow.
type WorkflowSnapshot struct {
	ProjectIdentity string
	WorkflowRuns    []executionmodel.WorkflowRunRecord
	Executions      []executionmodel.ExecutionRecord
	Attempts        []executionmodel.StepAttemptRecord
	Decisions       []executionmodel.DecisionRecord
}

// ActivityBinding records only the selected execution and attempt ownership
// facts needed before projecting their bounded event page.
type ActivityBinding struct {
	ProjectIdentity        string
	ExecutionID            string
	ExecutionWorkflowRunID string
	AttemptID              string
	AttemptExecutionID     string
}
