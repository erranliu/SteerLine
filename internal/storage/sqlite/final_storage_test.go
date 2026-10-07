package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	model "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestCompleteFinalIsImmutableAndProcessStorageRetired(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "final.db"))
	require(t, err)
	defer store.Close()
	write := conversationTestWrite("native-final", 1, "complete", []byte("完整回复🙂"), nil)
	first, err := store.CommitConversation(ctx, []ConversationWrite{write}, conversationTestCheckpoint(1, "original"))
	require(t, err)
	second, err := store.CommitConversation(ctx, []ConversationWrite{write}, conversationTestCheckpoint(1, "original"))
	require(t, err)
	if first[0].EventID != second[0].EventID || first[0].Body.Revision != 1 {
		t.Fatal("final replay duplicated content")
	}
	for _, mode := range []string{"delta", "snapshot"} {
		changed := write
		changed.Event.SourceEventID, changed.BodyMode = "new-"+mode, mode
		if _, err := store.CommitConversation(ctx, []ConversationWrite{changed}, conversationTestCheckpoint(2, "changed")); err == nil {
			t.Fatalf("retired %s update accepted", mode)
		}
	}
	changed := write
	changed.Body, changed.Event.SourceEventID = []byte("replacement"), "different-final-event"
	if _, err := store.CommitConversation(ctx, []ConversationWrite{changed}, conversationTestCheckpoint(2, "changed")); err == nil {
		t.Fatal("completed final item was overwritten")
	}
	changed = write
	changed.Event.Type, changed.Event.Phase = model.ConversationEventType("tool.completed"), model.MessagePhaseUnknown
	changed.Event.SourceEventID, changed.BodyMode = "tool", ""
	if _, err := store.CommitConversation(ctx, []ConversationWrite{changed}, conversationTestCheckpoint(2, "changed")); err == nil {
		t.Fatal("process event bypassed the final-only collector")
	}
	cp, err := store.ConversationCheckpoint(ctx, "project", "execution", "attempt", "fixture-journal", "segment-1")
	require(t, err)
	if cp.LocalSeq != 1 || cp.Cursor != "original" {
		t.Fatal("rejected content advanced the checkpoint")
	}
	var retired int
	require(t, store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name IN ('conversation_raw','conversation_current_messages')`).Scan(&retired))
	if retired != 0 {
		t.Fatal("retired process storage is still present")
	}
}

func TestLateFinalResolvesOnlyItsOriginalMissingFinal(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "late.db"))
	require(t, err)
	defer store.Close()
	final := conversationTestWrite("native-final", 2, "complete", []byte("late final"), nil)
	gap := final.Event
	gap.Type, gap.Phase, gap.Status = model.ConversationGap, model.MessagePhaseUnknown, "final_unavailable"
	gap.FinalAvailability, gap.SourceEventID, gap.Diagnostic = model.FinalUnknown, "missing-final", "missing final"
	other := gap
	other.TurnID, other.SourceEventID = "other-turn", "other-missing-final"
	_, err = store.CommitConversation(ctx, []ConversationWrite{{Event: gap}, {Event: other}}, ConversationCheckpoint{})
	require(t, err)
	generic := gap
	generic.Status, generic.SourceEventID, generic.Diagnostic = "", "read-error", "source read failed"
	_, err = store.CommitConversation(ctx, []ConversationWrite{{Event: generic}, final}, ConversationCheckpoint{})
	require(t, err)
	page, err := conversationQueries(store, "project").ReadConversation(ctx, ConversationFilter{WorkflowID: "workflow", ExecutionID: "execution", FinalsOnly: true}, "", 10)
	require(t, err)
	if len(page.Events) != 3 || page.Events[0].TurnID != "other-turn" || page.Events[1].Diagnostic != generic.Diagnostic || page.Events[2].Body == nil {
		t.Fatalf("late final erased another turn's gap or left its resolved gap: %+v", page)
	}
}
