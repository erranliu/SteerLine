package contracts

import (
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type DomainSnapshot struct {
	Deliveries      []TaskDeliveryRecord               `json:"deliveries"`
	ProjectIdentity string                             `json:"project_identity"`
	Tasks           []taskmodel.TaskRecord             `json:"tasks"`
	WorkflowRuns    []executionmodel.WorkflowRunRecord `json:"workflow_runs"`
	Executions      []executionmodel.ExecutionRecord   `json:"executions"`
	Attempts        []executionmodel.StepAttemptRecord `json:"attempts"`
	Decisions       []executionmodel.DecisionRecord    `json:"decisions"`
	Operations      []executionmodel.OperationRecord   `json:"operations"`
}
