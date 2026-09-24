package steerline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const ConversationVersion = 1
const ConversationSourceMarkerName = "conversation-source.v1.json"

type ConversationSourceMarker struct {
	Version        int               `json:"v"`
	ProjectID      string            `json:"project_id"`
	WorkflowID     string            `json:"workflow_id"`
	ExecutionID    string            `json:"execution_id"`
	AttemptID      string            `json:"attempt_id"`
	Backend        BackendKind       `json:"backend"`
	Protocol       ExecutionProtocol `json:"protocol"`
	Agent          string            `json:"agent"`
	TargetID       string            `json:"target_id"`
	Source         string            `json:"source"`
	SourceSegment  string            `json:"source_segment"`
	SourceIdentity string            `json:"source_identity"`
	RecordName     string            `json:"record_name"`
	CreatedAt      time.Time         `json:"created_at"`
	ReaderCommand  Command           `json:"reader_command,omitempty"`
	CodexHome      string            `json:"codex_home,omitempty"`
}

type ConversationEventType string

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
	Version            int                   `json:"v"`
	EventID            int64                 `json:"event_id,omitempty"`
	ProjectID          string                `json:"project_id"`
	TaskID             string                `json:"task_id,omitempty"`
	WorkflowID         string                `json:"workflow_id,omitempty"`
	ExecutionID        string                `json:"execution_id"`
	AttemptID          string                `json:"attempt_id"`
	Backend            BackendKind           `json:"backend,omitempty"`
	Protocol           ExecutionProtocol     `json:"protocol,omitempty"`
	Agent              string                `json:"agent,omitempty"`
	TargetID           string                `json:"target_id,omitempty"`
	ExternalRef        string                `json:"external_ref,omitempty"`
	Source             string                `json:"source"`
	SourceSegment      string                `json:"source_segment"`
	SourceIdentity     string                `json:"source_identity,omitempty"`
	SourceCursor       string                `json:"source_cursor,omitempty"`
	SessionID          string                `json:"session_id,omitempty"`
	SessionQuality     IdentityQuality       `json:"session_quality"`
	TurnID             string                `json:"turn_id,omitempty"`
	TurnQuality        IdentityQuality       `json:"turn_quality"`
	ItemID             string                `json:"item_id,omitempty"`
	ItemQuality        IdentityQuality       `json:"item_quality"`
	ItemKey            string                `json:"item_key,omitempty"`
	SourceEventID      string                `json:"source_event_id,omitempty"`
	SourceEventQuality IdentityQuality       `json:"source_event_quality"`
	SourceSeq          uint64                `json:"source_seq,omitempty"`
	SourceRevision     int64                 `json:"source_revision,omitempty"`
	LocalSeq           uint64                `json:"local_seq,omitempty"`
	Type               ConversationEventType `json:"type"`
	Phase              MessagePhase          `json:"phase,omitempty"`
	FinalAvailability  FinalAvailability     `json:"final_availability,omitempty"`
	Status             string                `json:"status,omitempty"`
	Body               *ConversationBodyRef  `json:"body,omitempty"`
	Raw                *ConversationRawRef   `json:"raw,omitempty"`
	Evidence           map[string]string     `json:"evidence,omitempty"`
	Attributes         map[string]any        `json:"attributes,omitempty"`
	OccurredAt         *time.Time            `json:"occurred_at,omitempty"`
	ReceivedAt         time.Time             `json:"received_at"`
	Diagnostic         string                `json:"diagnostic,omitempty"`
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

type ConversationCapability struct {
	Name     string           `json:"name"`
	Status   CapabilityStatus `json:"status"`
	Evidence string           `json:"evidence,omitempty"`
}

type ConversationCapabilityReport struct {
	Version         int                      `json:"version"`
	Backend         BackendKind              `json:"backend"`
	Protocol        ExecutionProtocol        `json:"protocol"`
	Agent           string                   `json:"agent"`
	AgentVersion    string                   `json:"agent_version,omitempty"`
	PlatformVersion string                   `json:"platform_version,omitempty"`
	Target          ExecutionTarget          `json:"target"`
	ExecutionID     string                   `json:"execution_id,omitempty"`
	AttemptID       string                   `json:"attempt_id,omitempty"`
	Source          string                   `json:"source,omitempty"`
	SourceIdentity  string                   `json:"source_identity,omitempty"`
	CheckedAt       time.Time                `json:"checked_at"`
	Capabilities    []ConversationCapability `json:"capabilities"`
}

type ConversationMapper struct {
	currentTurn    string
	turnQuality    IdentityQuality
	sessionID      string
	sessionQuality IdentityQuality
	turnFinalSeen  map[string]bool
}

