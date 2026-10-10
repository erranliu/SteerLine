package model

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// WorkerStateFields uses the same field names and projection as the public
// worker snapshot. It records facts, not a second state machine.
type WorkerStateFields struct {
	StepID            string `json:"step_id"`
	Status            string `json:"status"`
	ExecutionStatus   string `json:"execution_status"`
	ResourceStatus    string `json:"resource_status"`
	CurrentTurnID     string `json:"current_turn_id"`
	InputVersion      uint64 `json:"input_version"`
	RecoveryRequired  bool   `json:"recovery_required"`
	RecoveryState     string `json:"recovery_state"`
	EnvironmentReason string `json:"environment_reason"`
}

func ExecutionViewStatus(e ExecutionRecord) string {
	switch e.Status {
	case "active":
		return "running"
	case "pending":
		return "pending"
	case "idle", "succeeded", "stopped":
		if e.LastTurnOutcome != "" {
			return e.LastTurnOutcome
		}
		return "succeeded"
	case "failed":
		return "failed"
	default:
		return "unknown"
	}
}

func ExecutionResourceStatus(e ExecutionRecord) string {
	if e.ResourceStoppedAt != nil && e.ResourceStopOperationID != "" {
		return "stopped"
	}
	return e.Status
}

func workerStateFields(w *WorkflowRunRecord, e *ExecutionRecord) WorkerStateFields {
	if w == nil || e == nil {
		return WorkerStateFields{}
	}
	v := WorkerStateFields{StepID: w.StepID, Status: w.Status, ExecutionStatus: ExecutionViewStatus(*e), ResourceStatus: ExecutionResourceStatus(*e), CurrentTurnID: e.CurrentTurnID, InputVersion: w.InputVersion, RecoveryRequired: e.RecoveryRequired, RecoveryState: e.RecoveryState}
	if w.StepID == StepDelivered && w.Status == "completed" {
		v.ExecutionStatus = "succeeded"
	}
	if e.Environment != nil && e.Environment.Blocker != nil {
		v.EnvironmentReason = e.Environment.Blocker.Fact.Reason
	}
	return v
}

func WorkerStateActivity(beforeW *WorkflowRunRecord, beforeE *ExecutionRecord, w WorkflowRunRecord, e ExecutionRecord, at time.Time) (ActivityEvent, bool) {
	from, to := workerStateFields(beforeW, beforeE), workerStateFields(&w, &e)
	if from == to {
		return ActivityEvent{}, false
	}
	version := uint64(0)
	if beforeW != nil {
		version = beforeW.ControlRevision
	}
	raw, _ := json.Marshal(struct {
		From, To WorkerStateFields
		Version  uint64
	}{from, to, version})
	digest := sha256.Sum256(raw)
	ev := ActivityEvent{V: 1, TS: at, Source: "steer", Kind: "worker_state", RunID: w.WorkflowRunID, TaskID: w.TaskID,
		ItemID: fmt.Sprintf("worker-state:%x", digest[:16]), Status: to.Status,
		Title:  "step_id/status: " + from.StepID + " / " + from.Status + " → " + to.StepID + " / " + to.Status,
		Detail: w.StageDiagnostic, Meta: map[string]any{"from": from, "to": to, "execution_id": e.ExecutionID, "attempt_id": e.CurrentAttemptID}}
	if ev.Detail == "" && e.Error != nil {
		ev.Detail = *e.Error
	}
	return ev, true
}
