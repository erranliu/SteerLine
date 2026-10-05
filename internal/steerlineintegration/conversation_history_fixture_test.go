package steerlineintegration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
)

func TestCodexHistoryNativeFixtures(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "validation", "pr-04b-public-history-native-2026-09-25.json"))
	require(t, err)
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
			base := steerline.ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Backend: sample.Backend, Protocol: executionbackend.ProtocolLocalProcess, Agent: "codex", Source: "codex-app-server", SourceSegment: "native", SourceIdentity: "native", SessionID: sample.Thread.ID}
			if sample.Backend == executionbackend.BackendOrca {
				base.Protocol = executionbackend.ProtocolOrcaTerminalV2
			}
			var all []steerline.ConversationWrite
			for _, turn := range sample.Thread.Turns {
				writes, mapErr := codexworker.MapHistoryTurn(base, turn, uint64(len(all)+1))
				require(t, mapErr)
				all = append(all, writes...)
			}
			first, err := store.CommitConversation(context.Background(), all, steerline.ConversationCheckpoint{})
			require(t, err)
			second, err := store.CommitConversation(context.Background(), all, steerline.ConversationCheckpoint{})
			require(t, err)
			starts, ends, commentary, finals, tools := 0, 0, 0, 0, 0
			for i, e := range first {
				if e.EventID != second[i].EventID {
					t.Fatal("replay duplicated a native event")
				}
				switch e.Type {
				case steerline.ConversationTurnStarted:
					starts++
				case steerline.ConversationTurnCompleted:
					ends++
					if e.FinalAvailability != steerline.FinalAvailable {
						t.Fatal("native final evidence lost")
					}
				case steerline.ConversationToolCompleted:
					tools++
				}
				if e.Type == steerline.ConversationMessageComplete {
					if e.Phase == steerline.MessagePhaseCommentary {
						commentary++
					}
					if e.Phase == steerline.MessagePhaseFinalAnswer {
						finals++
					}
					if e.Body == nil || e.ItemQuality != steerline.IdentityNative || e.TurnQuality != steerline.IdentityNative {
						t.Fatalf("native message identity/body lost: %+v", e)
					}
				}
			}
			if starts != 2 || ends != 2 || commentary < 1 || finals != 2 {
				t.Fatalf("native facts lost: starts=%d ends=%d commentary=%d finals=%d", starts, ends, commentary, finals)
			}
			if sample.Backend == executionbackend.BackendOrca && tools < 2 {
				t.Fatalf("Orca native tool events lost: %d", tools)
			}
		})
	}
}

func TestCodexHistoryRealLocalToolInterleaveFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "validation", "pr-04b-local-tool-interleave-2026-09-25.json"))
	require(t, err)
	var fixture struct {
		Thread codexworker.HistoryThread `json:"thread"`
	}
	require(t, json.Unmarshal(raw, &fixture))
	if len(fixture.Thread.Turns) != 1 {
		t.Fatal("real local tool fixture must contain exactly one turn")
	}
	base := steerline.ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalProcess, Agent: "codex", SourceSegment: "native", SourceIdentity: "native", SessionID: fixture.Thread.ID}
	writes, err := codexworker.MapHistoryTurn(base, fixture.Thread.Turns[0], 1)
	require(t, err)
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "history.db"))
	require(t, err)
	defer store.Close()
	first, err := store.CommitConversation(context.Background(), writes, steerline.ConversationCheckpoint{})
	require(t, err)
	second, err := store.CommitConversation(context.Background(), writes, steerline.ConversationCheckpoint{})
	require(t, err)
	if len(first) != 6 || len(second) != len(first) {
		t.Fatalf("real interleaved turn was not preserved: %+v", first)
	}
	for i := range first {
		if first[i].EventID != second[i].EventID || first[i].TurnID != fixture.Thread.Turns[0].ID {
			t.Fatalf("interleaved replay duplicated or reassigned event %d: %+v", i, first[i])
		}
	}
	if first[0].Type != steerline.ConversationTurnStarted || first[2].Type != steerline.ConversationMessageComplete || first[2].Phase != steerline.MessagePhaseCommentary || first[3].Type != steerline.ConversationToolCompleted || first[3].Status != "completed" || first[4].Type != steerline.ConversationMessageComplete || first[4].Phase != steerline.MessagePhaseFinalAnswer || first[5].Type != steerline.ConversationTurnCompleted || first[5].FinalAvailability != steerline.FinalAvailable {
		t.Fatalf("real commentary/tool/final order or turn completion was lost: %+v", first)
	}
	query := steerline.NewConversationQueryService(store, "project")
	for _, tc := range []struct {
		index int
		want  string
	}{{2, "I’ll read `probe.txt` from the workspace and return its marker unchanged."}, {3, "PR04B_NATIVE_PROBE_ALPHA\r\n"}, {4, "PR04B_NATIVE_PROBE_ALPHA"}} {
		if first[tc.index].Body == nil || first[tc.index].Body.Completeness != steerline.BodyComplete {
			t.Fatalf("real item %d lacks complete body: %+v", tc.index, first[tc.index])
		}
		body, err := query.ReadMessageBody(context.Background(), *first[tc.index].Body, 0, 1024)
		require(t, err)
		if string(body.Data) != tc.want || !body.Complete {
			t.Fatalf("real item %d changed body: %+v", tc.index, body)
		}
	}
}

