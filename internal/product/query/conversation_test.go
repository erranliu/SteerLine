package query

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type conversationEvents func(context.Context, string, storageports.ConversationFilter, int64) (*storageports.ConversationEventRecord, error)

func (f conversationEvents) NextConversationEvent(ctx context.Context, project string, filter storageports.ConversationFilter, after int64) (*storageports.ConversationEventRecord, error) {
	return f(ctx, project, filter, after)
}

type conversationBodies func(context.Context, string, executionvalues.ConversationBodyRef, int) (storageports.ConversationBodyRecord, error)

func (f conversationBodies) ConversationBody(ctx context.Context, project string, ref executionvalues.ConversationBodyRef, limit int) (storageports.ConversationBodyRecord, error) {
	return f(ctx, project, ref, limit)
}

type conversationCapabilities func(context.Context, executionbackend.BackendKind, executionbackend.ExecutionProtocol, string, int) ([]executionvalues.ConversationCapabilityReport, error)

func (f conversationCapabilities) ConversationCapabilityReports(ctx context.Context, backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol, target string, limit int) ([]executionvalues.ConversationCapabilityReport, error) {
	return f(ctx, backend, protocol, target, limit)
}

func finalRecord(seq int64) *storageports.ConversationEventRecord {
	return &storageports.ConversationEventRecord{EventID: seq, ProjectID: "project", CandidateID: "candidate", TaskID: "task", RunID: "execution", Event: runnercontract.Event{EventID: fmt.Sprintf("event:%d", seq), RunID: "execution", AttemptID: "attempt", CreatedAt: time.Now().UTC(), Final: &runnercontract.Final{Verified: true, AttemptID: "attempt", SessionID: "session", TurnID: "turn", ItemID: "item", Source: "codex-app-server", Text: "你好🙂tail"}}}
}

func TestConversationQueriesUseIndependentEventPortAndBoundCursor(t *testing.T) {
	ctx := context.Background()
	filter := api.ConversationFilter{ExecutionID: "execution", AttemptID: "attempt", Source: "codex-app-server"}
	reads := 0
	service := ConversationService{Project: "project", Events: conversationEvents(func(_ context.Context, project string, got storageports.ConversationFilter, after int64) (*storageports.ConversationEventRecord, error) {
		reads++
		if project != "project" || got != storageports.ConversationFilter(filter) {
			t.Fatalf("lost scope: %s %+v", project, got)
		}
		if after >= 2 {
			return nil, nil
		}
		return finalRecord(after + 1), nil
	})}
	first, err := service.ReadConversation(ctx, filter, "", 1)
	if err != nil || len(first.Events) != 1 || first.Events[0].EventID != 1 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page: %+v %v", first, err)
	}
	raw, _ := json.Marshal(first.Events[0])
	if first.PayloadBytes != len(raw) || first.Events[0].SourceIdentity != "execution" || first.Events[0].SourceRole != api.ConversationSourceCanonical || first.Events[0].Body.ItemKey != "event:1" {
		t.Fatal("projection lost exact provenance or byte budget", first)
	}
	changed := filter
	changed.AttemptID = "another"
	before := reads
	_, err = service.ReadConversation(ctx, changed, first.NextCursor, 1)
	var cursor *CursorError
	if !errors.As(err, &cursor) || cursor.Code != "cursor_invalidated" || reads != before {
		t.Fatal("changed filter reached storage", err, reads)
	}
	last, err := service.ReadConversation(ctx, filter, first.NextCursor, 1)
	if err != nil || len(last.Events) != 1 || last.Events[0].EventID != 2 || last.HasMore {
		t.Fatalf("continuation: %+v %v", last, err)
	}
	if _, err := service.ReadMessageBody(ctx, api.ConversationBodyRef{}, 0, 10); err == nil {
		t.Fatal("missing body reader accepted")
	}
}

func TestConversationPageBudgetDefersOverflowValidation(t *testing.T) {
	first, second := finalRecord(1), finalRecord(2)
	first.Event.Final.Diagnostic = strings.Repeat("x", MaxEventPageBytes-4096)
	second.Event.Final.Diagnostic = strings.Repeat("x", MaxEventPageBytes)
	second.Event.CreatedAt = time.Time{} // Invalid evidence is not consumed on the preceding full page.
	service := ConversationService{Project: "project", Events: conversationEvents(func(_ context.Context, _ string, _ storageports.ConversationFilter, after int64) (*storageports.ConversationEventRecord, error) {
		if after == 0 {
			return first, nil
		}
		if after == 1 {
			return second, nil
		}
		return nil, nil
	})}
	page, err := service.ReadConversation(context.Background(), api.ConversationFilter{}, "", 10)
	if err != nil || len(page.Events) != 1 || !page.HasMore || page.PayloadBytes >= MaxEventPageBytes {
		t.Fatalf("overflow consumed: %+v %v", page, err)
	}
	if _, err := service.ReadConversation(context.Background(), api.ConversationFilter{}, page.NextCursor, 10); err == nil {
		t.Fatal("continuation skipped invalid unconsumed evidence")
	}
	second.Event.CreatedAt = time.Now().UTC()
	page, err = service.ReadConversation(context.Background(), api.ConversationFilter{}, page.NextCursor, 10)
	if err != nil || len(page.Events) != 1 || page.Events[0].EventID != 2 || page.PayloadBytes <= MaxEventPageBytes {
		t.Fatal("first oversized metadata record no longer makes forward progress", err)
	}
}

