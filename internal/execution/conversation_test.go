package execution

import (
	"context"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productdto "github.com/erranliu/SteerLine/internal/product/api"
	"path/filepath"
	"testing"
)

func TestConversationFactsDoNotStoreRaw(t *testing.T) {
	store, err := openSQLiteFixture(filepath.Join(t.TempDir(), "facts.db"))
	require(t, err)
	defer store.Close()
	run := &reconstructiontypes.RunView{RunID: newID(), ExecutionID: newID(), AttemptID: newID(), Agent: "fixture"}
	service := &Service{Config: Config{identity: "project"}, conversationStore: store}
	source := WorkerEvent{Source: "native-output", SourceSegment: "original", SourceEventID: "output", Raw: []byte("tool output and commentary")}
	require(t, service.recordConversationEvent(run, source))
	source.SourceGap, source.Diagnostic = true, "native output unavailable"
	require(t, service.recordConversationEvent(run, source))
	require(t, service.recordConversationEvent(run, source))
	page, err := newFixtureConversationQuery(store, "project").ReadConversation(context.Background(), productdto.ConversationFilter{ExecutionID: run.ExecutionID, AttemptID: run.AttemptID}, "", 10)
	require(t, err)
	if len(page.Events) != 1 || page.Events[0].Type != productdto.ConversationGap || page.Events[0].Body != nil || page.Events[0].Diagnostic != source.Diagnostic {
		t.Fatalf("raw output retained or gap lost: %+v", page)
	}
}
