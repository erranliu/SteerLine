package sqlite

import (
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/query"
	"github.com/erranliu/SteerLine/internal/steerline/task"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

type BodyCompleteness = contracts.BodyCompleteness
type Command = contracts.Command
type CommandRecord = contracts.CommandRecord
type ConversationBodyRef = contracts.ConversationBodyRef
type ConversationCapabilityReport = contracts.ConversationCapabilityReport
type ConversationCheckpoint = contracts.ConversationCheckpoint
type ConversationEvent = contracts.ConversationEvent
type ConversationFilter = contracts.ConversationFilter
type ConversationRawRef = contracts.ConversationRawRef
type ConversationWrite = contracts.ConversationWrite
type DecisionAnswer = contracts.DecisionAnswer
type DecisionRecord = contracts.DecisionRecord
type DecisionRequest = contracts.DecisionRequest
type DomainEvent = contracts.DomainEvent
type ExecutionRecord = contracts.ExecutionRecord
type FinalAvailability = contracts.FinalAvailability
type FixedExecutionProfile = contracts.FixedExecutionProfile
type ManualStopConfirmation = contracts.ManualStopConfirmation
type NativeTaskCapabilities = contracts.NativeTaskCapabilities
type NativeTaskDetail = contracts.NativeTaskDetail
type NativeTaskPage = contracts.NativeTaskPage
type OperationRecord = contracts.OperationRecord
type PRState = contracts.PRState
type PendingQuestion = contracts.PendingQuestion
type ProjectTaskAuthority = contracts.ProjectTaskAuthority
type ProjectView = contracts.ProjectView
type Repository = contracts.Repository
type Runtime = contracts.Runtime
type ServiceCheckpoint = contracts.ServiceCheckpoint
type StepAttemptRecord = contracts.StepAttemptRecord
type TaskBatchRecord = contracts.TaskBatchRecord
type TaskContract = contracts.TaskContract
type TaskDeliveryEvidence = contracts.TaskDeliveryEvidence
type TaskDeliveryRecord = contracts.TaskDeliveryRecord
type TaskDependencyRecord = contracts.TaskDependencyRecord
type TaskDependencyStatus = task.TaskDependencyStatus
type TaskRecord = contracts.TaskRecord
type WorkflowRunRecord = contracts.WorkflowRunRecord
type workerCommit = persistence.WorkerCommit
type workerCommitResult = persistence.WorkerCommitResult
type recordWrite[T any] = persistence.RecordWrite[T]
type taskGraphSnapshot = task.Snapshot

const WorkerFlowRuntimeSchema = contracts.WorkerFlowRuntimeSchema

type DomainSnapshot = contracts.DomainSnapshot

const MaxEventPageBytes = query.MaxEventPageBytes
const BodyPartial = contracts.BodyPartial
const BodyComplete = contracts.BodyComplete

const WorkerFlowVersion = workerflow.Version
const ConversationSession = contracts.ConversationSession
const ConversationTurnStarted = contracts.ConversationTurnStarted
const ConversationMessageComplete = contracts.ConversationMessageComplete
const MessagePhaseFinalAnswer = contracts.MessagePhaseFinalAnswer