// TestCodexHistoryRealSourceReplacement uses two exact isolated local probe
// sessions from one workspace. It is gated because CI has no such sessions.
func TestCodexHistoryRealCancelledTurnFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "validation", "pr-04b-native-cancelled-2026-09-25.json"))
	require(t, err)
	var fixture struct {
		Thread codexworker.HistoryThread `json:"thread"`
	}
	require(t, json.Unmarshal(raw, &fixture))
	if len(fixture.Thread.Turns) != 1 || fixture.Thread.Turns[0].CompletedAt == nil {
		t.Fatal("real cancelled turn lacks a native completion timestamp")
	}
	base := steerline.ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalProcess, Agent: "codex", SourceSegment: "native", SourceIdentity: "native", SessionID: fixture.Thread.ID}
	writes, err := codexworker.MapHistoryTurn(base, fixture.Thread.Turns[0], 1)
	require(t, err)
	if len(writes) != 2 || writes[0].Event.Type != steerline.ConversationTurnStarted || writes[1].Event.Type != steerline.ConversationTurnCancelled || writes[1].Event.Status != "interrupted" || writes[1].Event.FinalAvailability != steerline.FinalAbsent || writes[1].Event.TurnID != writes[0].Event.TurnID {
		t.Fatalf("real terminal cancellation or absent final was misrepresented: %+v", writes)
	}
}

func TestCodexHistoryRealFailedTurnFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "validation", "pr-04b-native-failed-2026-09-25.json"))
	require(t, err)
	var fixture struct {
		Thread codexworker.HistoryThread `json:"thread"`
	}
	require(t, json.Unmarshal(raw, &fixture))
	if len(fixture.Thread.Turns) != 1 || fixture.Thread.Turns[0].CompletedAt == nil || fixture.Thread.Turns[0].Status != "failed" {
		t.Fatal("real failed turn lacks native terminal evidence")
	}
	base := steerline.ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalProcess, Agent: "codex", SourceSegment: "native", SourceIdentity: "native", SessionID: fixture.Thread.ID}
	writes, err := codexworker.MapHistoryTurn(base, fixture.Thread.Turns[0], 1)
	require(t, err)
	if len(writes) != 3 || writes[0].Event.Type != steerline.ConversationTurnStarted || writes[1].Event.Type != steerline.ConversationObservation || writes[2].Event.Type != steerline.ConversationTurnFailed || writes[2].Event.Status != "failed" || writes[2].Event.FinalAvailability != steerline.FinalAbsent {
		t.Fatalf("real native failure or missing final was misrepresented: %+v", writes)
	}
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "failure.db"))
	require(t, err)
	defer store.Close()
	events, err := store.CommitConversation(context.Background(), writes, steerline.ConversationCheckpoint{})
	require(t, err)
	if events[2].Raw == nil {
		t.Fatal("real failure lost its raw error evidence")
	}
	page, err := steerline.NewConversationQueryService(store, "project").ReadRaw(context.Background(), *events[2].Raw, 0, 4096)
	require(t, err)
	if !page.Complete || !strings.Contains(string(page.Data), "401 Unauthorized") {
		t.Fatalf("real failure error evidence changed: %+v", page)
	}
}