func TestConversationReaderRejectsMissingOriginalProvenance(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*storageports.ConversationEventRecord)
	}{
		{"unverified", func(r *storageports.ConversationEventRecord) { r.Event.Final.Verified = false }},
		{"foreign project", func(r *storageports.ConversationEventRecord) { r.ProjectID = "foreign" }},
		{"foreign run", func(r *storageports.ConversationEventRecord) { r.Event.RunID = "foreign" }},
		{"missing session", func(r *storageports.ConversationEventRecord) { r.Event.Final.SessionID = "" }},
		{"missing source", func(r *storageports.ConversationEventRecord) { r.Event.Final.Source = "" }},
		{"missing item", func(r *storageports.ConversationEventRecord) { r.Event.Final.ItemID = "" }},
		{"stalled sequence", func(r *storageports.ConversationEventRecord) { r.EventID = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := finalRecord(1)
			tc.change(record)
			before, _ := json.Marshal(record)
			service := ConversationService{Project: "project", Events: conversationEvents(func(context.Context, string, storageports.ConversationFilter, int64) (*storageports.ConversationEventRecord, error) {
				return record, nil
			})}
			if _, err := service.ReadConversation(context.Background(), api.ConversationFilter{}, "", 1); err == nil {
				t.Fatal("invalid original final accepted")
			}
			after, _ := json.Marshal(record)
			if string(before) != string(after) {
				t.Fatal("projection changed evidence")
			}
		})
	}
}

func TestConversationBodyQueriesUseIndependentPort(t *testing.T) {
	ctx := context.Background()
	text := []byte("你好🙂tail")
	sum := sha256.Sum256(text)
	digest := hex.EncodeToString(sum[:])
	ref := api.ConversationBodyRef{ExecutionID: "execution", AttemptID: "attempt", ItemKey: "item", Revision: 2, SHA256: digest, Bytes: int64(len(text)), Completeness: api.BodyComplete}
	record := storageports.ConversationBodyRecord{Body: text, TotalBytes: ref.Bytes, SHA256: digest, Completeness: string(ref.Completeness)}
	service := ConversationService{Project: "project", Bodies: conversationBodies(func(_ context.Context, project string, got executionvalues.ConversationBodyRef, limit int) (storageports.ConversationBodyRecord, error) {
		if project != "project" || got.ExecutionID != ref.ExecutionID || got.AttemptID != ref.AttemptID || got.ItemKey != ref.ItemKey || got.Revision != ref.Revision || got.SHA256 != ref.SHA256 || got.Bytes != ref.Bytes || string(got.Completeness) != string(ref.Completeness) || limit != executionvalues.MaxConversationBodyBytes+1 {
			t.Fatal("lost bounded original body reference", got, limit)
		}
		return record, nil
	})}
	page, err := service.ReadMessageBody(ctx, ref, 0, 7)
	if err != nil || !utf8.Valid(page.Data) || string(page.Data) != "你好" || page.NextOffset != 6 {
		t.Fatalf("Unicode page changed: %+v %v", page, err)
	}
	page.Data[0] = 'x'
	if string(text) != "你好🙂tail" {
		t.Fatal("body page aliases immutable stored bytes")
	}
	if _, err := service.ReadMessageBody(ctx, ref, 1, 7); err == nil {
		t.Fatal("offset inside codepoint accepted")
	}
	if _, err := service.ReadMessageBody(ctx, ref, ref.Bytes+1, 7); err == nil {
		t.Fatal("offset beyond body accepted")
	}
	record.Body = []byte("tampered")
	if _, err := service.ReadMessageBody(ctx, ref, 0, 7); err == nil {
		t.Fatal("corrupt body revision accepted")
	}
}

func TestConversationCapabilitiesNeedNoUnrelatedReader(t *testing.T) {
	service := ConversationService{Capabilities: conversationCapabilities(func(_ context.Context, backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol, target string, limit int) ([]executionvalues.ConversationCapabilityReport, error) {
		if backend != executionbackend.BackendLocal || protocol != executionbackend.ProtocolLocalCodexAppServer || target != "local" || limit != 1000 {
			t.Fatal("capability scope changed")
		}
		return []executionvalues.ConversationCapabilityReport{{SourceIdentity: "source", Target: executionbackend.ExecutionTarget{ID: "local", ConnectionID: "private-connection"}, Capabilities: []executionvalues.ConversationCapability{{Name: "final", Status: "available", Evidence: "public"}}}}, nil
	})}
	reports, err := service.ReadCapabilities(context.Background(), executionbackend.BackendLocal, executionbackend.ProtocolLocalCodexAppServer, "local", 2000)
	raw, _ := json.Marshal(reports)
	if err != nil || len(reports) != 1 || reports[0].SourceIdentity != "source" || reports[0].Capabilities[0].Name != "final" || strings.Contains(string(raw), "private-connection") {
		t.Fatalf("capability projection: %+v %v", reports, err)
	}
}
