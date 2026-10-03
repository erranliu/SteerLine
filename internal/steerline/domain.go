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
type ExecutionRecord = contracts.ExecutionRecord

// These aliases are temporary decoded views for the not-yet-migrated lifecycle
// consumers. Persisted records carry only implementation data envelopes.
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

func SetTaskStatus(record *TaskRecord, next string, mergedPR bool) error {
	return task.SetStatus(record, next, mergedPR, now())
}