// TestCodexHistoryRealFailedAcceptance reads an exact isolated unauthenticated
// session through the production public-history collector.
func TestCodexHistoryTurnCompletionAndLateFinal(t *testing.T) {
	base := steerline.ConversationEvent{ProjectID: "p", ExecutionID: "e", AttemptID: "a", SourceSegment: "s", SessionID: "session"}
	completedAt := int64(1)
	turn := codexworker.HistoryTurn{ID: "turn", Status: "completed", CompletedAt: &completedAt, Items: []json.RawMessage{json.RawMessage(`{"id":"message","type":"agentMessage","text":"unknown"}`)}}
	first, err := codexworker.MapHistoryTurn(base, turn, 1)
	require(t, err)
	if first[len(first)-1].Event.FinalAvailability != steerline.FinalUnknown {
		t.Fatal("missing phase was called an absent final")
	}
	turn.Items = append(turn.Items, json.RawMessage(`{"id":"final","type":"agentMessage","phase":"final_answer","text":"late final"}`))
	late, err := codexworker.MapHistoryTurn(base, turn, 10)
	require(t, err)
	if late[len(late)-1].Event.FinalAvailability != steerline.FinalAvailable || late[len(late)-1].Event.SourceEventID == first[len(first)-1].Event.SourceEventID {
		t.Fatal("late native final cannot reconcile its original turn")
	}
	turn.Status = "interrupted"
	cancelled, err := codexworker.MapHistoryTurn(base, turn, 20)
	require(t, err)
	if cancelled[len(cancelled)-1].Event.Type != steerline.ConversationTurnCancelled {
		t.Fatal("interrupted native turn was not cancelled")
	}
	turn.CompletedAt = nil
	active, err := codexworker.MapHistoryTurn(base, turn, 30)
	require(t, err)
	for _, write := range active {
		if write.Event.Type == steerline.ConversationTurnCancelled || write.Event.Type == steerline.ConversationTurnCompleted {
			t.Fatal("offline history's synthetic interrupted status became a terminal event")
		}
	}
}

func TestCodexHistoryMissingTextDoesNotClaimFinalBody(t *testing.T) {
	base := steerline.ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", SourceSegment: "native", SessionID: "session"}
	completedAt := int64(1)
	missing := codexworker.HistoryTurn{ID: "missing-turn", Status: "completed", CompletedAt: &completedAt, Items: []json.RawMessage{json.RawMessage(`{"id":"missing-final","type":"agentMessage","phase":"final_answer"}`)}}
	writes, err := codexworker.MapHistoryTurn(base, missing, 1)
	require(t, err)
	if len(writes) != 3 || writes[1].BodyMode != "" || writes[1].Event.FinalAvailability != steerline.FinalUnknown || writes[2].Event.FinalAvailability != steerline.FinalUnknown {
		t.Fatalf("native final without text was claimed as complete: %+v", writes)
	}
	empty := codexworker.HistoryTurn{ID: "empty-turn", Status: "completed", CompletedAt: &completedAt, Items: []json.RawMessage{json.RawMessage(`{"id":"empty-final","type":"agentMessage","phase":"final_answer","text":""}`)}}
	writes, err = codexworker.MapHistoryTurn(base, empty, 10)
	require(t, err)
	if len(writes) != 3 || writes[1].BodyMode != "complete" || writes[1].Event.FinalAvailability != steerline.FinalAvailable || writes[2].Event.FinalAvailability != steerline.FinalAvailable {
		t.Fatalf("explicit empty native final was not complete: %+v", writes)
	}
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "history.db"))
	require(t, err)
	defer store.Close()
	events, err := store.CommitConversation(context.Background(), writes, steerline.ConversationCheckpoint{})
	require(t, err)
	if events[1].Body == nil || events[1].Body.Bytes != 0 || events[1].Body.Completeness != steerline.BodyComplete {
		t.Fatalf("explicit empty native final lacks a readable body reference: %+v", events[1])
	}
}

