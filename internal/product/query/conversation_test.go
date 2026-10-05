package query

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	api "github.com/erranliu/SteerLine/internal/product/api"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type conversationEvents func(context.Context, string, api.ConversationFilter, int64) (*storageports.ConversationEventRecord, error)

func (f conversationEvents) NextConversationEvent(ctx context.Context, project string, filter api.ConversationFilter, after int64) (*storageports.ConversationEventRecord, error) {
	return f(ctx, project, filter, after)
}

type conversationBodies func(context.Context, string, api.ConversationBodyRef, int) (storageports.ConversationBodyRecord, error)

func (f conversationBodies) ConversationBody(ctx context.Context, project string, ref api.ConversationBodyRef, limit int) (storageports.ConversationBodyRecord, error) {
	return f(ctx, project, ref, limit)
}

type conversationRaw func(context.Context, string, api.ConversationRawRef, int) (storageports.ConversationRawRecord, error)

func (f conversationRaw) ConversationRaw(ctx context.Context, project string, ref api.ConversationRawRef, limit int) (storageports.ConversationRawRecord, error) {
	return f(ctx, project, ref, limit)
}

type conversationCapabilities func(context.Context, executionbackend.BackendKind, executionbackend.ExecutionProtocol, string, int) ([]api.ConversationCapabilityReport, error)

func (f conversationCapabilities) ConversationCapabilityReports(ctx context.Context, backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol, target string, limit int) ([]api.ConversationCapabilityReport, error) {
	return f(ctx, backend, protocol, target, limit)
}

func TestConversationQueriesUseIndependentEventPortAndBoundCursor(t *testing.T) {
	ctx := context.Background()
	filter := api.ConversationFilter{ExecutionID: "execution", AttemptID: "attempt", Source: "codex-app-server"}
	reads := 0
	service := ConversationService{Project: "project", Events: conversationEvents(func(_ context.Context, project string, got api.ConversationFilter, after int64) (*storageports.ConversationEventRecord, error) {
		reads++
		if project != "project" || got != api.ConversationFilter(filter) {
			t.Fatalf("lost query scope: %s %+v", project, got)
		}
		if after >= 2 {
			return nil, nil
		}
		payload, _ := json.Marshal(executionvalues.ConversationEvent{Version: executionvalues.ConversationVersion, ProjectID: project, ExecutionID: filter.ExecutionID, AttemptID: filter.AttemptID,
			Source: filter.Source, SourceRole: executionvalues.ConversationSourceFallback, SourceSegment: "segment", SourceEventID: fmt.Sprintf("event:%d", after+1), SessionQuality: executionvalues.IdentityUnknown, TurnQuality: executionvalues.IdentityUnknown,
			ItemQuality: executionvalues.IdentityUnknown, SourceEventQuality: executionvalues.IdentityDerived, Type: executionvalues.ConversationObservation, Phase: executionvalues.MessagePhaseUnknown, ReceivedAt: time.Now().UTC()})
		return &storageports.ConversationEventRecord{EventID: after + 1, PayloadBytes: len(payload), Payload: payload}, nil
	})}
	first, err := service.ReadConversation(ctx, filter, "", 1)
	if err != nil || len(first.Events) != 1 || first.Events[0].EventID != 1 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("event-only reader: %+v %v", first, err)
	}
	changed := filter
	changed.AttemptID = "another"
	before := reads
	_, err = service.ReadConversation(ctx, changed, first.NextCursor, 1)
	var cursor *CursorError
	if !errors.As(err, &cursor) || cursor.Code != "cursor_invalidated" || reads != before {
		t.Fatalf("changed filter reached storage: %v reads=%d/%d", err, reads, before)
	}
	last, err := service.ReadConversation(ctx, filter, first.NextCursor, 1)
	if err != nil || len(last.Events) != 1 || last.Events[0].EventID != 2 || last.HasMore {
		t.Fatalf("continuation: %+v %v", last, err)
	}
	if _, err := service.ReadMessageBody(ctx, api.ConversationBodyRef{}, 0, 10); err == nil {
		t.Fatal("missing body reader accepted")
	}
}

