package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

const (
	ConversationVersion      = 3
	MaxConversationBodyBytes = 16 << 20
)

type ConversationEventType string
type ConversationSourceRole string

const (
	ConversationSourceCanonical ConversationSourceRole = "canonical"
	ConversationSourceFallback  ConversationSourceRole = "fallback"
)

const (
	ConversationSession         ConversationEventType = "session"
	ConversationTurnStarted     ConversationEventType = "turn.started"
	ConversationMessageComplete ConversationEventType = "message.completed"
	ConversationInputWait       ConversationEventType = "input.wait"
	ConversationTurnCompleted   ConversationEventType = "turn.completed"
	ConversationTurnFailed      ConversationEventType = "turn.failed"
	ConversationTurnCancelled   ConversationEventType = "turn.cancelled"
	ConversationGap             ConversationEventType = "gap"
)

type IdentityQuality string

const (
	IdentityNative  IdentityQuality = "native"
	IdentityDerived IdentityQuality = "derived"
	IdentityUnknown IdentityQuality = "unknown"
)

type MessagePhase string

const (
	MessagePhaseCommentary  MessagePhase = "commentary"
	MessagePhaseFinalAnswer MessagePhase = "final_answer"
	MessagePhaseUnknown     MessagePhase = "unknown"
)

type FinalAvailability string

const (
	FinalAvailable FinalAvailability = "available"
	FinalPending   FinalAvailability = "final_pending"
	FinalAbsent    FinalAvailability = "final_absent"
	FinalUnknown   FinalAvailability = "final_unknown"
)

type BodyCompleteness string

const (
	BodyComplete  BodyCompleteness = "complete"
	BodyPartial   BodyCompleteness = "partial"
	BodyTruncated BodyCompleteness = "truncated"
	BodyMissing   BodyCompleteness = "missing"
)

type ConversationEvent struct {
	Version            int                                `json:"v"`
	EventID            int64                              `json:"event_id,omitempty"`
	ProjectID          string                             `json:"project_id"`
	TaskID             string                             `json:"task_id,omitempty"`
	WorkflowID         string                             `json:"workflow_id,omitempty"`
	ExecutionID        string                             `json:"execution_id"`
	AttemptID          string                             `json:"attempt_id"`
	Backend            executionbackend.BackendKind       `json:"backend,omitempty"`
	Protocol           executionbackend.ExecutionProtocol `json:"protocol,omitempty"`
	Agent              string                             `json:"agent,omitempty"`
	TargetID           string                             `json:"target_id,omitempty"`
	ExternalRef        string                             `json:"external_ref,omitempty"`
	Source             string                             `json:"source"`
	SourceRole         ConversationSourceRole             `json:"source_role,omitempty"`
	SourceSegment      string                             `json:"source_segment"`
	SourceIdentity     string                             `json:"source_identity,omitempty"`
	SourceCursor       string                             `json:"source_cursor,omitempty"`
	SessionID          string                             `json:"session_id,omitempty"`
	SessionQuality     IdentityQuality                    `json:"session_quality"`
	TurnID             string                             `json:"turn_id,omitempty"`
	TurnQuality        IdentityQuality                    `json:"turn_quality"`
	ItemID             string                             `json:"item_id,omitempty"`
	ItemQuality        IdentityQuality                    `json:"item_quality"`
	ItemKey            string                             `json:"item_key,omitempty"`
	SourceEventID      string                             `json:"source_event_id,omitempty"`
	SourceEventQuality IdentityQuality                    `json:"source_event_quality"`
	SourceSeq          uint64                             `json:"source_seq,omitempty"`
	SourceRevision     int64                              `json:"source_revision,omitempty"`
	LocalSeq           uint64                             `json:"local_seq,omitempty"`
	Type               ConversationEventType              `json:"type"`
	Phase              MessagePhase                       `json:"phase,omitempty"`
	FinalAvailability  FinalAvailability                  `json:"final_availability,omitempty"`
	Status             string                             `json:"status,omitempty"`
	Body               *ConversationBodyRef               `json:"body,omitempty"`
	Evidence           map[string]string                  `json:"evidence,omitempty"`
	Attributes         map[string]any                     `json:"attributes,omitempty"`
	OccurredAt         *time.Time                         `json:"occurred_at,omitempty"`
	ReceivedAt         time.Time                          `json:"received_at"`
	Diagnostic         string                             `json:"diagnostic,omitempty"`
}

type ConversationBodyRef struct {
	ExecutionID  string           `json:"execution_id"`
	AttemptID    string           `json:"attempt_id"`
	ItemKey      string           `json:"item_key"`
	Revision     int64            `json:"revision"`
	SHA256       string           `json:"sha256"`
	Bytes        int64            `json:"bytes"`
	Source       string           `json:"source"`
	Completeness BodyCompleteness `json:"completeness"`
}