func TestCodexHistoryCanonicalSourceKeepsRawReadable(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "history.db"))
	require(t, err)
	defer store.Close()
	raw := conversationTestWrite("bytes:0:4", 1, "complete", []byte("raw reply"), []byte("raw"))
	_, err = store.CommitConversation(ctx, []steerline.ConversationWrite{raw}, steerline.ConversationCheckpoint{})
	require(t, err)
	native := raw.Event
	native.SourceRole = contracts.ConversationSourceCanonical
	native.Source, native.SourceSegment, native.SourceEventID = "codex-app-server", "native", "session"
	native.Type, native.ItemKey = steerline.ConversationSession, ""
	_, err = store.CommitConversation(ctx, []steerline.ConversationWrite{{Event: native}}, steerline.ConversationCheckpoint{})
	require(t, err)
	query := steerline.NewConversationQueryService(store, "project")
	page, err := query.ReadConversation(ctx, steerline.ConversationFilter{}, "", 100)
	require(t, err)
	if len(page.Events) != 1 || page.Events[0].Source != "codex-app-server" {
		t.Fatal("raw and native message projections were both displayed")
	}
	rawPage, err := query.ReadConversation(ctx, steerline.ConversationFilter{Source: "codex-jsonl"}, "", 100)
	require(t, err)
	if len(rawPage.Events) != 1 || rawPage.Events[0].Raw == nil {
		t.Fatal("raw source evidence became inaccessible")
	}
	other := raw
	other.Event.AttemptID = "other-attempt"
	other.Event.ItemKey = conversationItemKey(other.Event, other.Event.ItemID)
	_, err = store.CommitConversation(ctx, []steerline.ConversationWrite{other}, steerline.ConversationCheckpoint{})
	require(t, err)
	page, err = query.ReadConversation(ctx, steerline.ConversationFilter{}, "", 100)
	require(t, err)
	if len(page.Events) != 2 {
		t.Fatal("native source selection crossed attempt boundaries")
	}
	gap := raw.Event
	gap.Type, gap.SourceEventID, gap.ItemKey = steerline.ConversationGap, "source-changed", ""
	wait := gap
	wait.Source, wait.Type, wait.SourceEventID = "orca-terminal-v2", steerline.ConversationInputWait, "host-wait"
	_, err = store.CommitConversation(ctx, []steerline.ConversationWrite{{Event: gap}, {Event: wait}}, steerline.ConversationCheckpoint{})
	require(t, err)
	page, err = query.ReadConversation(ctx, steerline.ConversationFilter{}, "", 100)
	require(t, err)
	if len(page.Events) != 4 || page.Events[2].Type != steerline.ConversationGap || page.Events[3].Type != steerline.ConversationInputWait {
		t.Fatal("native source selection hid a gap or host wait")
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

func conversationItemKey(event steerline.ConversationEvent, item string) string {
	event.ItemID = item
	digest := sha256.Sum256([]byte(event.ItemIdentity()))
	return hex.EncodeToString(digest[:])
}

func conversationTestWrite(sourceID string, sequence uint64, mode string, body, raw []byte) steerline.ConversationWrite {
	event := steerline.ConversationEvent{Version: steerline.ConversationVersion, ProjectID: "project", TaskID: "task", WorkflowID: "workflow", ExecutionID: "execution", AttemptID: "attempt", Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalProcess, Agent: "codex", TargetID: "local-test", Source: "codex-jsonl", SourceRole: contracts.ConversationSourceFallback, SourceSegment: "segment-1", SessionID: "session", SessionQuality: steerline.IdentityNative, TurnID: "turn", TurnQuality: steerline.IdentityDerived, ItemID: "same-item", ItemQuality: steerline.IdentityNative, SourceEventID: sourceID, SourceEventQuality: steerline.IdentityDerived, SourceSeq: sequence, Type: steerline.ConversationMessageDelta, Phase: steerline.MessagePhaseUnknown, ReceivedAt: time.Now().UTC()}
	event.ItemKey = conversationItemKey(event, event.ItemID)
	return steerline.ConversationWrite{Event: event, Body: body, BodyMode: mode, Raw: raw, BodySource: "test-source"}
}
