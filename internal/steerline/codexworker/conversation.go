package codexworker

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type ConversationMapper struct {
	currentTurn    string
	turnQuality    contracts.IdentityQuality
	sessionID      string
	sessionQuality contracts.IdentityQuality
	turnFinalSeen  map[string]bool
}

func (m *ConversationMapper) MapRecord(event contracts.ConversationEvent, raw []byte) contracts.ConversationWrite {
	return m.MapCodexJSONL(event, raw)
}

// RestoreConversationMapper reconstructs ephemeral mapping state from the
// committed conversation projection. Callers discard the returned mapper when
// its corresponding batch fails to commit and restore it from the same
// committed snapshot on retry.
func RestoreConversationMapper(snapshot contracts.ConversationMapperSnapshot) *ConversationMapper {
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
func (m *ConversationMapper) MapCodexJSONL(ctx contracts.ConversationEvent, raw []byte) contracts.ConversationWrite {
	if m == nil {
		m = &ConversationMapper{}
	}
	ctx.Version = contracts.ConversationVersion
	ctx.Backend, ctx.Protocol = executionapi.BackendLocal, executionapi.ProtocolLocalProcess
	ctx.Source = "codex-jsonl"
	ctx.SourceRole = contracts.ConversationSourceFallback
	ctx.SourceSegment = nonEmpty(ctx.SourceSegment, "local-jsonl-v1")
	ctx.SessionQuality = contracts.IdentityUnknown
	ctx.TurnQuality = contracts.IdentityUnknown
	ctx.SourceEventQuality = contracts.IdentityUnknown
	ctx.ItemQuality = contracts.IdentityUnknown
	ctx.Phase = contracts.MessagePhaseUnknown
	ctx.ReceivedAt = nonZeroTime(ctx.ReceivedAt, time.Now().UTC())
	ctx.Evidence = cloneStringMap(ctx.Evidence)
	ctx.Attributes = map[string]any{}
	if len(raw) > 0 {
		ctx.Raw = &contracts.ConversationRawRef{}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		ctx.Type = contracts.ConversationUnknown
		ctx.Status = "unparsed"
		ctx.Diagnostic = "source line was not a JSON object"
		ctx.EventIDFallback("raw-line")
		if strings.HasPrefix(ctx.SourceEventID, "bytes:") || strings.HasPrefix(ctx.SourceEventID, "seq:") {
			ctx.SourceEventQuality = contracts.IdentityDerived
		}
		return contracts.ConversationWrite{Event: ctx, Raw: append([]byte(nil), raw...)}
	}
	var eventType string
	_ = json.Unmarshal(fields["type"], &eventType)
	ctx.SourceEventID = nonEmpty(ctx.SourceEventID, sourceEventFallback(ctx, raw))
	if strings.HasPrefix(ctx.SourceEventID, "bytes:") || strings.HasPrefix(ctx.SourceEventID, "seq:") {
		ctx.SourceEventQuality = contracts.IdentityDerived
	}
	ctx.SourceSeq = nonZeroUint(ctx.SourceSeq, uintField(fields, "sequence"))
	ctx.SourceRevision = int64Field(fields, "revision")
	if turnID := stringField(fields, "turn_id"); turnID != "" {
		m.currentTurn, m.turnQuality = turnID, contracts.IdentityNative
	}
	if sessionID := stringField(fields, "thread_id"); sessionID != "" {
		m.sessionID, m.sessionQuality = sessionID, contracts.IdentityNative
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
		ctx.ItemQuality = contracts.IdentityNative
	}
	ctx.ItemKey = conversationItemKey(ctx, itemID)
	ctx.Attributes["source_type"] = eventType
	if itemType != "" {
		ctx.Attributes["item_type"] = itemType
	}
	phase := nonEmpty(stringField(item, "phase"), stringField(fields, "phase"))
	if phase == string(contracts.MessagePhaseCommentary) || phase == string(contracts.MessagePhaseFinalAnswer) {
		ctx.Phase = contracts.MessagePhase(phase)
	}
	var body []byte
	bodyMode := ""
	switch eventType {
	case "thread.started":
		ctx.Type = contracts.ConversationSession
		ctx.SessionID = nonEmpty(m.sessionID, stringField(fields, "thread_id"))
		if ctx.SessionID != "" {
			ctx.SessionQuality = contracts.IdentityNative
			m.sessionID, m.sessionQuality = ctx.SessionID, contracts.IdentityNative
		}
	case "turn.started":
		if id := stringField(fields, "turn_id"); id != "" {
			m.currentTurn, m.turnQuality = id, contracts.IdentityNative
		} else {
			// The persisted source byte range identifies this explicit boundary.
			m.currentTurn = "derived:" + ctx.SourceEventID
			m.turnQuality = contracts.IdentityDerived
			ctx.Evidence["turn_derivation"] = "codex-jsonl-turn-boundary-v1"
		}
		if m.turnFinalSeen == nil {
			m.turnFinalSeen = map[string]bool{}
		}
		ctx.TurnID, ctx.TurnQuality, ctx.Type = m.currentTurn, m.turnQuality, contracts.ConversationTurnStarted
		ctx.FinalAvailability = contracts.FinalUnknown
	case "turn.completed":
		ctx.Type, ctx.Status = contracts.ConversationTurnCompleted, "completed"
		ctx.FinalAvailability = contracts.FinalUnknown
		if m.turnFinalSeen[m.currentTurn] {
			ctx.FinalAvailability = contracts.FinalAvailable
		}
	case "turn.failed":
		ctx.Type, ctx.Status = contracts.ConversationTurnFailed, "failed"
		ctx.FinalAvailability = contracts.FinalUnknown
	case "turn.cancelled":
		ctx.Type, ctx.Status = contracts.ConversationTurnCancelled, "cancelled"
		ctx.FinalAvailability = contracts.FinalUnknown
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
				ctx.Type, ctx.Status = contracts.ConversationMessageComplete, "completed"
				if ctx.Phase == contracts.MessagePhaseFinalAnswer {
					ctx.FinalAvailability = contracts.FinalUnknown
					if textPresent {
						ctx.FinalAvailability = contracts.FinalAvailable
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
				ctx.Type, ctx.Status = contracts.ConversationMessageSnapshot, "updated"
				if textPresent {
					bodyMode = "snapshot"
				}
			default:
				ctx.Type, ctx.Status = contracts.ConversationObservation, "started"
			}
		case "command_execution", "mcp_tool_call", "web_search_call":
			if eventType == "item.started" {
				ctx.Type, ctx.Status = contracts.ConversationToolStarted, "started"
			} else if eventType == "item.completed" {
				ctx.Type, ctx.Status = contracts.ConversationToolCompleted, stringField(item, "status")
				if ctx.Status == "" {
					ctx.Status = "completed"
				}
			} else {
				ctx.Type, ctx.Status = contracts.ConversationToolDelta, "updated"
			}
			ctx.Attributes["title"] = firstNonEmpty(stringField(item, "command"), stringField(item, "name"), itemType)
			if output := stringField(item, "aggregated_output"); output != "" {
				body, bodyMode = []byte(output), "snapshot"
			}
		default:
			ctx.Type, ctx.Status = contracts.ConversationUnknown, eventType
		}
	default:
		ctx.Type = contracts.ConversationUnknown
		ctx.Status = eventType
	}
	if ctx.Type == "" {
		ctx.Type = contracts.ConversationUnknown
	}
	write := contracts.ConversationWrite{Event: ctx, Body: body, BodyMode: bodyMode, Raw: append([]byte(nil), raw...), BodySource: "codex-jsonl"}
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

func qualityOrUnknown(value contracts.IdentityQuality) contracts.IdentityQuality {
	if value == "" {
		return contracts.IdentityUnknown
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
func sourceEventFallback(e contracts.ConversationEvent, raw []byte) string {
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

func newSourceID() string                         { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func (m *ConversationMapper) CurrentTurn() string { return m.currentTurn }
func (m *ConversationMapper) RestoreSession(e contracts.ConversationEvent) {
	m.sessionID, m.sessionQuality = e.SessionID, e.SessionQuality
}
func (m *ConversationMapper) RestoreTurn(e contracts.ConversationEvent, finalSeen bool) {
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
