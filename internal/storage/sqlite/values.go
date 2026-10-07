package sqlite

import (
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	productdto "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/query"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type BodyCompleteness = executionvalues.BodyCompleteness

type ConversationBodyRef = executionvalues.ConversationBodyRef

type ConversationCapabilityReport = executionvalues.ConversationCapabilityReport

type ConversationCheckpoint = executionvalues.ConversationCheckpoint

type ConversationEvent = executionvalues.ConversationEvent

type ConversationFilter = productdto.ConversationFilter

type ConversationWrite = executionvalues.ConversationWrite

type FinalAvailability = executionvalues.FinalAvailability

type recordWrite[T any] = storageports.RecordWrite[T]

type taskGraphSnapshot = storageports.TaskGraphSnapshot

const MaxEventPageBytes = query.MaxEventPageBytes

const BodyPartial = executionvalues.BodyPartial

const BodyComplete = executionvalues.BodyComplete

const ConversationSession = executionvalues.ConversationSession

const ConversationTurnStarted = executionvalues.ConversationTurnStarted

const ConversationMessageComplete = executionvalues.ConversationMessageComplete

const MessagePhaseFinalAnswer = executionvalues.MessagePhaseFinalAnswer
