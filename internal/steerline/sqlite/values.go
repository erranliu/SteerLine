package sqlite

import (
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/query"
	"github.com/erranliu/SteerLine/internal/steerline/task"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type BodyCompleteness = contracts.BodyCompleteness

type ConversationBodyRef = contracts.ConversationBodyRef

type ConversationCapabilityReport = contracts.ConversationCapabilityReport

type ConversationCheckpoint = contracts.ConversationCheckpoint

type ConversationEvent = contracts.ConversationEvent

type ConversationFilter = contracts.ConversationFilter

type ConversationRawRef = contracts.ConversationRawRef

type ConversationWrite = contracts.ConversationWrite

type FinalAvailability = contracts.FinalAvailability

type NativeTaskCapabilities = contracts.NativeTaskCapabilities

type NativeTaskDetail = contracts.NativeTaskDetail

type NativeTaskPage = contracts.NativeTaskPage

type ProjectView = contracts.ProjectView

type Repository = contracts.Repository

type Runtime = contracts.Runtime

type ServiceCheckpoint = contracts.ServiceCheckpoint

type TaskDeliveryEvidence = contracts.TaskDeliveryEvidence

type TaskDeliveryRecord = contracts.TaskDeliveryRecord

type workerCommit = persistence.WorkerCommit

type workerCommitResult = persistence.WorkerCommitResult

type recordWrite[T any] = storageports.RecordWrite[T]

type taskGraphSnapshot = task.Snapshot

const WorkerFlowRuntimeSchema = contracts.WorkerFlowRuntimeSchema

type DomainSnapshot = contracts.DomainSnapshot

const MaxEventPageBytes = query.MaxEventPageBytes

const BodyPartial = contracts.BodyPartial

const BodyComplete = contracts.BodyComplete

const ConversationSession = contracts.ConversationSession

const ConversationTurnStarted = contracts.ConversationTurnStarted

const ConversationMessageComplete = contracts.ConversationMessageComplete

const MessagePhaseFinalAnswer = contracts.MessagePhaseFinalAnswer
