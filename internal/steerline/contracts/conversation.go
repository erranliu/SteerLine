package contracts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

const ConversationVersion = 2

type ConversationEventType string

type ConversationSourceRole string

const (
	ConversationSourceCanonical ConversationSourceRole = "canonical"
	ConversationSourceFallback  ConversationSourceRole = "fallback"
)

const (
	ConversationSession         ConversationEventType = "session"
	ConversationTurnStarted     ConversationEventType = "turn.started"
	ConversationMessageDelta    ConversationEventType = "message.delta"
	ConversationMessageSnapshot ConversationEventType = "message.snapshot"
	ConversationMessageComplete ConversationEventType = "message.completed"
	ConversationToolStarted     ConversationEventType = "tool.started"
	ConversationToolDelta       ConversationEventType = "tool.delta"
	ConversationToolCompleted   ConversationEventType = "tool.completed"
	ConversationInputWait       ConversationEventType = "input.wait"
	ConversationTurnCompleted   ConversationEventType = "turn.completed"
	ConversationTurnFailed      ConversationEventType = "turn.failed"
	ConversationTurnCancelled   ConversationEventType = "turn.cancelled"
	ConversationObservation     ConversationEventType = "observation"
	ConversationGap             ConversationEventType = "gap"
	ConversationExecutionResult ConversationEventType = "execution.result"
	ConversationUnknown         ConversationEventType = "unknown"
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
	Raw                *ConversationRawRef                `json:"raw,omitempty"`
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

type ConversationRawRef struct {
	ExecutionID string `json:"execution_id"`
	AttemptID   string `json:"attempt_id"`
	ID          string `json:"id"`
	SHA256      string `json:"sha256"`
	Bytes       int64  `json:"bytes"`
}

type ConversationWrite struct {
	Event      ConversationEvent
	Body       []byte
	BodyMode   string // delta, snapshot, or complete
	Raw        []byte
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

// ConversationMapperSnapshot is the minimum committed history an implementation
// needs to reconstruct transient mapping state. It keeps mapper state derived
// from committed events instead of making a mutable mapper a second cursor.
type ConversationMapperSnapshot struct {
	Session   *ConversationEvent `json:"session,omitempty"`
	Turn      *ConversationEvent `json:"turn,omitempty"`
	FinalSeen bool               `json:"final_seen,omitempty"`
}

// ConversationMapper maps one source record to shared values. Implementations
// keep any candidate state local to the mapper instance; callers reconstruct
// it from committed events after a failed batch.
type ConversationMapper interface {
	MapRecord(ConversationEvent, []byte) ConversationWrite
}

type ConversationInput struct {
	Event         ConversationEvent
	Process       *executionbackend.ProcessIdentity
	Source        string
	Segment       string
	Identity      string
	Cursor        string
	Generation    string
	Session       string
	SourceSeq     uint64
	SourceOffset  int64
	SourceLength  int64
	Raw           []byte
	Diagnostic    string
	Gap           bool
	InputWait     bool
	SourceChanged bool
	Mapper        ConversationMapperSnapshot
}

type ConversationMappedBatch struct {
	Writes               []ConversationWrite
	Checkpoint           ConversationCheckpoint
	VerifySourcePosition bool
}

type ConversationInputMapper interface {
	MapConversation(ConversationInput) (ConversationMappedBatch, error)
}

type ConversationSourcePort interface {
	ConversationInputMapper
	RestoreMapper(ConversationMapperSnapshot) ConversationMapper
	JournalSources() []string
	JournalSource(executionbackend.BackendKind, executionbackend.ExecutionProtocol) string
	JournalSegment(executionbackend.BackendKind, executionbackend.ExecutionProtocol, string) string
	JournalGeneration(fileIdentity, segment string) (uint64, error)
	LiveJournalPath(context.Context, ConversationJournalPathRequest) (string, error)
	DiscoverHistory(context.Context, ConversationDiscovery) (ConversationDiscoveryResult, error)
	DiscoverJournals(context.Context, ConversationDiscovery, func(string) (string, error)) ([]ConversationJournalSource, error)
	ReadHistory(context.Context, ConversationHistoryReadRequest, func(*exec.Cmd) (executionbackend.StopHandle, error)) (ConversationHistoryBatch, error)
}

type ConversationJournalPathRequest struct {
	RunDirectory, WorkflowID, ExecutionID, AttemptID, SourceIdentity, LogPath string
	EventPaths                                                                []string
	WorkerFlow                                                                bool
}

type ConversationDiscovery struct {
	ProjectID, RunDirectory, Repository string
	Runs                                []ConversationRunSource
}

type ConversationRunSource struct {
	Base         ConversationEvent
	RunDirectory string
	Workspace    string
	WorkerFlow   bool
	Process      *executionbackend.ProcessIdentity
	Attempts     []ConversationAttemptSource
}

type ConversationAttemptSource struct {
	AttemptID string
	EventPath string
	FileID    string
	Ended     bool
}

type ConversationJournalSource struct {
	Path, AttemptID, FileID, ExpectedFileID, Diagnostic, Workspace string
	Event                                                          ConversationEvent
	Ended                                                          bool
}

type ConversationHistorySource struct {
	Identity                string
	BindingPath             string
	Workspace               string
	Event                   ConversationEvent
	Ended                   bool
	RequiresPlatformVersion bool
}

type ConversationDiscoveryResult struct {
	Sources []ConversationHistorySource
	Writes  []ConversationWrite
}

type ConversationHistoryReadRequest struct {
	Source          ConversationHistorySource
	Cursor          string
	HasCheckpoint   bool
	Repository      string
	RunDirectory    string
	PlatformVersion string
}

type ConversationHistoryBatch struct {
	Writes       []ConversationWrite
	Cursor       string
	Sequence     uint64
	Settled      bool
	More         bool
	Capabilities *ConversationCapabilityReport
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
	if e.ReceivedAt.IsZero() {
		return errors.New("conversation event is missing received_at")
	}
	if len(e.Attributes) > 0 {
		b, err := json.Marshal(e.Attributes)
		if err != nil || len(b) > 64<<10 {
			return errors.New("conversation event attributes exceed 64 KiB")
		}
	}
	switch e.SessionQuality {
	case IdentityNative, IdentityDerived, IdentityUnknown:
	default:
		return errors.New("conversation event has invalid session identity quality")
	}
	switch e.TurnQuality {
	case IdentityNative, IdentityDerived, IdentityUnknown:
	default:
		return errors.New("conversation event has invalid turn identity quality")
	}
	for name, quality := range map[string]IdentityQuality{"source event": e.SourceEventQuality, "item": e.ItemQuality} {
		switch quality {
		case IdentityNative, IdentityDerived, IdentityUnknown:
		default:
			return fmt.Errorf("conversation event has invalid %s identity quality", name)
		}
	}
	if e.Phase == "" {
		return errors.New("conversation event must declare a message phase, including unknown")
	}
	switch e.Phase {
	case MessagePhaseCommentary, MessagePhaseFinalAnswer, MessagePhaseUnknown:
	default:
		return errors.New("conversation event has invalid message phase")
	}
	return nil
}
