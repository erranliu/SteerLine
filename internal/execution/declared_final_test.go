package execution

import (
	"context"
	"testing"

	m "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestDeclaredFinalStoredExactlyAndReplaySafe(t *testing.T) {
	s, run, _ := workerStageFixture(t)
	evidence := workerStageEvidence(s, run, "完整正文🙂\n\"quoted\"\\", true)
	evidence.FinalOrigin = "terminal_observed"
	ctx := context.Background()
	require(t, s.recordDeclaredFinal(ctx, run, evidence))
	require(t, s.recordDeclaredFinal(ctx, run, evidence))
	query := newFixtureConversationQuery(s.conversationStore, s.Config.identity)
	page, err := query.ReadConversation(ctx, api.ConversationFilter{ExecutionID: run.ExecutionID, AttemptID: run.AttemptID}, "", 20)
	require(t, err)
	if len(page.Events) != 2 {
		t.Fatalf("replay duplicated canonical result: %+v", page)
	}
	var final *api.ConversationBodyRef
	for _, event := range page.Events {
		if event.Body != nil {
			final = event.Body
			if event.Source != evidence.FinalSource {
				t.Fatal("declared result source was lost")
			}
		}
	}
	if final == nil {
		t.Fatal("verified full final has no body")
	}
	body, err := query.ReadMessageBody(ctx, *final, 0, 8192)
	require(t, err)
	if string(body.Data) != evidence.LastMessage {
		t.Fatalf("canonical body changed: %+v", body)
	}
	evidence.LastMessage = "conflicting body"
	if s.recordDeclaredFinal(ctx, run, evidence) == nil {
		t.Fatal("completed final was overwritten")
	}
}

func TestDeclaredFinalMissingProvenanceStoresNoCanonicalBody(t *testing.T) {
	s, run, port := workerStageFixture(t)
	evidence := workerStageEvidence(s, run, "unverified summary", false)
	evidence.FinalOrigin, evidence.FinalVerified, evidence.FinalDiagnostic = "terminal_observed", false, "missing result"
	fixed, err := s.workerRecords(run)
	require(t, err)
	require(t, s.acceptStageTurn(context.Background(), run, port, fixed, evidence))
	w := s.workflowRecord(run)
	if w.StepID != m.StepImplement || w.Status != "waiting_input" || len(w.StageInputs) != 1 || len(w.StageResults) != 1 || run.Decision == nil || !s.executionRecord(run).RecoveryRequired {
		t.Fatal("failed native implementation advanced without explicit recovery")
	}
	result := w.StageResults[0]
	if result.CommandID != evidence.CommandID || result.Succeeded || result.FinalVerified || result.Outcome != "blocked" || result.Final != evidence.LastMessage || result.FinalDiagnostic != evidence.FinalDiagnostic {
		t.Fatal("failed native implementation lost its unverified source facts")
	}
	page, err := newFixtureConversationQuery(s.conversationStore, s.Config.identity).ReadConversation(context.Background(), api.ConversationFilter{ExecutionID: run.ExecutionID}, "", 10)
	require(t, err)
	if len(page.Events) != 0 {
		t.Fatal("unverified summary became a canonical body")
	}
}

func TestDeclaredReplyCannotInventNativeProvenance(t *testing.T) {
	s, run, _ := workerStageFixture(t)
	e := workerStageEvidence(s, run, "original", true)
	e.FinalOrigin = "terminal_observed"
	e.FinalNativeTurnID = "invented"
	if s.recordDeclaredFinal(context.Background(), run, e) == nil {
		t.Fatal("worker declaration became native provenance")
	}
}
