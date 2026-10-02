package steerline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

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

func conversationItemKey(e ConversationEvent, itemID string) string {
	if itemID == "" {
		return ""
	}
	e.ItemID = itemID
	sum := sha256.Sum256([]byte(e.ItemIdentity()))
	return hex.EncodeToString(sum[:])
}

func sourceEventFallback(e ConversationEvent, raw []byte) string {
	if e.SourceSeq != 0 {
		return fmt.Sprintf("seq:%d", e.SourceSeq)
	}
	if e.Evidence != nil {
		if start, ok := e.Evidence["byte_start"]; ok {
			if end, found := e.Evidence["byte_end"]; found {
				return "bytes:" + start + ":" + end
			}
		}
	}
	// No source identity was available. Persist uniquely and expose the missing
	// identity instead of deleting equal text as a duplicate.
	return "unidentified:" + newID()
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func nonZeroTime(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback.UTC()
	}
	return value.UTC()
}

func nonZeroUint(value, fallback uint64) uint64 {
	if value == 0 {
		return fallback
	}
	return value
}

func qualityOrUnknown(value IdentityQuality) IdentityQuality {
	if value == "" {
		return IdentityUnknown
	}
	return value
}

func cloneStringMap(value map[string]string) map[string]string {
	if value == nil {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func rawObject(raw json.RawMessage) map[string]json.RawMessage {
	var result map[string]json.RawMessage
	_ = json.Unmarshal(raw, &result)
	if result == nil {
		return map[string]json.RawMessage{}
	}
	return result
}

func stringField(fields map[string]json.RawMessage, name string) string {
	var value string
	_ = json.Unmarshal(fields[name], &value)
	return value
}

func stringFieldPresent(fields map[string]json.RawMessage, name string) (string, bool) {
	raw, ok := fields[name]
	if !ok {
		return "", false
	}
	var value *string
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return "", false
	}
	return *value, true
}

func uintField(fields map[string]json.RawMessage, name string) uint64 {
	var value uint64
	_ = json.Unmarshal(fields[name], &value)
	return value
}

func int64Field(fields map[string]json.RawMessage, name string) int64 {
	var value int64
	_ = json.Unmarshal(fields[name], &value)
	return value
}

func firstNonEmpty(values ...string) string { return nonEmpty(values...) }