func TestConversationPageBudgetDefersOverflowPayloadDecoding(t *testing.T) {
	payload, _ := json.Marshal(executionvalues.ConversationEvent{Version: executionvalues.ConversationVersion, ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt",
		Source: "fixture", SourceRole: executionvalues.ConversationSourceFallback, SourceSegment: "segment", SourceEventID: "event:1", SessionQuality: executionvalues.IdentityUnknown, TurnQuality: executionvalues.IdentityUnknown,
		ItemQuality: executionvalues.IdentityUnknown, SourceEventQuality: executionvalues.IdentityDerived, Type: executionvalues.ConversationObservation, Phase: executionvalues.MessagePhaseUnknown,
		ReceivedAt: time.Now().UTC(), Diagnostic: strings.Repeat("x", MaxEventPageBytes-4096)})
	if len(payload) >= MaxEventPageBytes {
		t.Fatal("fixture must leave room for a budget boundary")
	}
	service := ConversationService{Project: "project", Events: conversationEvents(func(_ context.Context, _ string, _ api.ConversationFilter, after int64) (*storageports.ConversationEventRecord, error) {
		if after == 0 {
			return &storageports.ConversationEventRecord{EventID: 1, PayloadBytes: len(payload), Payload: payload}, nil
		}
		return &storageports.ConversationEventRecord{EventID: 2, PayloadBytes: MaxEventPageBytes, Payload: []byte("invalid JSON")}, nil
	})}
	page, err := service.ReadConversation(context.Background(), api.ConversationFilter{}, "", 10)
	if err != nil || len(page.Events) != 1 || !page.HasMore || page.PayloadBytes != len(payload) {
		t.Fatalf("overflow record was decoded or consumed: %+v %v", page, err)
	}
	if _, err := service.ReadConversation(context.Background(), api.ConversationFilter{}, page.NextCursor, 10); err == nil {
		t.Fatal("next page must still encounter the unconsumed corrupt record")
	}
}

