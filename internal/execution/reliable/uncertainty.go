package reliable

import (
	"encoding/json"
	"strings"
)

type OperationFact struct {
	Kind, Status, IdempotencyKey string
	Request                      []byte
}

// Operations are bound by their saved request and exact identity components,
// including host publication which is keyed by WorkflowRun rather than Execution.
func HasUnknownOperation(operations []OperationFact, executionID, associationID string) bool {
	for _, op := range operations {
		if op.Kind == "worker_input_value" {
			continue
		} // This journal has no external outcome to reconcile.
		if op.Status != "outcome_unknown" && op.Status != "in_flight" && !(op.Kind == "stop_worker_observation" && op.Status == "accepted") {
			continue
		}
		var target struct {
			ExecutionID   string `json:"execution_id"`
			WorkflowRunID string `json:"workflow_run_id"`
			RunID         string `json:"run_id"`
		}
		if json.Unmarshal(op.Request, &target) == nil && (target.ExecutionID == executionID || target.WorkflowRunID == associationID || target.RunID == associationID) {
			return true
		}
		for _, identity := range strings.Split(op.IdempotencyKey, ":") {
			if identity == executionID || identity == associationID {
				return true
			}
		}
	}
	return false
}
