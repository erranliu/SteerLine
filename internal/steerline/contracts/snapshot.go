package contracts

type DomainSnapshot struct {
	Deliveries      []TaskDeliveryRecord `json:"deliveries"`
	ProjectIdentity string               `json:"project_identity"`
	Tasks           []TaskRecord         `json:"tasks"`
	WorkflowRuns    []WorkflowRunRecord  `json:"workflow_runs"`
	Executions      []ExecutionRecord    `json:"executions"`
	Attempts        []StepAttemptRecord  `json:"attempts"`
	Decisions       []DecisionRecord     `json:"decisions"`
	Operations      []OperationRecord    `json:"operations"`
}
