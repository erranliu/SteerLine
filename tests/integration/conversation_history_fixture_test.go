package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

// Historical native receipts belong to the immutable evidence snapshot, not
// the current documentation tree. Preserve the original bytes and assertions.
func historicalConversationFixture(t *testing.T, name string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "git", "show", "172cea6c6530b6693b742324a9c1d4f9aeb17b6d:docs/validation/"+name).Output()
	require(t, err)
	return raw
}

func TestCodexHistoryNativeFixtures(t *testing.T) {
	raw := historicalConversationFixture(t, "pr-04b-public-history-native-2026-09-25.json")
	var fixture struct {
		Samples []struct {
			Backend executionbackend.BackendKind `json:"backend"`
			Thread  codexworker.HistoryThread    `json:"thread"`
		} `json:"samples"`
	}
	require(t, json.Unmarshal(raw, &fixture))
	if len(fixture.Samples) != 2 {
		t.Fatalf("need both native source fixtures, got %d", len(fixture.Samples))
	}
	for _, sample := range fixture.Samples {
		t.Run(string(sample.Backend), func(t *testing.T) {
			store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "history.db"))
			require(t, err)
			defer store.Close()
			base := executionvalues.ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Backend: sample.Backend, Protocol: executionbackend.ProtocolLocalProcess, Agent: "codex", Source: "codex-app-server", SourceSegment: "native", SourceIdentity: "native", SessionID: sample.Thread.ID}
			if sample.Backend == executionbackend.BackendOrca {
				base.Protocol = executionbackend.ProtocolOrcaTerminalV3
			}
			var all []executionvalues.ConversationWrite
			for _, turn := range sample.Thread.Turns {
				writes, mapErr := codexworker.MapHistoryTurn(base, turn, uint64(len(all)+1))
				require(t, mapErr)
				all = append(all, writes...)
			}
			first, err := store.CommitConversation(context.Background(), all, executionvalues.ConversationCheckpoint{})
			require(t, err)
			second, err := store.CommitConversation(context.Background(), all, executionvalues.ConversationCheckpoint{})
			require(t, err)
			starts, ends, commentary, finals, tools := 0, 0, 0, 0, 0
			for i, e := range first {
				if e.EventID != second[i].EventID {
					t.Fatal("replay duplicated a native event")
				}
				switch e.Type {
				case executionvalues.ConversationTurnStarted:
					starts++
				case executionvalues.ConversationTurnCompleted:
					ends++
					if e.FinalAvailability != executionvalues.FinalAvailable {
						t.Fatal("native final evidence lost")
					}
				case executionvalues.ConversationEventType("tool.completed"):
					tools++
				}
				if e.Type == executionvalues.ConversationMessageComplete {
					if e.Phase == executionvalues.MessagePhaseCommentary {
						commentary++
					}
					if e.Phase == executionvalues.MessagePhaseFinalAnswer {
						finals++
					}
					if e.Body == nil || e.ItemQuality != executionvalues.IdentityNative || e.TurnQuality != executionvalues.IdentityNative {
						t.Fatalf("native message identity/body lost: %+v", e)
					}
				}
			}
			if starts != 2 || ends != 2 || commentary != 0 || finals != 2 {
				t.Fatalf("native facts lost: starts=%d ends=%d commentary=%d finals=%d", starts, ends, commentary, finals)
			}
			if sample.Backend == executionbackend.BackendOrca && tools != 0 {
				t.Fatalf("Orca native tool events lost: %d", tools)
			}
		})
	}
}

func TestCodexHistoryRealLocalToolInterleaveFixture(t *testing.T) {
	raw := historicalConversationFixture(t, "pr-04b-local-tool-interleave-2026-09-25.json")
	var fixture struct {
		Thread codexworker.HistoryThread `json:"thread"`
	}
	require(t, json.Unmarshal(raw, &fixture))
	if len(fixture.Thread.Turns) != 1 {
		t.Fatal("real local tool fixture must contain exactly one turn")
	}
	base := executionvalues.ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalProcess, Agent: "codex", SourceSegment: "native", SourceIdentity: "native", SessionID: fixture.Thread.ID}
	writes, err := codexworker.MapHistoryTurn(base, fixture.Thread.Turns[0], 1)
	require(t, err)
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "history.db"))
	require(t, err)
	defer store.Close()
	first, err := store.CommitConversation(context.Background(), writes, executionvalues.ConversationCheckpoint{})
	require(t, err)
	second, err := store.CommitConversation(context.Background(), writes, executionvalues.ConversationCheckpoint{})
	require(t, err)
	if len(first) != 3 || len(second) != len(first) {
		t.Fatalf("real interleaved turn was not preserved: %+v", first)
	}
	for i := range first {
		if first[i].EventID != second[i].EventID || first[i].TurnID != fixture.Thread.Turns[0].ID {
			t.Fatalf("interleaved replay duplicated or reassigned event %d: %+v", i, first[i])
		}
	}
	if first[0].Type != executionvalues.ConversationTurnStarted || first[1].Type != executionvalues.ConversationMessageComplete || first[1].Phase != executionvalues.MessagePhaseFinalAnswer || first[2].Type != executionvalues.ConversationTurnCompleted || first[2].FinalAvailability != executionvalues.FinalAvailable {
		t.Fatalf("native final and turn facts lost: %+v", first)
	}
	query := productquery.NewConversationService(store, "project")
	if first[0].Body != nil || first[2].Body != nil || first[1].Body == nil || first[1].Body.Completeness != executionvalues.BodyComplete {
		t.Fatalf("process body retained or complete final missing: %+v", first)
	}
	publicEvent, err := productquery.ConversationEvent(first[1])
	require(t, err)
	body, err := query.ReadMessageBody(context.Background(), *publicEvent.Body, 0, 1024)
	require(t, err)
	if string(body.Data) != "PR04B_NATIVE_PROBE_ALPHA" || !body.Complete {
		t.Fatalf("real final changed body: %+v", body)
	}
}

