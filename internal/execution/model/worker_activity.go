package model

// WorkerStateFields uses the same field names and projection as the public
// worker snapshot. It records facts, not a second state machine.
type WorkerStateFields struct {
	WorkflowStage     string `json:"workflow_stage"`
	WorkflowStatus    string `json:"workflow_status"`
	ExecutionStatus   string `json:"execution_status"`
	ResourceStatus    string `json:"resource_status"`
	CurrentTurnID     string `json:"current_turn_id"`
	InputVersion      uint64 `json:"input_version"`
	RecoveryRequired  bool   `json:"recovery_required"`
	RecoveryState     string `json:"recovery_state"`
	EnvironmentReason string `json:"environment_reason"`
}
