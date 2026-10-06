package host

import (
	"context"
	"errors"
	"fmt"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

const neutralJournalSource = "fixture-journal"

type neutralJournalPort struct{ neutralConversationSources }

type neutralJournalMarker struct{ ProjectID, WorkflowID, ExecutionID, AttemptID, SourceIdentity string }

type neutralRecordMapper struct {
	snapshot executionvalues.ConversationMapperSnapshot
}

func (p neutralJournalPort) RestoreMapper(s executionvalues.ConversationMapperSnapshot) conversationtypes.ConversationMapper {
	return &neutralRecordMapper{snapshot: s}
}

func (p neutralJournalPort) JournalSources() []string { return []string{neutralJournalSource} }

func (p neutralJournalPort) JournalSource(executionbackend.BackendKind, executionbackend.ExecutionProtocol) string {
	return neutralJournalSource
}

func (p neutralJournalPort) JournalSegment(_ executionbackend.BackendKind, _ executionbackend.ExecutionProtocol, id string) string {
	return "fixture-journal-v1:" + id
}

func (p neutralJournalPort) JournalGeneration(id, segment string) (uint64, error) {
	prefix := "fixture-journal-v1:" + id
	if segment == prefix {
		return 1, nil
	}
	if strings.HasPrefix(segment, prefix+":") {
		return strconv.ParseUint(strings.TrimPrefix(segment, prefix+":"), 10, 64)
	}
	return 0, errors.New("fixture source generation differs")
}

func (p neutralJournalPort) LiveJournalPath(_ context.Context, r conversationtypes.ConversationJournalPathRequest) (string, error) {
	if r.LogPath != "" {
		return r.LogPath, nil
	}
	if len(r.EventPaths) != 0 {
		return r.EventPaths[0], nil
	}
	return "", errors.New("fixture source path unavailable")
}

func (p neutralJournalPort) MapConversation(input executionvalues.ConversationInput) (executionvalues.ConversationMappedBatch, error) {
	event := input.Event
	event.ReceivedAt = now()
	event.Phase = executionvalues.MessagePhaseUnknown
	event.Version, event.Source, event.SourceRole = executionvalues.ConversationVersion, neutralJournalSource, executionvalues.ConversationSourceFallback
	event.SourceIdentity, event.SourceSegment = input.Identity, input.Segment
	if event.SourceSegment == "" {
		event.SourceSegment = p.JournalSegment(event.Backend, event.Protocol, input.Identity)
	}
	event.SourceSeq, event.SourceEventID, event.SourceEventQuality = input.SourceSeq, fmt.Sprintf("bytes:%d:%d", input.SourceOffset, input.SourceOffset+input.SourceLength), executionvalues.IdentityDerived
	cursor := string(mustJSON(struct {
		ByteOffset int64  `json:"byte_offset"`
		ByteLength int64  `json:"byte_length"`
		SourceSeq  uint64 `json:"source_seq"`
	}{input.SourceOffset, input.SourceLength, input.SourceSeq}))
	event.SourceCursor = cursor
	write := p.RestoreMapper(input.Mapper).MapRecord(event, input.Raw)
	cp := executionvalues.ConversationCheckpoint{ProjectID: event.ProjectID, ExecutionID: event.ExecutionID, AttemptID: event.AttemptID, Source: event.Source, SourceSegment: event.SourceSegment, SourceIdentity: input.Identity, Generation: input.Identity, Cursor: cursor, LocalSeq: input.SourceSeq}
	return executionvalues.ConversationMappedBatch{Writes: []executionvalues.ConversationWrite{write}, Checkpoint: cp, VerifySourcePosition: true}, nil
}

func (m *neutralRecordMapper) MapRecord(e executionvalues.ConversationEvent, data []byte) executionvalues.ConversationWrite {
	var record struct {
		Kind    string `json:"kind"`
		Session string `json:"session,omitempty"`
		ID      string `json:"id,omitempty"`
		Text    string `json:"text,omitempty"`
	}
	w := executionvalues.ConversationWrite{Raw: append([]byte(nil), data...)}
	if err := decode(data, &record); err != nil {
		e.Type, e.Status, e.Diagnostic = executionvalues.ConversationGap, "unparsed", err.Error()
		w.Event = e
		return w
	}
	e.SessionQuality, e.TurnQuality, e.ItemQuality = executionvalues.IdentityUnknown, executionvalues.IdentityUnknown, executionvalues.IdentityUnknown
	if m.snapshot.Session != nil {
		e.SessionID, e.SessionQuality = m.snapshot.Session.SessionID, m.snapshot.Session.SessionQuality
	}
	if m.snapshot.Turn != nil {
		e.TurnID, e.TurnQuality = m.snapshot.Turn.TurnID, m.snapshot.Turn.TurnQuality
	}
	switch record.Kind {
	case "session":
		e.Type, e.SessionID, e.SessionQuality = executionvalues.ConversationSession, record.Session, executionvalues.IdentityNative
		m.snapshot.Session = &e
	case "turn":
		e.Type, e.TurnID, e.TurnQuality = executionvalues.ConversationTurnStarted, "fixture-turn:"+e.SourceEventID, executionvalues.IdentityDerived
		m.snapshot.Turn = &e
	case "message":
		e.Type, e.ItemID, e.ItemKey, e.ItemQuality = executionvalues.ConversationMessageComplete, record.ID, record.ID, executionvalues.IdentityNative
		w.Body, w.BodyMode = []byte(record.Text), "complete"
	case "completed":
		e.Type = executionvalues.ConversationTurnCompleted
	default:
		e.Type, e.Status = executionvalues.ConversationGap, "unknown"
	}
	w.Event = e
	return w
}

func (p neutralJournalPort) DiscoverJournals(_ context.Context, d conversationtypes.ConversationDiscovery, identify func(string) (string, error)) ([]conversationtypes.ConversationJournalSource, error) {
	var sources []conversationtypes.ConversationJournalSource
	for _, run := range d.Runs {
		root := run.RunDirectory
		raw, err := os.ReadFile(filepath.Join(root, "fixture-source.json"))
		if err != nil {
			continue
		}
		var marker neutralJournalMarker
		if decode(raw, &marker) != nil || marker.ProjectID != d.ProjectID || marker.ExecutionID != run.Base.ExecutionID {
			continue
		}
		path := filepath.Join(root, "events.jsonl")
		id, err := identify(path)
		if err != nil || id != marker.SourceIdentity {
			continue
		}
		event := run.Base
		event.Source, event.SourceRole, event.SourceSegment, event.SourceIdentity = neutralJournalSource, executionvalues.ConversationSourceFallback, p.JournalSegment(event.Backend, event.Protocol, id), id
		event.AttemptID = marker.AttemptID
		sources = append(sources, conversationtypes.ConversationJournalSource{Path: path, AttemptID: marker.AttemptID, FileID: id, ExpectedFileID: id, Event: event, Ended: true})
	}
	return sources, nil
}
