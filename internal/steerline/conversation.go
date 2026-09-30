package steerline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

const ConversationVersion = 1
const ConversationSourceMarkerName = "conversation-source.v1.json"

type ConversationSourceMarker struct {
	Version        int                            `json:"v"`
	ProjectID      string                         `json:"project_id"`
	WorkflowID     string                         `json:"workflow_id"`
	ExecutionID    string                         `json:"execution_id"`
	AttemptID      string                         `json:"attempt_id"`
	Backend        executionapi.BackendKind       `json:"backend"`
	Protocol       executionapi.ExecutionProtocol `json:"protocol"`
	Agent          string                         `json:"agent"`
	TargetID       string                         `json:"target_id"`
	Source         string                         `json:"source"`
	SourceSegment  string                         `json:"source_segment"`
	SourceIdentity string                         `json:"source_identity"`
	RecordName     string                         `json:"record_name"`
	CreatedAt      time.Time                      `json:"created_at"`
	ReaderCommand  Command                        `json:"reader_command,omitempty"`
	CodexHome      string                         `json:"codex_home,omitempty"`
}

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

func (m ConversationSourceMarker) validate() error {
	local := m.Backend == executionapi.BackendLocal && m.Protocol == executionapi.ProtocolLocalProcess && m.SourceSegment == "local-jsonl-v1:"+m.SourceIdentity
	orca := m.Backend == executionapi.BackendOrca && (m.Protocol == executionapi.ProtocolOrcaTerminalV2 || m.Protocol == executionapi.ProtocolOrcaSupervised) && m.SourceSegment == "orca-jsonl-v1:"+m.SourceIdentity
	if m.Version != 1 || m.ProjectID == "" || m.WorkflowID == "" || m.ExecutionID == "" || m.AttemptID == "" || !local && !orca || m.Agent == "" || m.TargetID == "" || m.Source != "codex-jsonl" || m.SourceIdentity == "" || m.RecordName != "events.jsonl" || m.CreatedAt.IsZero() {
		return errors.New("conversation source marker is incomplete or unsupported")
	}
	return nil
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