// MapCodexJSONL maps observed JSONL fields only. In particular, turn completion
// does not promote an item to final_answer, and an absent phase stays unknown.
func (m *ConversationMapper) MapCodexJSONL(ctx ConversationEvent, raw []byte) ConversationWrite {
	if m == nil {
		m = &ConversationMapper{}
	}
	ctx.Version = ConversationVersion
	ctx.Backend, ctx.Protocol = BackendLocal, ProtocolLocalProcess
	ctx.Source = "codex-jsonl"
	ctx.SourceSegment = nonEmpty(ctx.SourceSegment, "local-jsonl-v1")
	ctx.SessionQuality = IdentityUnknown
	ctx.TurnQuality = IdentityUnknown
	ctx.SourceEventQuality = IdentityUnknown
	ctx.ItemQuality = IdentityUnknown
	ctx.Phase = MessagePhaseUnknown
	ctx.ReceivedAt = nonZeroTime(ctx.ReceivedAt, now())
	ctx.Evidence = cloneStringMap(ctx.Evidence)
	ctx.Attributes = map[string]any{}
	if len(raw) > 0 {
		ctx.Raw = &ConversationRawRef{}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		ctx.Type = ConversationUnknown
		ctx.Status = "unparsed"
		ctx.Diagnostic = "source line was not a JSON object"
		ctx.EventIDFallback("raw-line")
		if strings.HasPrefix(ctx.SourceEventID, "bytes:") || strings.HasPrefix(ctx.SourceEventID, "seq:") {
			ctx.SourceEventQuality = IdentityDerived
		}
		return ConversationWrite{Event: ctx, Raw: append([]byte(nil), raw...)}
	}
	var eventType string
	_ = json.Unmarshal(fields["type"], &eventType)
	ctx.SourceEventID = nonEmpty(ctx.SourceEventID, sourceEventFallback(ctx, raw))
	if strings.HasPrefix(ctx.SourceEventID, "bytes:") || strings.HasPrefix(ctx.SourceEventID, "seq:") {
		ctx.SourceEventQuality = IdentityDerived
	}
	ctx.SourceSeq = nonZeroUint(ctx.SourceSeq, uintField(fields, "sequence"))
	ctx.SourceRevision = int64Field(fields, "revision")
	if turnID := stringField(fields, "turn_id"); turnID != "" {
		m.currentTurn, m.turnQuality = turnID, IdentityNative
	}
	if sessionID := stringField(fields, "thread_id"); sessionID != "" {
		m.sessionID, m.sessionQuality = sessionID, IdentityNative
	}
	ctx.SessionID, ctx.SessionQuality = m.sessionID, qualityOrUnknown(m.sessionQuality)
	ctx.TurnID, ctx.TurnQuality = m.currentTurn, qualityOrUnknown(m.turnQuality)
	if ts := stringField(fields, "timestamp"); ts != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			v := parsed.UTC()
			ctx.OccurredAt = &v
		}
	}
	item := rawObject(fields["item"])
	if ctx.SourceRevision == 0 {
		ctx.SourceRevision = int64Field(item, "revision")
	}
	itemID := stringField(item, "id")
	itemType := stringField(item, "type")
	ctx.ItemID = itemID
	if itemID != "" {
		ctx.ItemQuality = IdentityNative
	}
	ctx.ItemKey = conversationItemKey(ctx, itemID)
	ctx.Attributes["source_type"] = eventType
	if itemType != "" {
		ctx.Attributes["item_type"] = itemType
	}
	phase := nonEmpty(stringField(item, "phase"), stringField(fields, "phase"))
	if phase == string(MessagePhaseCommentary) || phase == string(MessagePhaseFinalAnswer) {
		ctx.Phase = MessagePhase(phase)
	}
	var body []byte
	bodyMode := ""
	switch eventType {
	case "thread.started":
		ctx.Type = ConversationSession
		ctx.SessionID = nonEmpty(m.sessionID, stringField(fields, "thread_id"))
		if ctx.SessionID != "" {
			ctx.SessionQuality = IdentityNative
			m.sessionID, m.sessionQuality = ctx.SessionID, IdentityNative
		}
	case "turn.started":
		if id := stringField(fields, "turn_id"); id != "" {
			m.currentTurn, m.turnQuality = id, IdentityNative
		} else {
			// The persisted source byte range identifies this explicit boundary.
			m.currentTurn = "derived:" + ctx.SourceEventID
			m.turnQuality = IdentityDerived
			ctx.Evidence["turn_derivation"] = "codex-jsonl-turn-boundary-v1"
		}
		if m.turnFinalSeen == nil {
			m.turnFinalSeen = map[string]bool{}
		}
		ctx.TurnID, ctx.TurnQuality, ctx.Type = m.currentTurn, m.turnQuality, ConversationTurnStarted
		ctx.FinalAvailability = FinalUnknown
	case "turn.completed":
		ctx.Type, ctx.Status = ConversationTurnCompleted, "completed"
		ctx.FinalAvailability = FinalUnknown
		if m.turnFinalSeen[m.currentTurn] {
			ctx.FinalAvailability = FinalAvailable
		}
	case "turn.failed":
		ctx.Type, ctx.Status = ConversationTurnFailed, "failed"
		ctx.FinalAvailability = FinalUnknown
	case "turn.cancelled":
		ctx.Type, ctx.Status = ConversationTurnCancelled, "cancelled"
		ctx.FinalAvailability = FinalUnknown
	case "item.started", "item.updated", "item.completed":
		switch itemType {
		case "agent_message":
			text := stringField(item, "text")
			if text == "" {
				text = stringField(item, "message")
			}
			body = []byte(text)
			switch eventType {
			case "item.completed":
				ctx.Type, ctx.Status = ConversationMessageComplete, "completed"
				if ctx.Phase == MessagePhaseFinalAnswer {
					ctx.FinalAvailability = FinalAvailable
					if m.turnFinalSeen == nil {
						m.turnFinalSeen = map[string]bool{}
					}
					m.turnFinalSeen[ctx.TurnID] = true
				}
				if text != "" {
					bodyMode = "complete"
				}
			case "item.updated":
				ctx.Type, ctx.Status, bodyMode = ConversationMessageSnapshot, "updated", "snapshot"
			default:
				ctx.Type, ctx.Status = ConversationObservation, "started"
			}
		case "command_execution", "mcp_tool_call", "web_search_call":
			if eventType == "item.started" {
				ctx.Type, ctx.Status = ConversationToolStarted, "started"
			} else if eventType == "item.completed" {
				ctx.Type, ctx.Status = ConversationToolCompleted, stringField(item, "status")
				if ctx.Status == "" {
					ctx.Status = "completed"
				}
			} else {
				ctx.Type, ctx.Status = ConversationToolDelta, "updated"
			}
			ctx.Attributes["title"] = firstNonEmpty(stringField(item, "command"), stringField(item, "name"), itemType)
			if output := stringField(item, "aggregated_output"); output != "" {
				body, bodyMode = []byte(output), "snapshot"
			}
		default:
			ctx.Type, ctx.Status = ConversationUnknown, eventType
		}
	default:
		ctx.Type = ConversationUnknown
		ctx.Status = eventType
	}
	if ctx.Type == "" {
		ctx.Type = ConversationUnknown
	}
	write := ConversationWrite{Event: ctx, Body: body, BodyMode: bodyMode, Raw: append([]byte(nil), raw...), BodySource: "codex-jsonl"}
	return write
}

