package sqlite

import (
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
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

type Repository = contracts.Repository

type recordWrite[T any] = storageports.RecordWrite[T]

type taskGraphSnapshot = storageports.TaskGraphSnapshot

const MaxEventPageBytes = query.MaxEventPageBytes

const BodyPartial = contracts.BodyPartial

const BodyComplete = contracts.BodyComplete

const ConversationSession = contracts.ConversationSession

const ConversationTurnStarted = contracts.ConversationTurnStarted

const ConversationMessageComplete = contracts.ConversationMessageComplete

const MessagePhaseFinalAnswer = contracts.MessagePhaseFinalAnswer
