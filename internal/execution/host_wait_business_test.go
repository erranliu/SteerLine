package execution

import (
	"context"
	"encoding/json"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
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

type lifecycleWaitObserver struct {
	events chan executionbackend.LifecycleEvent
	done   chan error
}

func (o lifecycleWaitObserver) Events() <-chan executionbackend.LifecycleEvent { return o.events }
func (o lifecycleWaitObserver) Done() <-chan error                             { return o.done }
func (o lifecycleWaitObserver) StopHandle() executionbackend.StopHandle        { return nil }
func (o lifecycleWaitObserver) Close(context.Context) error                    { return nil }
func TestLifecycleHostWaitPersistsThroughWorkerBridge(t *testing.T) {
	store, err := openSQLiteFixture(filepath.Join(t.TempDir(), "wait-bridge.db"))
	require(t, err)
	defer store.Close()
	run := &reconstructiontypes.RunView{RunID: newID(), AttemptID: newID(), ExecutionID: newID(), Agent: "fixture"}
	service := &Service{Config: Config{identity: "project"}, domainStore: store, commandStore: store, conversationStore: store}
	observer := lifecycleWaitObserver{events: make(chan executionbackend.LifecycleEvent, 1), done: make(chan error, 1)}
	observer.events <- executionbackend.LifecycleEvent{InputWait: true, Diagnostic: "confirmed: approval", Fact: executionbackend.EventFact{Kind: executionbackend.EventStatus, Source: "native-host", SourceIdentity: "original-session", SourceSegment: "original-process", SourceEventID: "wait-1", SessionID: "session"}, Kind: "execution", Status: "waiting", Title: "Host wait"}
	close(observer.events)
	observer.done <- nil
	close(observer.done)
	worker := workerFromLifecycle(run.RunID, run.AttemptID, observer)
	event := <-worker.Events
	if !event.InputWait || event.Source != "native-host" || event.SourceIdentity != "original-session" {
		t.Fatal("lifecycle wait evidence lost in bridge")
	}
	require(t, service.recordConversationEvent(run, event))
	page, err := newFixtureConversationQuery(store, "project").ReadConversation(context.Background(), productdto.ConversationFilter{ExecutionID: run.ExecutionID, AttemptID: run.AttemptID}, "", 10)
	require(t, err)
	if len(page.Events) != 1 || page.Events[0].Type != productdto.ConversationInputWait || page.Events[0].Diagnostic != "confirmed: approval" || run.Decision != nil {
		t.Fatal("native wait not preserved independently of business decision")
	}
}
