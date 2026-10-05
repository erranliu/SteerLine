package execution

import (
	"context"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productdto "github.com/erranliu/SteerLine/internal/product/api"
	"path/filepath"
	"testing"
)

type neutralHostWaitSource struct{ neutralConversationSources }

func (neutralHostWaitSource) MapConversation(input executionvalues.ConversationInput) (executionvalues.ConversationMappedBatch, error) {
	event := input.Event
	event.ReceivedAt, event.Source, event.SourceRole = now(), input.Source, executionvalues.ConversationSourceFallback
	event.SourceSegment, event.SourceIdentity, event.SourceSeq = input.Segment, input.Identity, input.SourceSeq
	event.SourceEventQuality, event.Phase = executionvalues.IdentityDerived, executionvalues.MessagePhaseUnknown
	event.Type, event.Status = executionvalues.ConversationInputWait, "waiting"
	event.Attributes = map[string]any{"wait_origin": "host", "business_decision": false}
	event.Evidence = map[string]string{"wait_source": "fixture.host-observation"}
	return executionvalues.ConversationMappedBatch{Writes: []executionvalues.ConversationWrite{{Event: event}},
		Checkpoint: executionvalues.ConversationCheckpoint{ProjectID: event.ProjectID, ExecutionID: event.ExecutionID, AttemptID: event.AttemptID, Source: event.Source, SourceSegment: event.SourceSegment, SourceIdentity: event.SourceIdentity, Generation: input.Generation, LocalSeq: input.SourceSeq}}, nil
}

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
	if len(page.Events) != 1 || page.Events[0].Type != executionvalues.ConversationInputWait || page.Events[0].Status != "waiting" || page.Events[0].Attributes["wait_origin"] != "host" || page.Events[0].Attributes["business_decision"] != false || page.Events[0].Evidence["wait_source"] != "fixture.host-observation" {
		t.Fatalf("persisted host wait became business input or lost source identity: %+v", page.Events)
	}
	if run.Decision != nil || len(service.domain.Decisions) != 0 {
		t.Fatal("host wait created a center business decision")
	}
}
