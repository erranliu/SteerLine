package execution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

const neutralJournalSource = "fixture-journal"

type neutralJournalPort struct{ neutralConversationSources }

type neutralJournalMarker struct{ ProjectID, WorkflowID, ExecutionID, AttemptID, SourceIdentity string }

type neutralRecordMapper struct {
	snapshot contracts.ConversationMapperSnapshot
}

func (p neutralJournalPort) RestoreMapper(s contracts.ConversationMapperSnapshot) contracts.ConversationMapper {
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

func (p neutralJournalPort) LiveJournalPath(_ context.Context, r contracts.ConversationJournalPathRequest) (string, error) {
	if r.LogPath != "" {
		return r.LogPath, nil
	}
	if len(r.EventPaths) != 0 {
		return r.EventPaths[0], nil
	}
	return "", errors.New("fixture source path unavailable")
}

func (p neutralJournalPort) MapConversation(input contracts.ConversationInput) (contracts.ConversationMappedBatch, error) {
	event := input.Event
	event.ReceivedAt = now()
	event.Phase = contracts.MessagePhaseUnknown
	event.Version, event.Source, event.SourceRole = contracts.ConversationVersion, neutralJournalSource, contracts.ConversationSourceFallback
	event.SourceIdentity, event.SourceSegment = input.Identity, input.Segment
	if event.SourceSegment == "" {
		event.SourceSegment = p.JournalSegment(event.Backend, event.Protocol, input.Identity)
	}
	event.SourceSeq, event.SourceEventID, event.SourceEventQuality = input.SourceSeq, fmt.Sprintf("bytes:%d:%d", input.SourceOffset, input.SourceOffset+input.SourceLength), contracts.IdentityDerived
	cursor := string(mustJSON(struct {
		ByteOffset int64  `json:"byte_offset"`
		ByteLength int64  `json:"byte_length"`
		SourceSeq  uint64 `json:"source_seq"`
	}{input.SourceOffset, input.SourceLength, input.SourceSeq}))
	event.SourceCursor = cursor
	write := p.RestoreMapper(input.Mapper).MapRecord(event, input.Raw)
	cp := contracts.ConversationCheckpoint{ProjectID: event.ProjectID, ExecutionID: event.ExecutionID, AttemptID: event.AttemptID, Source: event.Source, SourceSegment: event.SourceSegment, SourceIdentity: input.Identity, Generation: input.Identity, Cursor: cursor, LocalSeq: input.SourceSeq}
	return contracts.ConversationMappedBatch{Writes: []contracts.ConversationWrite{write}, Checkpoint: cp, VerifySourcePosition: true}, nil
}

func (m *neutralRecordMapper) MapRecord(e contracts.ConversationEvent, data []byte) contracts.ConversationWrite {
	var record struct {
		Kind    string `json:"kind"`
		Session string `json:"session,omitempty"`
		ID      string `json:"id,omitempty"`
		Text    string `json:"text,omitempty"`
	}
	w := contracts.ConversationWrite{Raw: append([]byte(nil), data...)}
	if err := decode(data, &record); err != nil {
		e.Type, e.Status, e.Diagnostic = contracts.ConversationGap, "unparsed", err.Error()
		w.Event = e
		return w
	}
	e.SessionQuality, e.TurnQuality, e.ItemQuality = contracts.IdentityUnknown, contracts.IdentityUnknown, contracts.IdentityUnknown
	if m.snapshot.Session != nil {
		e.SessionID, e.SessionQuality = m.snapshot.Session.SessionID, m.snapshot.Session.SessionQuality
	}
	if m.snapshot.Turn != nil {
		e.TurnID, e.TurnQuality = m.snapshot.Turn.TurnID, m.snapshot.Turn.TurnQuality
	}
	switch record.Kind {
	case "session":
		e.Type, e.SessionID, e.SessionQuality = contracts.ConversationSession, record.Session, contracts.IdentityNative
		m.snapshot.Session = &e
	case "turn":
		e.Type, e.TurnID, e.TurnQuality = contracts.ConversationTurnStarted, "fixture-turn:"+e.SourceEventID, contracts.IdentityDerived
		m.snapshot.Turn = &e
	case "message":
		e.Type, e.ItemID, e.ItemKey, e.ItemQuality = contracts.ConversationMessageComplete, record.ID, record.ID, contracts.IdentityNative
		w.Body, w.BodyMode = []byte(record.Text), "complete"
	case "completed":
		e.Type = contracts.ConversationTurnCompleted
	default:
		e.Type, e.Status = contracts.ConversationGap, "unknown"
	}
	w.Event = e
	return w
}

func (p neutralJournalPort) DiscoverJournals(_ context.Context, d contracts.ConversationDiscovery, identify func(string) (string, error)) ([]contracts.ConversationJournalSource, error) {
	var sources []contracts.ConversationJournalSource
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
		event.Source, event.SourceRole, event.SourceSegment, event.SourceIdentity = neutralJournalSource, contracts.ConversationSourceFallback, p.JournalSegment(event.Backend, event.Protocol, id), id
		event.AttemptID = marker.AttemptID
		sources = append(sources, contracts.ConversationJournalSource{Path: path, AttemptID: marker.AttemptID, FileID: id, ExpectedFileID: id, Event: event, Ended: true})
	}
	return sources, nil
}
