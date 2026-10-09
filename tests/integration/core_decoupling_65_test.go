package integration

import (
	"context"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productdto "github.com/erranliu/SteerLine/internal/product/api"
	productquery "github.com/erranliu/SteerLine/internal/product/query"

	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestCD65HistoryAndPreparation(t *testing.T) {
	t.Run("history_commits_without_advancing_business_gate", testCD65HistoryCommitsOnly)

}

func testCD65HistoryCommitsOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fixture := newLifecycleProject(t, "local")
	createIntegrationTask(t, ctx, fixture, 901)
	resolver := &integrationLifecycleResolver{}
	service := startIntegrationService(t, ctx, fixture.config, resolver)
	defer service.stop(t)
	claimIntegrationTask(t, ctx, fixture, 901, service.instanceID)
	run := waitIntegrationRun(t, ctx, service, fixture.config, func(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord) bool {
		return run.Decision != nil && run.Decision.Status == "pending" && execution.TurnNumber == 1 && execution.CurrentTurnID == execution.LastStoppedTurnID
	})
	if run.run.Process == nil || run.execution.SessionID != integrationAppServerSession {
		t.Fatalf("history source lost the original session/process identity: %+v", run)
	}
	filter := productdto.ConversationFilter{ExecutionID: run.execution.ExecutionID, AttemptID: run.run.AttemptID, Source: "codex-app-server"}
	query := productquery.NewConversationService(service.reader, fixture.config.Identity())
	var page productdto.ConversationPage
	var pageError error
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("last history page: error=%v events=%s", pageError, mustJSON(page.Events))
		}
	})
	waitFor(t, 30*time.Second, func() bool {
		page, pageError = query.ReadConversation(ctx, filter, "", 50)
		if pageError != nil {
			return false
		}
		hasFinal, hasCompletion := false, false
		for _, event := range page.Events {
			if event.Type == productdto.ConversationMessageComplete && event.Phase == productdto.MessagePhaseFinalAnswer && event.Body != nil {
				hasFinal = true
			}
			if event.Type == productdto.ConversationTurnCompleted {
				hasCompletion = true
			}
		}
		return hasFinal && hasCompletion
	})
	sessions, starts, finals, completed := 0, 0, 0, 0
	var sourceSegment string
	for _, event := range page.Events {
		if event.AttemptID != run.run.AttemptID {
			t.Fatalf("history crossed its source attempt: %+v", event)
		}
		sourceSegment = event.SourceSegment
		switch event.Type {
		case productdto.ConversationSession:
			sessions++
		case productdto.ConversationTurnStarted:
			starts++
		case productdto.ConversationTurnCompleted:
			completed++
		case productdto.ConversationMessageComplete:
			if event.Phase == productdto.MessagePhaseFinalAnswer {
				finals++
				if event.Body == nil {
					t.Fatal("committed final history event has no body reference")
				}
				body, err := query.ReadMessageBody(ctx, *event.Body, 0, 64<<10)
				require(t, err)
				if !body.Complete || body.SHA256 != event.Body.SHA256 {
					t.Fatal("history body reference does not match its committed content")
				}
			}
		}
	}
	if sessions != 1 || starts != 1 || finals != 1 || completed != 1 {
		t.Fatalf("history page did not retain the one-turn source facts: session=%d starts=%d finals=%d completed=%d events=%+v", sessions, starts, finals, completed, page.Events)
	}
	checkpoint, err := service.reader.ConversationCheckpoint(ctx, fixture.config.Identity(), run.execution.ExecutionID, run.run.AttemptID, "codex-app-server", sourceSegment)
	require(t, err)
	if checkpoint.Cursor == "" || checkpoint.LocalSeq == 0 {
		t.Fatal("conversation source checkpoint was not committed with its mapped batch")
	}
	assertQueriedTaskPhase(t, ctx, fixture.config, "901", "waiting_decision")
	if run.run.Decision == nil || run.run.Decision.Status != "pending" {
		t.Fatal("history collection advanced the pending business decision")
	}
	if resolver.count(executionbackend.ProtocolLocalCodexAppServer) == 0 {
		t.Fatal("history source was collected without resolving the injected implementation")
	}
}