func TestConversationReaderRejectsPreviousEventFormat(t *testing.T) {
	old, err := json.Marshal(executionvalues.ConversationEvent{Version: 1, ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Source: "fixture",
		SourceSegment: "segment", SourceEventID: "old-event", SessionQuality: executionvalues.IdentityUnknown, TurnQuality: executionvalues.IdentityUnknown, ItemQuality: executionvalues.IdentityUnknown,
		SourceEventQuality: executionvalues.IdentityDerived, Type: executionvalues.ConversationObservation, Phase: executionvalues.MessagePhaseUnknown, ReceivedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	service := ConversationService{Project: "project", Events: conversationEvents(func(context.Context, string, api.ConversationFilter, int64) (*storageports.ConversationEventRecord, error) {
		return &storageports.ConversationEventRecord{EventID: 1, PayloadBytes: len(old), Payload: old}, nil
	})}
	if _, err = service.ReadConversation(context.Background(), api.ConversationFilter{}, "", 1); err == nil || !strings.Contains(err.Error(), "unsupported schema") && !strings.Contains(err.Error(), "missing versioned identity") {
		t.Fatalf("previous conversation event format was read without migration: %v", err)
	}
}

func TestConversationReaderRejectsMissingSourceRole(t *testing.T) {
	payload, err := json.Marshal(executionvalues.ConversationEvent{Version: executionvalues.ConversationVersion, ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt",
		Source: "fixture", SourceSegment: "segment", SourceEventID: "event:1", SessionQuality: executionvalues.IdentityUnknown, TurnQuality: executionvalues.IdentityUnknown,
		ItemQuality: executionvalues.IdentityUnknown, SourceEventQuality: executionvalues.IdentityDerived, Type: executionvalues.ConversationObservation, Phase: executionvalues.MessagePhaseUnknown,
		ReceivedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	service := ConversationService{Project: "project", Events: conversationEvents(func(context.Context, string, api.ConversationFilter, int64) (*storageports.ConversationEventRecord, error) {
		return &storageports.ConversationEventRecord{EventID: 1, PayloadBytes: len(payload), Payload: payload}, nil
	})}
	if _, err = service.ReadConversation(context.Background(), api.ConversationFilter{}, "", 1); err == nil || !strings.Contains(err.Error(), "source role") {
		t.Fatalf("event without an explicit source role was accepted: %v", err)
	}
}

func TestConversationBodyAndRawQueriesUseSeparatePorts(t *testing.T) {
	ctx := context.Background()
	text := []byte("你好🙂tail")
	sum := sha256.Sum256(text)
	digest := hex.EncodeToString(sum[:])
	ref := api.ConversationBodyRef{ExecutionID: "execution", AttemptID: "attempt", ItemKey: "item", Revision: 2, SHA256: digest, Bytes: int64(len(text)), Completeness: api.BodyComplete}
	record := storageports.ConversationBodyRecord{Body: text, TotalBytes: ref.Bytes, SHA256: digest, Completeness: string(ref.Completeness)}
	service := ConversationService{Project: "project", Bodies: conversationBodies(func(_ context.Context, project string, got api.ConversationBodyRef, limit int) (storageports.ConversationBodyRecord, error) {
		if project != "project" || got != ref || limit != executionvalues.MaxConversationBodyBytes+1 {
			t.Fatalf("lost bounded revision input: %s %+v %d", project, got, limit)
		}
		return record, nil
	})}
	page, err := service.ReadMessageBody(ctx, ref, 0, 7)
	if err != nil || !utf8.Valid(page.Data) || string(page.Data) != "你好" || page.NextOffset != 6 {
		t.Fatalf("Unicode page changed: %+v %v", page, err)
	}
	if _, err := service.ReadMessageBody(ctx, ref, 1, 7); err == nil {
		t.Fatal("offset inside a codepoint was accepted")
	}
	record.Body = []byte("tampered")
	if _, err := service.ReadMessageBody(ctx, ref, 0, 7); err == nil {
		t.Fatal("corrupt revision was accepted")
	}
	rawRef := api.ConversationRawRef{ExecutionID: "execution", AttemptID: "attempt", ID: "raw", SHA256: digest, Bytes: int64(len(text))}
	service = ConversationService{Project: "project", Raw: conversationRaw(func(_ context.Context, project string, got api.ConversationRawRef, limit int) (storageports.ConversationRawRecord, error) {
		if project != "project" || got != rawRef || limit != executionvalues.MaxConversationBodyBytes+1 {
			t.Fatal("lost raw identity or bound")
		}
		return storageports.ConversationRawRecord{Data: text, TotalBytes: rawRef.Bytes, SHA256: digest}, nil
	})}
	raw, err := service.ReadRaw(ctx, rawRef, 3, 3)
	if err != nil || string(raw.Data) != "好" || raw.NextOffset != 6 {
		t.Fatalf("raw-only read failed: %+v %v", raw, err)
	}
}

func TestConversationCapabilitiesNeedNoUnrelatedReader(t *testing.T) {
	service := ConversationService{Capabilities: conversationCapabilities(func(_ context.Context, backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol, target string, limit int) ([]api.ConversationCapabilityReport, error) {
		if backend != executionbackend.BackendLocal || protocol != executionbackend.ProtocolLocalCodexAppServer || target != "local" || limit != 1000 {
			t.Fatal("capability filter or limit changed")
		}
		return []api.ConversationCapabilityReport{{SourceIdentity: "source"}}, nil
	})}
	reports, err := service.ReadCapabilities(context.Background(), executionbackend.BackendLocal, executionbackend.ProtocolLocalCodexAppServer, "local", 2000)
	if err != nil || len(reports) != 1 || reports[0].SourceIdentity != "source" {
		t.Fatalf("capability-only read failed: %+v %v", reports, err)
	}
}
