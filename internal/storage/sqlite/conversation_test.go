package sqlite_test

import (
	"context"
	"database/sql"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	productdto "github.com/erranliu/SteerLine/internal/product/api"
	"net/url"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/product/query"
	readsqlite "github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func TestConversationReaderKeepsSourceScopeOnReadOnlyConnection(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "steerline.db")
	writer, err := readsqlite.OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	event := executionvalues.ConversationEvent{Version: executionvalues.ConversationVersion, ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Source: "fallback-demo", SourceRole: executionvalues.ConversationSourceFallback, SourceSegment: "segment", SourceEventID: "raw", ItemKey: "item", Type: executionvalues.ConversationMessageComplete, Phase: executionvalues.MessagePhaseFinalAnswer, SessionQuality: executionvalues.IdentityUnknown, TurnQuality: executionvalues.IdentityUnknown, ItemQuality: executionvalues.IdentityUnknown, SourceEventQuality: executionvalues.IdentityDerived, ReceivedAt: time.Now().UTC()}
	committed, err := writer.CommitConversation(ctx, []executionvalues.ConversationWrite{{Event: event, Body: []byte("你好🙂"), BodyMode: "complete", Raw: []byte("raw bytes")}}, executionvalues.ConversationCheckpoint{})
	if err != nil {
		t.Fatal(err)
	}
	native := event
	native.Source, native.SourceRole, native.SourceEventID, native.Type = "canonical-demo", executionvalues.ConversationSourceCanonical, "session", executionvalues.ConversationSession
	other := event
	other.ProjectID = "other-project"
	if _, err := writer.CommitConversation(ctx, []executionvalues.ConversationWrite{{Event: native}, {Event: other}}, executionvalues.ConversationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	report := executionvalues.ConversationCapabilityReport{Version: 1, Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalCodexAppServer, Agent: "codex", Target: executionbackend.ExecutionTarget{ID: "local"}, CheckedAt: time.Now().UTC(), Capabilities: []executionvalues.ConversationCapability{{Name: "final_content", Status: executionbackend.CapabilityVerified, Evidence: "fixture"}}}
	if err := writer.SaveConversationCapabilityReport(ctx, report); err != nil {
		t.Fatal(err)
	}
	uriPath := filepath.ToSlash(path)
	if runtime.GOOS == "windows" {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath}
	db, err := sql.Open("sqlite", uri.String()+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reader := readsqlite.NewReader(db, nil)
	service := query.ConversationService{Project: "project", Events: reader, Bodies: reader, Raw: reader, Capabilities: reader}
	filter := productdto.ConversationFilter{ExecutionID: event.ExecutionID, AttemptID: event.AttemptID}
	page, err := service.ReadConversation(ctx, filter, "", 10)
	if err != nil || len(page.Events) != 1 || page.Events[0].Source != "canonical-demo" {
		t.Fatalf("canonical source selection changed: %+v %v", page, err)
	}
	filter.Source = "fallback-demo"
	page, err = service.ReadConversation(ctx, filter, "", 10)
	if err != nil || len(page.Events) != 1 || page.Events[0].ProjectID != "project" {
		t.Fatalf("explicit raw source crossed scope: %+v %v", page, err)
	}
	body, err := service.ReadMessageBody(ctx, productdto.ConversationBodyRef{ExecutionID: committed[0].Body.ExecutionID, AttemptID: committed[0].Body.AttemptID, ItemKey: committed[0].Body.ItemKey, Revision: committed[0].Body.Revision, SHA256: committed[0].Body.SHA256, Bytes: committed[0].Body.Bytes, Source: committed[0].Body.Source, Completeness: productdto.BodyCompleteness(committed[0].Body.Completeness)}, 0, 4096)
	if err != nil || string(body.Data) != "你好🙂" || !body.Complete {
		t.Fatalf("read-only body query failed: %+v %v", body, err)
	}
	raw, err := service.ReadRaw(ctx, productdto.ConversationRawRef{ExecutionID: committed[0].Raw.ExecutionID, AttemptID: committed[0].Raw.AttemptID, ID: committed[0].Raw.ID, SHA256: committed[0].Raw.SHA256, Bytes: committed[0].Raw.Bytes}, 0, 4096)
	if err != nil || string(raw.Data) != "raw bytes" || !raw.Complete {
		t.Fatalf("read-only raw query failed: %+v %v", raw, err)
	}
	reports, err := service.ReadCapabilities(ctx, report.Backend, report.Protocol, report.Target.ID, 1)
	if err != nil || len(reports) != 1 || reports[0].Capabilities[0].Evidence != "fixture" {
		t.Fatalf("read-only capability query failed: %+v %v", reports, err)
	}
	service.Project = "other-project"
	filter.Source = ""
	page, err = service.ReadConversation(ctx, filter, "", 10)
	if err != nil || len(page.Events) != 1 || page.Events[0].Source != "fallback-demo" {
		t.Fatalf("another project's native session hid raw evidence: %+v %v", page, err)
	}
}
