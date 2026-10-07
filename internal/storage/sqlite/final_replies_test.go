package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestConversationFinalFilterExcludesPreparationAndProcess(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "finals.db"))
	require(t, err)
	defer store.Close()
	// Storage-owned fixtures isolate projection from the business claim pipeline.
	_, err = store.db.ExecContext(ctx, `INSERT INTO tasks(project_identity,task_id,status,version,payload) VALUES('project','task','in_progress',1,'{}')`)
	require(t, err)
	_, err = store.db.ExecContext(ctx, `INSERT INTO workflow_runs(project_identity,workflow_run_id,task_id,phase,payload) VALUES('project','workflow','task','executing','{}')`)
	require(t, err)
	_, err = store.db.ExecContext(ctx, `INSERT INTO executions(project_identity,execution_id,workflow_run_id,status,target_json,payload) VALUES('project','execution','workflow','active','{}','{"environment":{"preparation":{"turn_id":"prepare"},"history":[{"turn_id":"old-prepare"}]}}')`)
	require(t, err)
	for i, turn := range []string{"prepare", "old-prepare", "business", "process", "other-session", "unknown"} {
		write := conversationTestWrite(fmt.Sprintf("source-%d", i), uint64(i+1), "complete", []byte(turn), nil)
		write.Event.TurnID = turn
		write.Event.ItemID = turn
		write.Event.ItemKey = turn
		write.Event.Type = executionmodel.ConversationMessageComplete
		write.Event.Phase = executionmodel.MessagePhaseFinalAnswer
		if turn == "process" {
			write.Event.Phase = executionmodel.MessagePhaseCommentary
		}
		if turn == "other-session" {
			write.Event.SessionID = "other"
		}
		if turn == "unknown" {
			write.Event.TurnID = ""
			write.Event.TurnQuality = executionmodel.IdentityUnknown
		}
		_, err = store.CommitConversation(ctx, []ConversationWrite{write}, ConversationCheckpoint{})
		if turn == "process" {
			if err == nil {
				t.Fatal("commentary was retained")
			}
			continue
		}
		require(t, err)
	}
	gap := conversationTestWrite("prepare-gap", 20, "", nil, nil)
	gap.Event.Type, gap.Event.Phase, gap.Event.Status = executionmodel.ConversationGap, executionmodel.MessagePhaseUnknown, "final_unavailable"
	gap.Event.TurnID = "prepare"
	_, err = store.CommitConversation(ctx, []ConversationWrite{gap}, ConversationCheckpoint{})
	require(t, err)
	filter := api.ConversationFilter{WorkflowID: "workflow", ExecutionID: "execution", SessionID: "session", FinalsOnly: true}
	page, err := conversationQueries(store, "project").ReadConversation(ctx, filter, "", 10)
	require(t, err)
	if len(page.Events) != 1 || page.Events[0].TurnID != "business" {
		t.Fatalf("preparation/process leaked into final index: %+v", page.Events)
	}
	filter.FinalsOnly = false
	page, err = conversationQueries(store, "project").ReadConversation(ctx, filter, "", 10)
	require(t, err)
	if len(page.Events) != 5 {
		t.Fatalf("final projection deleted diagnostics: %+v", page.Events)
	}
}