// TestCodexHistoryRealSourceReplacement uses two exact isolated local probe
// sessions from one workspace. It is gated because CI has no such sessions.
func TestCodexHistoryRealCancelledTurnFixture(t *testing.T) {
	raw := historicalConversationFixture(t, "pr-04b-native-cancelled-2026-09-25.json")
	var fixture struct {
		Thread codexworker.HistoryThread `json:"thread"`
	}
	require(t, json.Unmarshal(raw, &fixture))
	if len(fixture.Thread.Turns) != 1 || fixture.Thread.Turns[0].CompletedAt == nil {
		t.Fatal("real cancelled turn lacks a native completion timestamp")
	}
	base := executionvalues.ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalProcess, Agent: "codex", SourceSegment: "native", SourceIdentity: "native", SessionID: fixture.Thread.ID}
	writes, err := codexworker.MapHistoryTurn(base, fixture.Thread.Turns[0], 1)
	require(t, err)
	if len(writes) != 3 || writes[0].Event.Type != executionvalues.ConversationTurnStarted || writes[1].Event.Type != executionvalues.ConversationTurnCancelled || writes[1].Event.Status != "interrupted" || writes[1].Event.FinalAvailability != executionvalues.FinalAbsent || writes[1].Event.TurnID != writes[0].Event.TurnID {
		t.Fatalf("real terminal cancellation or absent final was misrepresented: %+v", writes)
	}
}

func TestCodexHistoryRealFailedTurnFixture(t *testing.T) {
	raw := historicalConversationFixture(t, "pr-04b-native-failed-2026-09-25.json")
	var fixture struct {
		Thread codexworker.HistoryThread `json:"thread"`
	}
	require(t, json.Unmarshal(raw, &fixture))
	if len(fixture.Thread.Turns) != 1 || fixture.Thread.Turns[0].CompletedAt == nil || fixture.Thread.Turns[0].Status != "failed" {
		t.Fatal("real failed turn lacks native terminal evidence")
	}
	base := executionvalues.ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalProcess, Agent: "codex", SourceSegment: "native", SourceIdentity: "native", SessionID: fixture.Thread.ID}
	writes, err := codexworker.MapHistoryTurn(base, fixture.Thread.Turns[0], 1)
	require(t, err)
	if len(writes) != 3 || writes[0].Event.Type != executionvalues.ConversationTurnStarted || writes[1].Event.Type != executionvalues.ConversationTurnFailed || writes[1].Event.Status != "failed" || writes[1].Event.FinalAvailability != executionvalues.FinalAbsent {
		t.Fatalf("real native failure or missing final was misrepresented: %+v", writes)
	}
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "failure.db"))
	require(t, err)
	defer store.Close()
	events, err := store.CommitConversation(context.Background(), writes, executionvalues.ConversationCheckpoint{})
	require(t, err)
	if events[1].Diagnostic == "" {
		t.Fatal("native failure lost its bounded diagnostic")
	}
}

// TestCodexHistoryRealFailedAcceptance reads an exact isolated unauthenticated
// session through the production public-history collector.
func TestCodexHistoryTurnCompletionAndLateFinal(t *testing.T) {
	base := executionvalues.ConversationEvent{ProjectID: "p", ExecutionID: "e", AttemptID: "a", SourceSegment: "s", SessionID: "session"}
	completedAt := int64(1)
	turn := codexworker.HistoryTurn{ID: "turn", Status: "completed", CompletedAt: &completedAt, Items: []json.RawMessage{json.RawMessage(`{"id":"message","type":"agentMessage","text":"unknown"}`)}}
	first, err := codexworker.MapHistoryTurn(base, turn, 1)
	require(t, err)
	if first[len(first)-1].Event.FinalAvailability != executionvalues.FinalUnknown {
		t.Fatal("missing phase was called an absent final")
	}
	turn.Items = append(turn.Items, json.RawMessage(`{"id":"final","type":"agentMessage","phase":"final_answer","text":"late final"}`))
	late, err := codexworker.MapHistoryTurn(base, turn, 10)
	require(t, err)
	if late[len(late)-1].Event.FinalAvailability != executionvalues.FinalAvailable || late[len(late)-1].Event.SourceEventID == first[len(first)-1].Event.SourceEventID {
		t.Fatal("late native final cannot reconcile its original turn")
	}
	turn.Status = "interrupted"
	cancelled, err := codexworker.MapHistoryTurn(base, turn, 20)
	require(t, err)
	if cancelled[len(cancelled)-1].Event.Type != executionvalues.ConversationTurnCancelled {
		t.Fatal("interrupted native turn was not cancelled")
	}
	turn.CompletedAt = nil
	active, err := codexworker.MapHistoryTurn(base, turn, 30)
	require(t, err)
	for _, write := range active {
		if write.Event.Type == executionvalues.ConversationTurnCancelled || write.Event.Type == executionvalues.ConversationTurnCompleted {
			t.Fatal("offline history's synthetic interrupted status became a terminal event")
		}
	}
}

