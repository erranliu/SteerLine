package execution

import (
	"context"
	"encoding/json"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productdto "github.com/erranliu/SteerLine/internal/product/api"
	"path/filepath"
	"testing"
)

type neutralHostWaitSource struct{ neutralConversationSources }

func TestHostInputWaitPersistsWithoutBusinessDecision(t *testing.T) {
	store, err := openSQLiteFixture(filepath.Join(t.TempDir(), "wait.db"))
	require(t, err)
	defer store.Close()
	run := &reconstructiontypes.RunView{RunID: newID(), AttemptID: newID(), ExecutionID: newID(), Agent: "fixture"}
	service := &Service{Config: Config{identity: "project"}, domainStore: store, commandStore: store, conversationStore: store, conversationSources: neutralHostWaitSource{}}
	source := WorkerEvent{RunID: run.RunID, AttemptID: run.AttemptID, Source: "fixture-host", SourceSegment: "fixture-host-v1:probe", SourceEventID: "host-wait:probe", SourceIdentity: "fixture-original-resource", SourceSeq: 1, InputWait: true}
	require(t, service.recordConversationEvent(run, source))
	require(t, service.recordConversationEvent(run, source))
	page, err := newFixtureConversationQuery(store, "project").ReadConversation(context.Background(), productdto.ConversationFilter{ExecutionID: run.ExecutionID, AttemptID: run.AttemptID}, "", 10)
	require(t, err)
	if len(page.Events) != 1 || page.Events[0].Type != productdto.ConversationInputWait || page.Events[0].Status != "waiting" {
		t.Fatalf("persisted host wait became business input or lost source identity: %+v", page.Events)
	}
	record, err := store.NextConversationEvent(context.Background(), "project", productdto.ConversationFilter{ExecutionID: run.ExecutionID, AttemptID: run.AttemptID}, 0)
	require(t, err)
	if record == nil {
		t.Fatal("host wait event was not persisted")
	}
	var stored executionvalues.ConversationEvent
	require(t, json.Unmarshal(record.Payload, &stored))
	if stored.Attributes["wait_origin"] != "host" || stored.Attributes["business_decision"] != false {
		t.Fatalf("persisted host wait lost its internal source evidence: %+v", stored)
	}
	if run.Decision != nil || len(service.domain.Decisions) != 0 {
		t.Fatal("host wait created a center business decision")
	}
}
