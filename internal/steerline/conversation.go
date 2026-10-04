package steerline

import (
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

const ConversationVersion = contracts.ConversationVersion

type ConversationEventType = contracts.ConversationEventType

type IdentityQuality = contracts.IdentityQuality

type MessagePhase = contracts.MessagePhase

type FinalAvailability = contracts.FinalAvailability

type BodyCompleteness = contracts.BodyCompleteness

type ConversationEvent = contracts.ConversationEvent

type ConversationBodyRef = contracts.ConversationBodyRef

type ConversationRawRef = contracts.ConversationRawRef

type ConversationWrite = contracts.ConversationWrite

type ConversationCheckpoint = contracts.ConversationCheckpoint

type ConversationCapability = contracts.ConversationCapability

type ConversationCapabilityReport = contracts.ConversationCapabilityReport

const (
	ConversationSession         = contracts.ConversationSession
	ConversationTurnStarted     = contracts.ConversationTurnStarted
	ConversationMessageDelta    = contracts.ConversationMessageDelta
	ConversationMessageSnapshot = contracts.ConversationMessageSnapshot
	ConversationMessageComplete = contracts.ConversationMessageComplete
	ConversationToolStarted     = contracts.ConversationToolStarted
	ConversationToolDelta       = contracts.ConversationToolDelta
	ConversationToolCompleted   = contracts.ConversationToolCompleted
	ConversationInputWait       = contracts.ConversationInputWait
	ConversationTurnCompleted   = contracts.ConversationTurnCompleted
	ConversationTurnFailed      = contracts.ConversationTurnFailed
	ConversationTurnCancelled   = contracts.ConversationTurnCancelled
	ConversationObservation     = contracts.ConversationObservation
	ConversationGap             = contracts.ConversationGap
	ConversationExecutionResult = contracts.ConversationExecutionResult
	ConversationUnknown         = contracts.ConversationUnknown
	IdentityNative              = contracts.IdentityNative
	IdentityDerived             = contracts.IdentityDerived
	IdentityUnknown             = contracts.IdentityUnknown
	MessagePhaseCommentary      = contracts.MessagePhaseCommentary
	MessagePhaseFinalAnswer     = contracts.MessagePhaseFinalAnswer
	MessagePhaseUnknown         = contracts.MessagePhaseUnknown
	FinalAvailable              = contracts.FinalAvailable
	FinalPending                = contracts.FinalPending
	FinalAbsent                 = contracts.FinalAbsent
	FinalUnknown                = contracts.FinalUnknown
	BodyComplete                = contracts.BodyComplete
	BodyPartial                 = contracts.BodyPartial
	BodyTruncated               = contracts.BodyTruncated
	BodyMissing                 = contracts.BodyMissing
)