type ConversationWrite struct {
	Event      ConversationEvent
	Body       []byte
	BodyMode   string
	BodySource string
}

type ConversationCheckpoint struct {
	ProjectID      string    `json:"project_id"`
	ExecutionID    string    `json:"execution_id"`
	AttemptID      string    `json:"attempt_id"`
	Source         string    `json:"source"`
	SourceSegment  string    `json:"source_segment"`
	SourceIdentity string    `json:"source_identity,omitempty"`
	Generation     string    `json:"generation,omitempty"`
	Cursor         string    `json:"cursor"`
	LocalSeq       uint64    `json:"local_seq"`
	UpdatedAt      time.Time `json:"updated_at"`
	AllowReplay    bool      `json:"-"`
}

type ConversationCapability struct {
	Name     string                            `json:"name"`
	Status   executionbackend.CapabilityStatus `json:"status"`
	Evidence string                            `json:"evidence,omitempty"`
}

type ConversationCapabilityReport struct {
	Version         int                                `json:"version"`
	Backend         executionbackend.BackendKind       `json:"backend"`
	Protocol        executionbackend.ExecutionProtocol `json:"protocol"`
	Agent           string                             `json:"agent"`
	AgentVersion    string                             `json:"agent_version,omitempty"`
	PlatformVersion string                             `json:"platform_version,omitempty"`
	Target          executionbackend.ExecutionTarget   `json:"target"`
	ExecutionID     string                             `json:"execution_id,omitempty"`
	AttemptID       string                             `json:"attempt_id,omitempty"`
	Source          string                             `json:"source,omitempty"`
	SourceIdentity  string                             `json:"source_identity,omitempty"`
	CheckedAt       time.Time                          `json:"checked_at"`
	Capabilities    []ConversationCapability           `json:"capabilities"`
}

func (e *ConversationEvent) EventIDFallback(reason string) {
	if e.SourceEventID == "" {
		e.SourceEventID = reason + ":" + fmt.Sprint(e.SourceSeq)
	}
}

func (e ConversationEvent) ItemIdentity() string {
	return strings.Join([]string{e.ProjectID, e.ExecutionID, e.AttemptID, e.SourceSegment, e.SessionID, e.TurnID, e.ItemID}, "\x00")
}

func (e ConversationEvent) SourceKey() string {
	identity := strings.Join([]string{e.ProjectID, e.ExecutionID, e.AttemptID, e.Source, e.SourceSegment, e.SourceEventID}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}

func (e ConversationEvent) Validate() error {
	if e.Version != ConversationVersion || e.ProjectID == "" || e.ExecutionID == "" || e.AttemptID == "" || e.Source == "" || e.SourceSegment == "" || e.Type == "" || e.SourceEventID == "" {
		return errors.New("conversation event is missing versioned identity or source evidence")
	}
	if e.SourceRole != ConversationSourceCanonical && e.SourceRole != ConversationSourceFallback {
		return errors.New("conversation event has an unsupported source role")
	}
	switch e.Type {
	case ConversationSession, ConversationTurnStarted, ConversationTurnCompleted, ConversationTurnFailed, ConversationTurnCancelled, ConversationInputWait, ConversationGap:
	case ConversationMessageComplete:
		if e.Phase != MessagePhaseFinalAnswer || e.FinalAvailability != FinalAvailable {
			return errors.New("only a complete final reply may be stored as a message")
		}
	default:
		return errors.New("process conversation content is not retained by the center")
	}
	if e.ReceivedAt.IsZero() {
		return errors.New("conversation event is missing received_at")
	}
	if len(e.Attributes) > 0 {
		b, err := json.Marshal(e.Attributes)
		if err != nil || len(b) > 64<<10 {
			return errors.New("conversation event attributes exceed 64 KiB")
		}
	}
	if !validIdentityQuality(e.SessionQuality) || !validIdentityQuality(e.TurnQuality) || !validIdentityQuality(e.SourceEventQuality) || !validIdentityQuality(e.ItemQuality) {
		return errors.New("conversation event has invalid identity quality")
	}
	if e.Phase != MessagePhaseCommentary && e.Phase != MessagePhaseFinalAnswer && e.Phase != MessagePhaseUnknown {
		return errors.New("conversation event has invalid message phase")
	}
	return nil
}

func validIdentityQuality(value IdentityQuality) bool {
	return value == IdentityNative || value == IdentityDerived || value == IdentityUnknown
}