func TestCodexHistoryMissingTextDoesNotClaimFinalBody(t *testing.T) {
	base := executionvalues.ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", SourceSegment: "native", SessionID: "session"}
	completedAt := int64(1)
	missing := codexworker.HistoryTurn{ID: "missing-turn", Status: "completed", CompletedAt: &completedAt, Items: []json.RawMessage{json.RawMessage(`{"id":"missing-final","type":"agentMessage","phase":"final_answer"}`)}}
	writes, err := codexworker.MapHistoryTurn(base, missing, 1)
	require(t, err)
	if len(writes) != 3 || writes[1].BodyMode != "" || writes[1].Event.FinalAvailability != executionvalues.FinalUnknown || writes[2].Event.FinalAvailability != executionvalues.FinalUnknown {
		t.Fatalf("native final without text was claimed as complete: %+v", writes)
	}
	empty := codexworker.HistoryTurn{ID: "empty-turn", Status: "completed", CompletedAt: &completedAt, Items: []json.RawMessage{json.RawMessage(`{"id":"empty-final","type":"agentMessage","phase":"final_answer","text":""}`)}}
	writes, err = codexworker.MapHistoryTurn(base, empty, 10)
	require(t, err)
	if len(writes) != 3 || writes[1].BodyMode != "complete" || writes[1].Event.FinalAvailability != executionvalues.FinalAvailable || writes[2].Event.FinalAvailability != executionvalues.FinalAvailable {
		t.Fatalf("explicit empty native final was not complete: %+v", writes)
	}
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "history.db"))
	require(t, err)
	defer store.Close()
	events, err := store.CommitConversation(context.Background(), writes, executionvalues.ConversationCheckpoint{})
	require(t, err)
	if events[1].Body == nil || events[1].Body.Bytes != 0 || events[1].Body.Completeness != executionvalues.BodyComplete {
		t.Fatalf("explicit empty native final lacks a readable body reference: %+v", events[1])
	}
}

func TestCodexHistoryBindingCannotChangeAttempt(t *testing.T) {
	b := codexworker.HistoryBinding{Version: 1, ProjectID: "p", WorkflowID: "w", ExecutionID: "e", AttemptID: "a", Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalProcess, TargetID: "local", Workspace: t.TempDir(), SessionID: "00000000-0000-0000-0000-000000000001", Command: executionbackend.Command{Executable: "codex"}}
	path := filepath.Join(t.TempDir(), "codex-history.v1.json")
	require(t, codexworker.WriteHistoryBinding(path, b, steerline.AtomicWrite))
	b.AttemptID = "replacement"
	if err := codexworker.WriteHistoryBinding(path, b, steerline.AtomicWrite); err == nil {
		t.Fatal("a native session binding crossed attempts")
	}
}

func conversationItemKey(event executionvalues.ConversationEvent, item string) string {
	event.ItemID = item
	digest := sha256.Sum256([]byte(event.ItemIdentity()))
	return hex.EncodeToString(digest[:])
}

func conversationTestWrite(sourceID string, sequence uint64, mode string, body, raw []byte) executionvalues.ConversationWrite {
	event := executionvalues.ConversationEvent{Version: executionvalues.ConversationVersion, ProjectID: "project", TaskID: "task", WorkflowID: "workflow", ExecutionID: "execution", AttemptID: "attempt", Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalProcess, Agent: "codex", TargetID: "local-test", Source: "codex-jsonl", SourceRole: executionvalues.ConversationSourceFallback, SourceSegment: "segment-1", SessionID: "session", SessionQuality: executionvalues.IdentityNative, TurnID: "turn", TurnQuality: executionvalues.IdentityDerived, ItemID: "same-item", ItemQuality: executionvalues.IdentityNative, SourceEventID: sourceID, SourceEventQuality: executionvalues.IdentityDerived, SourceSeq: sequence, Type: executionvalues.ConversationMessageComplete, Phase: executionvalues.MessagePhaseFinalAnswer, FinalAvailability: executionvalues.FinalAvailable, ReceivedAt: time.Now().UTC()}
	event.ItemKey = conversationItemKey(event, event.ItemID)
	return executionvalues.ConversationWrite{Event: event, Body: body, BodyMode: mode, BodySource: "test-source"}
}
