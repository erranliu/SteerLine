package sqlite_test

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/query"
	readsqlite "github.com/erranliu/SteerLine/internal/steerline/sqlite"
)

func TestConversationReaderKeepsSourceScopeOnReadOnlyConnection(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "steerline.db")
	writer, err := steerline.OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	event := contracts.ConversationEvent{Version: 1, ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Source: "codex-jsonl", SourceSegment: "segment", SourceEventID: "raw", ItemKey: "item", Type: contracts.ConversationMessageComplete, Phase: contracts.MessagePhaseFinalAnswer, SessionQuality: contracts.IdentityUnknown, TurnQuality: contracts.IdentityUnknown, ItemQuality: contracts.IdentityUnknown, SourceEventQuality: contracts.IdentityDerived, ReceivedAt: time.Now().UTC()}
	committed, err := writer.CommitConversation(ctx, []contracts.ConversationWrite{{Event: event, Body: []byte("你好🙂"), BodyMode: "complete", Raw: []byte("raw bytes")}}, contracts.ConversationCheckpoint{})
	if err != nil {
		t.Fatal(err)
	}
	native := event
	native.Source, native.SourceEventID, native.Type = "codex-app-server", "session", contracts.ConversationSession
	other := event
	other.ProjectID = "other-project"
	if _, err := writer.CommitConversation(ctx, []contracts.ConversationWrite{{Event: native}, {Event: other}}, contracts.ConversationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	report := contracts.ConversationCapabilityReport{Version: 1, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalCodexAppServer, Agent: "codex", Target: executionapi.ExecutionTarget{ID: "local"}, CheckedAt: time.Now().UTC(), Capabilities: []contracts.ConversationCapability{{Name: "final_content", Status: executionapi.CapabilityVerified, Evidence: "fixture"}}}
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
	if err != nil || len(page.Events) != 1 || page.Events[0].Source != "codex-app-server" {
		t.Fatalf("canonical source selection changed: %+v %v", page, err)
	}
	filter.Source = "codex-jsonl"
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
	if err != nil || len(page.Events) != 1 || page.Events[0].Source != "codex-jsonl" {
		t.Fatalf("another project's native session hid raw evidence: %+v %v", page, err)
	}
}
