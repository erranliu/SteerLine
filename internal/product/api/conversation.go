package api

import (
	"time"
)

const ConversationVersion = 3

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

// ConversationEvent is the public, backend-neutral record consumed by the
// console. Source IDs and turn IDs carry explicit quality; display text and
// timestamps are never used to infer identity.
type ConversationEvent struct {
	Version            int                    `json:"v"`
	EventID            int64                  `json:"event_id,omitempty"`
	ProjectID          string                 `json:"project_id"`
	TaskID             string                 `json:"task_id,omitempty"`
	WorkflowID         string                 `json:"workflow_id,omitempty"`
	ExecutionID        string                 `json:"execution_id"`
	AttemptID          string                 `json:"attempt_id"`
	Backend            string                 `json:"backend,omitempty"`
	Protocol           string                 `json:"protocol,omitempty"`
	Agent              string                 `json:"agent,omitempty"`
	TargetID           string                 `json:"target_id,omitempty"`
	Source             string                 `json:"source"`
	SourceRole         ConversationSourceRole `json:"source_role,omitempty"`
	SourceSegment      string                 `json:"source_segment"`
	SourceIdentity     string                 `json:"source_identity,omitempty"`
	SessionID          string                 `json:"session_id,omitempty"`
	SessionQuality     IdentityQuality        `json:"session_quality"`
	TurnID             string                 `json:"turn_id,omitempty"`
	TurnQuality        IdentityQuality        `json:"turn_quality"`
	ItemID             string                 `json:"item_id,omitempty"`
	ItemQuality        IdentityQuality        `json:"item_quality"`
	ItemKey            string                 `json:"item_key,omitempty"`
	SourceEventID      string                 `json:"source_event_id,omitempty"`
	SourceEventQuality IdentityQuality        `json:"source_event_quality"`
	SourceSeq          uint64                 `json:"source_seq,omitempty"`
	SourceRevision     int64                  `json:"source_revision,omitempty"`
	Type               ConversationEventType  `json:"type"`
	Phase              MessagePhase           `json:"phase,omitempty"`
	FinalAvailability  FinalAvailability      `json:"final_availability,omitempty"`
	Status             string                 `json:"status,omitempty"`
	Body               *ConversationBodyRef   `json:"body,omitempty"`
	EndReason          string                 `json:"end_reason,omitempty"`
	OccurredAt         *time.Time             `json:"occurred_at,omitempty"`
	ReceivedAt         time.Time              `json:"received_at"`
	Diagnostic         string                 `json:"diagnostic,omitempty"`
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

type ConversationCapability struct {
	Evidence string `json:"evidence,omitempty"`
	Name     string `json:"name"`
	Status   string `json:"status"`
}

type ConversationCapabilityReport struct {
	Version         int                      `json:"version"`
	Backend         string                   `json:"backend"`
	Protocol        string                   `json:"protocol"`
	Agent           string                   `json:"agent"`
	AgentVersion    string                   `json:"agent_version,omitempty"`
	PlatformVersion string                   `json:"platform_version,omitempty"`
	Target          TargetSummary            `json:"target"`
	ExecutionID     string                   `json:"execution_id,omitempty"`
	AttemptID       string                   `json:"attempt_id,omitempty"`
	Source          string                   `json:"source,omitempty"`
	SourceIdentity  string                   `json:"source_identity,omitempty"`
	CheckedAt       time.Time                `json:"checked_at"`
	Capabilities    []ConversationCapability `json:"capabilities"`
}

type ConversationFilter struct {
	FinalsOnly  bool   `json:"finals_only,omitempty"`
	WorkflowID  string `json:"workflow_id,omitempty"`
	ExecutionID string `json:"execution_id,omitempty"`
	AttemptID   string `json:"attempt_id,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	TurnID      string `json:"turn_id,omitempty"`
	Source      string `json:"source,omitempty"`
}

type ConversationPage struct {
	Events       []ConversationEvent `json:"events"`
	NextCursor   string              `json:"next_cursor,omitempty"`
	HasMore      bool                `json:"has_more"`
	PayloadBytes int                 `json:"payload_bytes"`
}

type ConversationBodyPage struct {
	ItemKey      string           `json:"item_key"`
	Revision     int64            `json:"revision"`
	Offset       int64            `json:"offset"`
	NextOffset   int64            `json:"next_offset"`
	TotalBytes   int64            `json:"total_bytes"`
	SHA256       string           `json:"sha256"`
	Completeness BodyCompleteness `json:"completeness"`
	Complete     bool             `json:"complete"`
	Data         []byte           `json:"data"`
}

type ActivityEvent struct {
	V         int            `json:"v"`
	Seq       uint64         `json:"seq"`
	TS        time.Time      `json:"ts"`
	Source    string         `json:"source"`
	RunID     string         `json:"run_id,omitempty"`
	TaskID    string         `json:"task_id,omitempty"`
	ItemID    string         `json:"item_id,omitempty"`
	Kind      string         `json:"kind"`
	Status    string         `json:"status,omitempty"`
	Title     string         `json:"title,omitempty"`
	Detail    string         `json:"detail,omitempty"`
	ElapsedMS int64          `json:"elapsed_ms,omitempty"`
	Meta      map[string]any `json:"meta,omitempty"`
}
