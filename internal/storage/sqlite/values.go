package sqlite

import (
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type BodyCompleteness = executionvalues.BodyCompleteness

type ConversationBodyRef = executionvalues.ConversationBodyRef

type ConversationCapabilityReport = executionvalues.ConversationCapabilityReport

type ConversationCheckpoint = executionvalues.ConversationCheckpoint

type ConversationEvent = executionvalues.ConversationEvent

type ConversationWrite = executionvalues.ConversationWrite

type FinalAvailability = executionvalues.FinalAvailability

type taskGraphSnapshot = storageports.TaskGraphSnapshot

const MaxEventPageBytes = storageports.MaxEventPageBytes

const BodyPartial = executionvalues.BodyPartial

const BodyComplete = executionvalues.BodyComplete

const ConversationSession = executionvalues.ConversationSession

const ConversationTurnStarted = executionvalues.ConversationTurnStarted

const ConversationMessageComplete = executionvalues.ConversationMessageComplete

const MessagePhaseFinalAnswer = executionvalues.MessagePhaseFinalAnswer
