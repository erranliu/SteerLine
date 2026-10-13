package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"strings"
	"testing"
)

func TestTUIHTTPRoutes(t *testing.T) {
	t.Run("recent activity advances beyond the stream first page", testTUIHTTPRecentActivity)
	server, _, reader, _ := newAPIFixture(t)
	ctx := context.Background()
	run := &api.CandidateSummary{RunID: strings.Repeat("a", 32), ExecutionID: strings.Repeat("b", 32), AttemptID: strings.Repeat("c", 32), SessionID: "original", SessionGeneration: "generation"}
	reader.snapshot.Connection.Snapshot.Page.Runs = []*api.CandidateSummary{run}
	opened := 0
	reader.onOpenNative = func(target api.NativeSessionTarget) error {
		if target.RunID != run.RunID || target.SessionID != run.SessionID {
			t.Fatal("native binding changed")
		}
		opened++
		return nil
	}
	client, err := Open(ctx, server.options.RunDir, server.options.ProjectIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var report api.RecoveryInspection
	if err = client.Get(ctx, "/runs/"+run.RunID+"/recovery?execution_id="+run.ExecutionID+"&attempt_id="+run.AttemptID, &report); err != nil || report.RunID != run.RunID {
		t.Fatalf("recovery: %+v %v", report, err)
	}
	if err = client.Get(ctx, "/runs/"+run.RunID+"/recovery?execution_id=wrong&attempt_id="+run.AttemptID, &report); err == nil {
		t.Fatal("wrong recovery target accepted")
	}
	if reader.recoveryReads != 1 {
		t.Fatal("invalid recovery invoked implementation")
	}
	payload, _ := json.Marshal(api.NativeSessionTarget{RunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, SessionID: run.SessionID, SessionGeneration: run.SessionGeneration})
	envelope := CommandEnvelope{RequestID: strings.Repeat("d", 32), ProjectID: client.Identity().ProjectID, InstanceID: client.Identity().InstanceID, Action: "open_native_session", TargetID: run.RunID, Payload: payload}
	response, err := client.SubmitCommand(ctx, envelope)
	if err != nil || response.Status != "applied" {
		t.Fatalf("open: %+v %v", response, err)
	}
	response, err = client.SubmitCommand(ctx, envelope)
	if err != nil || response.Status != "applied" || opened != 1 {
		t.Fatalf("reopened original: %d %+v %v", opened, response, err)
	}
	response, err = client.Command(ctx, envelope.RequestID)
	if err != nil || response.Status != "applied" {
		t.Fatal("native result not recoverable", err)
	}
	// A different original generation must be rejected before any host side effect.
	envelope.RequestID = strings.Repeat("e", 32)
	var target api.NativeSessionTarget
	json.Unmarshal(payload, &target)
	target.SessionGeneration = "other"
	envelope.Payload, _ = json.Marshal(target)
	if response, err = client.SubmitCommand(ctx, envelope); err != nil || response.Status != "rejected" {
		t.Fatalf("stale generation was not rejected: %+v %v", response, err)
	}
	if opened != 1 {
		t.Fatal("stale native target created a side effect")
	}
	reader.onOpenNative = func(api.NativeSessionTarget) error { opened++; return errors.New("viewer outcome unverified") }
	envelope.RequestID = strings.Repeat("f", 32)
	envelope.Payload = payload
	for i := 0; i < 2; i++ {
		response, err = client.SubmitCommand(ctx, envelope)
		if err != nil || response.Status != "outcome_unknown" {
			t.Fatalf("unknown viewer: %+v %v", response, err)
		}
	}
	if opened != 2 {
		t.Fatal("unknown viewer was invoked again")
	}
	envelope.RequestID = strings.Repeat("1", 32)
	reader.commands[envelope.RequestID] = api.CommandReceipt{CommandID: envelope.RequestID, Kind: "answer_decision", Status: "accepted"}
	if _, err = client.SubmitCommand(ctx, envelope); err == nil || opened != 2 {
		t.Fatal("viewer stole formal command ID")
	}

	// Full final history must include old attempts, while a foreign attempt is
	// rejected before a body query. Session/source filters remain intact.
	old := api.ConversationEvent{WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: "old-attempt", SessionID: "original", Source: "canonical"}
	reader.onConversation = func(filter api.ConversationFilter, _ string, _ int) (api.ConversationPage, error) {
		if filter.WorkflowID != run.RunID || filter.ExecutionID != run.ExecutionID {
			t.Fatal("conversation escaped original run")
		}
		if filter.AttemptID == "foreign" {
			return api.ConversationPage{}, nil
		}
		return api.ConversationPage{Events: []api.ConversationEvent{old}}, nil
	}
	page, err := client.Conversation(ctx, run.RunID, api.ConversationFilter{ExecutionID: run.ExecutionID, FinalsOnly: true, SessionID: "original", Source: "canonical"}, "", 100)
	if err != nil || len(page.Page.Events) != 1 || page.Page.Events[0].AttemptID != "old-attempt" {
		t.Fatalf("history: %+v %v", page, err)
	}
	ref := api.ConversationBodyRef{ExecutionID: run.ExecutionID, AttemptID: "old-attempt", ItemKey: "original", Revision: 1}
	if _, err = client.Body(ctx, run.RunID, ref, 0, 100); err != nil {
		t.Fatal("historical body", err)
	}
	ref.AttemptID = "foreign"
	if _, err = client.Body(ctx, run.RunID, ref, 0, 100); err == nil {
		t.Fatal("foreign historical body accepted")
	}
}

func testTUIHTTPRecentActivity(t *testing.T) {
	server, _, reader, _ := newAPIFixture(t)
	run := &api.CandidateSummary{RunID: strings.Repeat("a", 32), ExecutionID: strings.Repeat("b", 32), AttemptID: strings.Repeat("c", 32)}
	reader.snapshot.Connection.Snapshot.Page.Runs = []*api.CandidateSummary{run}
	reader.activity = api.LogBatch{Cursor: api.ActivityCursor{RunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, Initialized: true, AfterEventID: 200}}
	latest := int64(205)
	reader.onRecentActivity = func(r, e, a string) (api.LogBatch, error) {
		if r != run.RunID || e != run.ExecutionID || a != run.AttemptID {
			t.Fatal("recent activity target changed")
		}
		page := reader.activity
		page.Cursor.AfterEventID = latest
		return page, nil
	}
	client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for _, event := range []int64{205, 230} {
		latest = event
		page, err := client.RecentActivity(context.Background(), run.RunID, run.ExecutionID, run.AttemptID)
		if err != nil || page.Page.Cursor.AfterEventID != event {
			t.Fatalf("recent activity is stale: %+v %v", page, err)
		}
	}
	if _, err = client.RecentActivity(context.Background(), run.RunID, "wrong", run.AttemptID); err == nil {
		t.Fatal("foreign recent activity accepted")
	}
}