func (e *ConversationEvent) EventIDFallback(reason string) {
	if e.SourceEventID == "" {
		e.SourceEventID = reason + ":" + fmt.Sprint(e.SourceSeq)
	}
}

func (e ConversationEvent) itemIdentity() string {
	return strings.Join([]string{e.ProjectID, e.ExecutionID, e.AttemptID, e.SourceSegment, e.SessionID, e.TurnID, e.ItemID}, "\x00")
}

func conversationItemKey(e ConversationEvent, itemID string) string {
	if itemID == "" {
		return ""
	}
	e.ItemID = itemID
	sum := sha256.Sum256([]byte(e.itemIdentity()))
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

func (e ConversationEvent) sourceKey() string {
	identity := strings.Join([]string{e.ProjectID, e.ExecutionID, e.AttemptID, e.Source, e.SourceSegment, e.SourceEventID}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}

func (e ConversationEvent) validate() error {
	if e.Version != ConversationVersion || e.ProjectID == "" || e.ExecutionID == "" || e.AttemptID == "" || e.Source == "" || e.SourceSegment == "" || e.Type == "" || e.SourceEventID == "" {
		return errors.New("conversation event is missing versioned identity or source evidence")
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

func (m ConversationSourceMarker) validate() error {
	if m.Version != 1 || m.ProjectID == "" || m.WorkflowID == "" || m.ExecutionID == "" || m.AttemptID == "" || m.Backend != BackendLocal || m.Protocol != ProtocolLocalProcess || m.Agent == "" || m.TargetID == "" || m.Source != "codex-jsonl" || m.SourceSegment == "" || m.SourceIdentity == "" || m.SourceSegment != "local-jsonl-v1:"+m.SourceIdentity || m.RecordName != "events.jsonl" || m.CreatedAt.IsZero() {
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
