package steerline

import (
	"regexp"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/task"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

// Shared records and client views live in contracts; task owns task rules.
// These aliases wire the center's existing command and worker entry points.
// WorkerFlowVersion is the persisted protocol for a single resumable worker session.
const WorkerFlowVersion = workerflow.Version

type TaskRecord = contracts.TaskRecord
type FileReference = contracts.FileReference
type TaskContract = contracts.TaskContract
type ProjectTaskAuthority = contracts.ProjectTaskAuthority
type TaskBatchRecord = contracts.TaskBatchRecord
type TaskDependencyRecord = contracts.TaskDependencyRecord
type TaskDeliveryRecord = contracts.TaskDeliveryRecord
type TaskDeliveryEvidence = contracts.TaskDeliveryEvidence
type WorkflowRunRecord = contracts.WorkflowRunRecord
type StepRunnerSettings = contracts.StepRunnerSettings
type ExecutionRecord = contracts.ExecutionRecord
type OrcaNativeExecution = contracts.OrcaNativeExecution
type OrcaNativeTurn = contracts.OrcaNativeTurn
type ExecutionTarget = contracts.ExecutionTarget
type WorkspaceEvidence = contracts.WorkspaceEvidence
type StepAttemptRecord = contracts.StepAttemptRecord
type DecisionRecord = contracts.DecisionRecord
type DomainEvent = contracts.DomainEvent
type OperationRecord = contracts.OperationRecord
type CommandRecord = contracts.CommandRecord

type WorkerEnvironment = contracts.WorkerEnvironment
type EnvironmentFeedback = contracts.EnvironmentFeedback
type WorkerPreparation = contracts.WorkerPreparation
type WorkerHostBlocker = contracts.WorkerHostBlocker

var revisionPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func validStepAttemptStatus(v string) bool {
	switch v {
	case "planned", "dispatching", "dispatched", "running", "waiting_decision", "succeeded", "failed", "rejected", "stopping", "stopped", "outcome_unknown", "abandoned":
		return true
	}
	return false
}

func validTaskStatus(v string) bool { return task.ValidStatus(v) }
func validWorkflowPhase(v string) bool {
	switch v {
	case "queued", "executing", "waiting_decision", "waiting_pr_merge", "completing", "completed", "blocked", "abandoned":
		return true
	}
	return false
}
func validExecutionStatus(v string) bool {
	switch v {
	case "pending", "active", "idle", "waiting_decision", "succeeded", "failed", "blocked", "stopped", "abandoned":
		return true
	}
	return false
}
func validDecisionStatus(v string) bool {
	switch v {
	case "pending", "answer_saved", "resuming", "reply_accepted", "applied", "failed", "superseded":
		return true
	}
	return false
}
func validOperationStatus(v string) bool {
	switch v {
	case "pending", "in_flight", "accepted", "confirmed", "rejected", "outcome_unknown":
		return true
	}
	return false
}

func SetTaskStatus(record *TaskRecord, next string, mergedPR bool) error {
	return task.SetStatus(record, next, mergedPR, now())
}
