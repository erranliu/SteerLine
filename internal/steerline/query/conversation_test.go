package query

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type conversationEvents func(context.Context, string, contracts.ConversationFilter, int64) (*contracts.ConversationEventRecord, error)

func (f conversationEvents) NextConversationEvent(ctx context.Context, project string, filter contracts.ConversationFilter, after int64) (*contracts.ConversationEventRecord, error) {
	return f(ctx, project, filter, after)
}

type conversationBodies func(context.Context, string, contracts.ConversationBodyRef, int) (contracts.ConversationBodyRecord, error)

func (f conversationBodies) ConversationBody(ctx context.Context, project string, ref contracts.ConversationBodyRef, limit int) (contracts.ConversationBodyRecord, error) {
	return f(ctx, project, ref, limit)
}

type conversationRaw func(context.Context, string, contracts.ConversationRawRef, int) (contracts.ConversationRawRecord, error)

func (f conversationRaw) ConversationRaw(ctx context.Context, project string, ref contracts.ConversationRawRef, limit int) (contracts.ConversationRawRecord, error) {
	return f(ctx, project, ref, limit)
}

type conversationCapabilities func(context.Context, executionapi.BackendKind, executionapi.ExecutionProtocol, string, int) ([]contracts.ConversationCapabilityReport, error)

func (f conversationCapabilities) ConversationCapabilityReports(ctx context.Context, backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol, target string, limit int) ([]contracts.ConversationCapabilityReport, error) {
	return f(ctx, backend, protocol, target, limit)
}

func TestConversationQueriesUseIndependentEventPortAndBoundCursor(t *testing.T) {
	ctx := context.Background()
	filter := contracts.ConversationFilter{ExecutionID: "execution", AttemptID: "attempt", Source: "codex-app-server"}
	reads := 0
	service := ConversationService{Project: "project", Events: conversationEvents(func(_ context.Context, project string, got contracts.ConversationFilter, after int64) (*contracts.ConversationEventRecord, error) {
		reads++
		if project != "project" || got != filter {
			t.Fatalf("lost query scope: %s %+v", project, got)
		}
		if after >= 2 {
			return nil, nil
		}
		payload, _ := json.Marshal(contracts.ConversationEvent{ProjectID: project, ExecutionID: filter.ExecutionID, AttemptID: filter.AttemptID})
		return &contracts.ConversationEventRecord{EventID: after + 1, PayloadBytes: len(payload), Payload: payload}, nil
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
	if _, err := service.ReadMessageBody(ctx, contracts.ConversationBodyRef{}, 0, 10); err == nil {
		t.Fatal("missing body reader accepted")
	}
}

func TestConversationPageBudgetDefersOverflowPayloadDecoding(t *testing.T) {
	payload, _ := json.Marshal(contracts.ConversationEvent{Attributes: map[string]any{"text": strings.Repeat("x", MaxEventPageBytes-4096)}})
	if len(payload) >= MaxEventPageBytes {
		t.Fatal("fixture must leave room for a budget boundary")
	}
	service := ConversationService{Project: "project", Events: conversationEvents(func(_ context.Context, _ string, _ contracts.ConversationFilter, after int64) (*contracts.ConversationEventRecord, error) {
		if after == 0 {
			return &contracts.ConversationEventRecord{EventID: 1, PayloadBytes: len(payload), Payload: payload}, nil
		}
		return &contracts.ConversationEventRecord{EventID: 2, PayloadBytes: MaxEventPageBytes, Payload: []byte("invalid JSON")}, nil
	})}
	page, err := service.ReadConversation(context.Background(), contracts.ConversationFilter{}, "", 10)
	if err != nil || len(page.Events) != 1 || !page.HasMore || page.PayloadBytes != len(payload) {
		t.Fatalf("overflow record was decoded or consumed: %+v %v", page, err)
	}
	if _, err := service.ReadConversation(context.Background(), contracts.ConversationFilter{}, page.NextCursor, 10); err == nil {
		t.Fatal("next page must still encounter the unconsumed corrupt record")
	}
}

func TestConversationBodyAndRawQueriesUseSeparatePorts(t *testing.T) {
	ctx := context.Background()
	text := []byte("你好🙂tail")
	sum := sha256.Sum256(text)
	digest := hex.EncodeToString(sum[:])
	ref := contracts.ConversationBodyRef{ExecutionID: "execution", AttemptID: "attempt", ItemKey: "item", Revision: 2, SHA256: digest, Bytes: int64(len(text)), Completeness: contracts.BodyComplete}
	record := contracts.ConversationBodyRecord{Body: text, TotalBytes: ref.Bytes, SHA256: digest, Completeness: ref.Completeness}
	service := ConversationService{Project: "project", Bodies: conversationBodies(func(_ context.Context, project string, got contracts.ConversationBodyRef, limit int) (contracts.ConversationBodyRecord, error) {
		if project != "project" || got != ref || limit != contracts.MaxConversationBodyBytes+1 {
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
	rawRef := contracts.ConversationRawRef{ExecutionID: "execution", AttemptID: "attempt", ID: "raw", SHA256: digest, Bytes: int64(len(text))}
	service = ConversationService{Project: "project", Raw: conversationRaw(func(_ context.Context, project string, got contracts.ConversationRawRef, limit int) (contracts.ConversationRawRecord, error) {
		if project != "project" || got != rawRef || limit != contracts.MaxConversationBodyBytes+1 {
			t.Fatal("lost raw identity or bound")
		}
		return contracts.ConversationRawRecord{Data: text, TotalBytes: rawRef.Bytes, SHA256: digest}, nil
	})}
	raw, err := service.ReadRaw(ctx, rawRef, 3, 3)
	if err != nil || string(raw.Data) != "好" || raw.NextOffset != 6 {
		t.Fatalf("raw-only read failed: %+v %v", raw, err)
	}
}

func TestConversationCapabilitiesNeedNoUnrelatedReader(t *testing.T) {
	service := ConversationService{Capabilities: conversationCapabilities(func(_ context.Context, backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol, target string, limit int) ([]contracts.ConversationCapabilityReport, error) {
		if backend != executionapi.BackendLocal || protocol != executionapi.ProtocolLocalCodexAppServer || target != "local" || limit != 1000 {
			t.Fatal("capability filter or limit changed")
		}
		return []contracts.ConversationCapabilityReport{{SourceIdentity: "source"}}, nil
	})}
	reports, err := service.ReadCapabilities(context.Background(), executionapi.BackendLocal, executionapi.ProtocolLocalCodexAppServer, "local", 2000)
	if err != nil || len(reports) != 1 || reports[0].SourceIdentity != "source" {
		t.Fatalf("capability-only read failed: %+v %v", reports, err)
	}
}
