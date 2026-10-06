package codexworker

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type ConversationMapper struct {
	currentTurn    string
	turnQuality    executionvalues.IdentityQuality
	sessionID      string
	sessionQuality executionvalues.IdentityQuality
	turnFinalSeen  map[string]bool
}

func (m *ConversationMapper) MapRecord(event executionvalues.ConversationEvent, raw []byte) executionvalues.ConversationWrite {
	return m.MapCodexJSONL(event, raw)
}

// RestoreConversationMapper reconstructs ephemeral mapping state from the
// committed conversation projection. Callers discard the returned mapper when
// its corresponding batch fails to commit and restore it from the same
// committed snapshot on retry.
func RestoreConversationMapper(snapshot executionvalues.ConversationMapperSnapshot) *ConversationMapper {
	m := &ConversationMapper{}
	if snapshot.Session != nil {
		m.RestoreSession(*snapshot.Session)
	}
	if snapshot.Turn != nil {
		m.RestoreTurn(*snapshot.Turn, snapshot.FinalSeen)
	}
	return m
}

// MapCodexJSONL maps observed JSONL fields only. In particular, turn completion
// does not promote an item to final_answer, and an absent phase stays unknown.
func (m *ConversationMapper) MapCodexJSONL(ctx executionvalues.ConversationEvent, raw []byte) executionvalues.ConversationWrite {
	if m == nil {
		m = &ConversationMapper{}
	}
	ctx.Version = executionvalues.ConversationVersion
	if ctx.Backend == "" && ctx.Protocol == "" {
		ctx.Backend, ctx.Protocol = executionbackend.BackendLocal, executionbackend.ProtocolLocalProcess
	}
	ctx.Source = "codex-jsonl"
	ctx.SourceRole = executionvalues.ConversationSourceFallback
	ctx.SourceSegment = nonEmpty(ctx.SourceSegment, "local-jsonl-v1")
	ctx.SessionQuality = executionvalues.IdentityUnknown
	ctx.TurnQuality = executionvalues.IdentityUnknown
	ctx.SourceEventQuality = executionvalues.IdentityUnknown
	ctx.ItemQuality = executionvalues.IdentityUnknown
	ctx.Phase = executionvalues.MessagePhaseUnknown
	ctx.ReceivedAt = nonZeroTime(ctx.ReceivedAt, time.Now().UTC())
	ctx.Evidence = cloneStringMap(ctx.Evidence)
	ctx.Attributes = map[string]any{}
	if len(raw) > 0 {
		ctx.Raw = &executionvalues.ConversationRawRef{}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		ctx.Type = executionvalues.ConversationUnknown
		ctx.Status = "unparsed"
		ctx.Diagnostic = "source line was not a JSON object"
		ctx.EventIDFallback("raw-line")
		if strings.HasPrefix(ctx.SourceEventID, "bytes:") || strings.HasPrefix(ctx.SourceEventID, "seq:") {
			ctx.SourceEventQuality = executionvalues.IdentityDerived
		}
		return executionvalues.ConversationWrite{Event: ctx, Raw: append([]byte(nil), raw...)}
	}
	var eventType string
	_ = json.Unmarshal(fields["type"], &eventType)
	ctx.SourceEventID = nonEmpty(ctx.SourceEventID, sourceEventFallback(ctx, raw))
	if strings.HasPrefix(ctx.SourceEventID, "bytes:") || strings.HasPrefix(ctx.SourceEventID, "seq:") {
		ctx.SourceEventQuality = executionvalues.IdentityDerived
	}
	ctx.SourceSeq = nonZeroUint(ctx.SourceSeq, uintField(fields, "sequence"))
	ctx.SourceRevision = int64Field(fields, "revision")
	if turnID := stringField(fields, "turn_id"); turnID != "" {
		m.currentTurn, m.turnQuality = turnID, executionvalues.IdentityNative
	}
	if sessionID := stringField(fields, "thread_id"); sessionID != "" {
		m.sessionID, m.sessionQuality = sessionID, executionvalues.IdentityNative
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
		ctx.ItemQuality = executionvalues.IdentityNative
	}
	ctx.ItemKey = conversationItemKey(ctx, itemID)
	ctx.Attributes["source_type"] = eventType
	if itemType != "" {
		ctx.Attributes["item_type"] = itemType
	}
	phase := nonEmpty(stringField(item, "phase"), stringField(fields, "phase"))
	if phase == string(executionvalues.MessagePhaseCommentary) || phase == string(executionvalues.MessagePhaseFinalAnswer) {
		ctx.Phase = executionvalues.MessagePhase(phase)
	}
	var body []byte
	bodyMode := ""
	switch eventType {
	case "thread.started":
		ctx.Type = executionvalues.ConversationSession
		ctx.SessionID = nonEmpty(m.sessionID, stringField(fields, "thread_id"))
		if ctx.SessionID != "" {
			ctx.SessionQuality = executionvalues.IdentityNative
			m.sessionID, m.sessionQuality = ctx.SessionID, executionvalues.IdentityNative
		}
	case "turn.started":
		if id := stringField(fields, "turn_id"); id != "" {
			m.currentTurn, m.turnQuality = id, executionvalues.IdentityNative
		} else {
			// The persisted source byte range identifies this explicit boundary.
			m.currentTurn = "derived:" + ctx.SourceEventID
			m.turnQuality = executionvalues.IdentityDerived
			ctx.Evidence["turn_derivation"] = "codex-jsonl-turn-boundary-v1"
		}
		if m.turnFinalSeen == nil {
			m.turnFinalSeen = map[string]bool{}
		}
		ctx.TurnID, ctx.TurnQuality, ctx.Type = m.currentTurn, m.turnQuality, executionvalues.ConversationTurnStarted
		ctx.FinalAvailability = executionvalues.FinalUnknown
	case "turn.completed":
		ctx.Type, ctx.Status = executionvalues.ConversationTurnCompleted, "completed"
		ctx.FinalAvailability = executionvalues.FinalUnknown
		if m.turnFinalSeen[m.currentTurn] {
			ctx.FinalAvailability = executionvalues.FinalAvailable
		}
	case "turn.failed":
		ctx.Type, ctx.Status = executionvalues.ConversationTurnFailed, "failed"
		ctx.FinalAvailability = executionvalues.FinalUnknown
	case "turn.cancelled":
		ctx.Type, ctx.Status = executionvalues.ConversationTurnCancelled, "cancelled"
		ctx.FinalAvailability = executionvalues.FinalUnknown
	case "item.started", "item.updated", "item.completed":
		switch itemType {
		case "agent_message":
			text, textPresent := stringFieldPresent(item, "text")
			if !textPresent {
				text, textPresent = stringFieldPresent(item, "message")
			}
			body = []byte(text)
			switch eventType {
			case "item.completed":
				ctx.Type, ctx.Status = executionvalues.ConversationMessageComplete, "completed"
				if ctx.Phase == executionvalues.MessagePhaseFinalAnswer {
					ctx.FinalAvailability = executionvalues.FinalUnknown
					if textPresent {
						ctx.FinalAvailability = executionvalues.FinalAvailable
						if m.turnFinalSeen == nil {
							m.turnFinalSeen = map[string]bool{}
						}
						m.turnFinalSeen[ctx.TurnID] = true
					}
				}
				if textPresent {
					bodyMode = "complete"
				}
			case "item.updated":
				ctx.Type, ctx.Status = executionvalues.ConversationMessageSnapshot, "updated"
				if textPresent {
					bodyMode = "snapshot"
				}
			default:
				ctx.Type, ctx.Status = executionvalues.ConversationObservation, "started"
			}
		case "command_execution", "mcp_tool_call", "web_search_call":
			if eventType == "item.started" {
				ctx.Type, ctx.Status = executionvalues.ConversationToolStarted, "started"
			} else if eventType == "item.completed" {
				ctx.Type, ctx.Status = executionvalues.ConversationToolCompleted, stringField(item, "status")
				if ctx.Status == "" {
					ctx.Status = "completed"
				}
			} else {
				ctx.Type, ctx.Status = executionvalues.ConversationToolDelta, "updated"
			}
			ctx.Attributes["title"] = firstNonEmpty(stringField(item, "command"), stringField(item, "name"), itemType)
			if output := stringField(item, "aggregated_output"); output != "" {
				body, bodyMode = []byte(output), "snapshot"
			}
		default:
			ctx.Type, ctx.Status = executionvalues.ConversationUnknown, eventType
		}
	default:
		ctx.Type = executionvalues.ConversationUnknown
		ctx.Status = eventType
	}
	if ctx.Type == "" {
		ctx.Type = executionvalues.ConversationUnknown
	}
	write := executionvalues.ConversationWrite{Event: ctx, Body: body, BodyMode: bodyMode, Raw: append([]byte(nil), raw...), BodySource: "codex-jsonl"}
	return write
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

func qualityOrUnknown(value executionvalues.IdentityQuality) executionvalues.IdentityQuality {
	if value == "" {
		return executionvalues.IdentityUnknown
	}
	return value
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

func sourceEventFallback(e executionvalues.ConversationEvent, raw []byte) string {
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
	return "unidentified:" + newSourceID()
}

func newSourceID() string { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }

func (m *ConversationMapper) CurrentTurn() string { return m.currentTurn }

func (m *ConversationMapper) RestoreSession(e executionvalues.ConversationEvent) {
	m.sessionID, m.sessionQuality = e.SessionID, e.SessionQuality
}

func (m *ConversationMapper) RestoreTurn(e executionvalues.ConversationEvent, finalSeen bool) {
	m.currentTurn, m.turnQuality = e.TurnID, e.TurnQuality
	if m.turnFinalSeen == nil {
		m.turnFinalSeen = map[string]bool{}
	}
	m.turnFinalSeen[e.TurnID] = finalSeen
}

func (m *ConversationMapper) Clone() *ConversationMapper {
	copy := *m
	if m.turnFinalSeen != nil {
		copy.turnFinalSeen = map[string]bool{}
		for k, v := range m.turnFinalSeen {
			copy.turnFinalSeen[k] = v
		}
	}
	return &copy
}
