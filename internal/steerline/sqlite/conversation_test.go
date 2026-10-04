package sqlite_test

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/query"
	readsqlite "github.com/erranliu/SteerLine/internal/steerline/sqlite"
)

func TestConversationReaderKeepsSourceScopeOnReadOnlyConnection(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "steerline.db")
	writer, err := readsqlite.OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	event := contracts.ConversationEvent{Version: contracts.ConversationVersion, ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Source: "fallback-demo", SourceRole: contracts.ConversationSourceFallback, SourceSegment: "segment", SourceEventID: "raw", ItemKey: "item", Type: contracts.ConversationMessageComplete, Phase: contracts.MessagePhaseFinalAnswer, SessionQuality: contracts.IdentityUnknown, TurnQuality: contracts.IdentityUnknown, ItemQuality: contracts.IdentityUnknown, SourceEventQuality: contracts.IdentityDerived, ReceivedAt: time.Now().UTC()}
	committed, err := writer.CommitConversation(ctx, []contracts.ConversationWrite{{Event: event, Body: []byte("你好🙂"), BodyMode: "complete", Raw: []byte("raw bytes")}}, contracts.ConversationCheckpoint{})
	if err != nil {
		t.Fatal(err)
	}
	native := event
	native.Source, native.SourceRole, native.SourceEventID, native.Type = "canonical-demo", contracts.ConversationSourceCanonical, "session", contracts.ConversationSession
	other := event
	other.ProjectID = "other-project"
	if _, err := writer.CommitConversation(ctx, []contracts.ConversationWrite{{Event: native}, {Event: other}}, contracts.ConversationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	report := contracts.ConversationCapabilityReport{Version: 1, Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalCodexAppServer, Agent: "codex", Target: executionbackend.ExecutionTarget{ID: "local"}, CheckedAt: time.Now().UTC(), Capabilities: []contracts.ConversationCapability{{Name: "final_content", Status: executionbackend.CapabilityVerified, Evidence: "fixture"}}}
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
	filter := contracts.ConversationFilter{ExecutionID: event.ExecutionID, AttemptID: event.AttemptID}
	page, err := service.ReadConversation(ctx, filter, "", 10)
	if err != nil || len(page.Events) != 1 || page.Events[0].Source != "canonical-demo" {
		t.Fatalf("canonical source selection changed: %+v %v", page, err)
	}
	filter.Source = "fallback-demo"
	page, err = service.ReadConversation(ctx, filter, "", 10)
	if err != nil || len(page.Events) != 1 || page.Events[0].ProjectID != "project" {
		t.Fatalf("explicit raw source crossed scope: %+v %v", page, err)
	}
	body, err := service.ReadMessageBody(ctx, *committed[0].Body, 0, 4096)
	if err != nil || string(body.Data) != "你好🙂" || !body.Complete {
		t.Fatalf("read-only body query failed: %+v %v", body, err)
	}
	raw, err := service.ReadRaw(ctx, *committed[0].Raw, 0, 4096)
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
