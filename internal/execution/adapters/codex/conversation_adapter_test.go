package codexworker

import (
	"bufio"
	"context"
	"encoding/json"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestConversationHistoryProtocolHelper(t *testing.T) {
	if os.Getenv("STEERLINE_CONVERSATION_HISTORY_MOCK") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Cursor   string `json:"cursor"`
				ThreadID string `json:"threadId"`
			} `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			os.Exit(2)
		}
		if request.ID == 0 {
			continue
		}
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"userAgent": "history-fixture/1", "codexHome": EnvironmentHome(), "platformOs": "windows"}
		case "thread/read":
			result = map[string]any{"thread": map[string]any{"id": request.Params.ThreadID, "cwd": os.Getenv("STEERLINE_CONVERSATION_HISTORY_WORKSPACE")}}
		case "thread/turns/list":
			if os.Getenv("STEERLINE_CONVERSATION_HISTORY_STAGE") == "cursor-too-long" {
				result = map[string]any{"data": []any{}, "nextCursor": nil, "backwardsCursor": strings.Repeat("x", MaxHistoryCursorBytes+1)}
				break
			}
			first := historyFixtureTurn("first-turn", "first-message", "commentary", "progress")
			if os.Getenv("STEERLINE_CONVERSATION_HISTORY_STAGE") == "late" {
				first["items"] = append(first["items"].([]any), map[string]any{"id": "late-final", "type": "agentMessage", "phase": "final_answer", "text": "late final"})
			}
			second := historyFixtureTurn("second-turn", "second-final", "final_answer", "second final")
			switch request.Params.Cursor {
			case "", "first-anchor":
				result = map[string]any{"data": []any{first}, "nextCursor": "after-first", "backwardsCursor": "first-anchor"}
			case "after-first", "second-anchor":
				result = map[string]any{"data": []any{second}, "nextCursor": nil, "backwardsCursor": "second-anchor"}
			default:
				os.Exit(3)
			}
		default:
			os.Exit(4)
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"id": request.ID, "result": result}); err != nil {
			os.Exit(5)
		}
	}
	os.Exit(0)
}

func TestHistoryDiscoveryRejectsWorkspaceReplacementAndKeepsSourceAttempts(t *testing.T) {
	root, workspace, wrongWorkspace := t.TempDir(), t.TempDir(), t.TempDir()
	workflowID, executionID, attemptID := strings.Repeat("4", 32), strings.Repeat("5", 32), strings.Repeat("6", 32)
	runDirectory := filepath.Join(root, "runs", workflowID)
	if err := os.MkdirAll(runDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	eventPath := filepath.Join(runDirectory, "events.jsonl")
	if err := os.WriteFile(eventPath, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	binding := historyFixtureBinding(workspace, workflowID, executionID, attemptID, "00000000-0000-0000-0000-000000000001")
	path := localConversationHistoryBindingPath(runDirectory, binding.SessionID)
	if err := WriteHistoryBinding(path, binding, func(path string, data []byte) error { return os.WriteFile(path, data, 0600) }); err != nil {
		t.Fatal(err)
	}
	base := executionvalues.ConversationEvent{Version: executionvalues.ConversationVersion, ProjectID: binding.ProjectID, TaskID: binding.TaskID, WorkflowID: workflowID,
		ExecutionID: executionID, Backend: binding.Backend, Protocol: binding.Protocol, Agent: "codex", TargetID: binding.TargetID}
	discovery := conversationtypes.ConversationDiscovery{ProjectID: binding.ProjectID, RunDirectory: root, Runs: []conversationtypes.ConversationRunSource{{Base: base, RunDirectory: runDirectory, Workspace: wrongWorkspace,
		Attempts: []conversationtypes.ConversationAttemptSource{{AttemptID: attemptID, EventPath: eventPath}}}}}
	port := NewConversationAdapter()
	wrong, err := port.DiscoverHistory(context.Background(), discovery)
	if err != nil || len(wrong.Sources) != 0 || len(wrong.Writes) != 1 || wrong.Writes[0].Event.Type != executionvalues.ConversationGap {
		t.Fatalf("replacement workspace was discovered: sources=%+v writes=%+v err=%v", wrong.Sources, wrong.Writes, err)
	}
	discovery.Runs[0].Workspace = workspace
	second := historyFixtureBinding(workspace, workflowID, executionID, attemptID, "00000000-0000-0000-0000-000000000002")
	secondPath := localConversationHistoryBindingPath(runDirectory, second.SessionID)
	if err = WriteHistoryBinding(secondPath, second, func(path string, data []byte) error { return os.WriteFile(path, data, 0600) }); err != nil {
		t.Fatal(err)
	}
	result, err := port.DiscoverHistory(context.Background(), discovery)
	if err != nil || len(result.Sources) != 2 || result.Sources[0].Event.AttemptID != attemptID || result.Sources[1].Event.AttemptID != attemptID ||
		result.Sources[0].Event.SessionID == result.Sources[1].Event.SessionID {
		t.Fatalf("session replacement lost its original attempt source: %+v err=%v", result, err)
	}
}

func TestHistoryAdapterRejectsOversizedNativeCursor(t *testing.T) {
	workspace, bindingsDir := t.TempDir(), t.TempDir()
	t.Setenv("STEERLINE_CONVERSATION_HISTORY_MOCK", "1")
	t.Setenv("STEERLINE_CONVERSATION_HISTORY_WORKSPACE", workspace)
	t.Setenv("STEERLINE_CONVERSATION_HISTORY_STAGE", "cursor-too-long")
	binding := historyFixtureBinding(workspace, strings.Repeat("7", 32), strings.Repeat("8", 32), strings.Repeat("9", 32), "00000000-0000-0000-0000-000000000003")
	path := filepath.Join(bindingsDir, "binding.json")
	if err := WriteHistoryBinding(path, binding, func(path string, data []byte) error { return os.WriteFile(path, data, 0600) }); err != nil {
		t.Fatal(err)
	}
	source := conversationtypes.ConversationHistorySource{Identity: binding.Identity() + ":" + binding.AttemptID, BindingPath: path, Workspace: workspace, Event: binding.Event()}
	_, err := NewConversationAdapter().ReadHistory(context.Background(), conversationtypes.ConversationHistoryReadRequest{Source: source}, startHistoryFixtureProcess)
	if err == nil || !strings.Contains(err.Error(), "bounded source limit") {
		t.Fatalf("oversized native cursor was accepted: %v", err)
	}
}

func TestHistoryPageCandidateIsRecreatedAfterDiscardAndLateFinalKeepsTurnIdentity(t *testing.T) {
	workspace, bindingsDir := t.TempDir(), t.TempDir()
	t.Setenv("STEERLINE_CONVERSATION_HISTORY_MOCK", "1")
	t.Setenv("STEERLINE_CONVERSATION_HISTORY_WORKSPACE", workspace)
	t.Setenv("STEERLINE_CONVERSATION_HISTORY_STAGE", "initial")
	binding := HistoryBinding{Version: 1, ProjectID: "project", TaskID: "task", WorkflowID: "workflow", ExecutionID: "execution", AttemptID: "attempt",
		Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalProcess, TargetID: "local", Workspace: workspace,
		SessionID: "00000000-0000-0000-0000-000000000001", Command: executionbackend.Command{Executable: os.Args[0], Args: []string{"-test.run=^TestConversationHistoryProtocolHelper$"}}}
	path := filepath.Join(bindingsDir, "binding.json")
	if err := WriteHistoryBinding(path, binding, func(path string, data []byte) error { return os.WriteFile(path, data, 0600) }); err != nil {
		t.Fatal(err)
	}
	source := conversationtypes.ConversationHistorySource{Identity: binding.Identity() + ":" + binding.AttemptID, BindingPath: path, Workspace: workspace, Event: binding.Event()}
	port := &ConversationAdapter{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	first, err := port.ReadHistory(ctx, conversationtypes.ConversationHistoryReadRequest{Source: source}, startHistoryFixtureProcess)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := port.ReadHistory(ctx, conversationtypes.ConversationHistoryReadRequest{Source: source}, startHistoryFixtureProcess)
	if err != nil {
		t.Fatal(err)
	}
	if first.Cursor != retry.Cursor || len(first.Writes) != len(retry.Writes) {
		t.Fatal("discarded history mapper candidate changed the batch or candidate cursor")
	}
	for i := range first.Writes {
		if first.Writes[i].Event.SourceEventID != retry.Writes[i].Event.SourceEventID || first.Writes[i].Event.ItemID != retry.Writes[i].Event.ItemID {
			t.Fatal("retry changed a native event identity")
		}
	}
	second, err := port.ReadHistory(ctx, conversationtypes.ConversationHistoryReadRequest{Source: source, Cursor: retry.Cursor, HasCheckpoint: true}, startHistoryFixtureProcess)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := decodeHistoryPosition(binding, second.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(cursor.Pending) != 1 || cursor.Pending[0].ID != "first-turn" {
		t.Fatalf("final-less native turn was not held for later reconciliation: %+v", cursor.Pending)
	}
	t.Setenv("STEERLINE_CONVERSATION_HISTORY_STAGE", "late")
	resolved, err := port.ReadHistory(ctx, conversationtypes.ConversationHistoryReadRequest{Source: source, Cursor: second.Cursor, HasCheckpoint: true}, startHistoryFixtureProcess)
	if err != nil {
		t.Fatal(err)
	}
	finalCursor, err := decodeHistoryPosition(binding, resolved.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(finalCursor.Pending) != 0 {
		t.Fatal("late final left the original turn pending")
	}
	finals := 0
	for _, write := range resolved.Writes {
		if write.Event.TurnID == "first-turn" && write.Event.Phase == executionvalues.MessagePhaseFinalAnswer && write.Event.Type == executionvalues.ConversationMessageComplete {
			finals++
		}
	}
	if finals != 1 {
		t.Fatalf("late final was not mapped once to the original turn: %+v", resolved.Writes)
	}
}

type historyFixtureStop struct{ process *os.Process }

func (p historyFixtureStop) Identity() executionbackend.ProcessIdentity {
	return executionbackend.ProcessIdentity{Kind: "fixture", PID: p.process.Pid, Created: "history-fixture"}
}

func (p historyFixtureStop) Kill() { _ = p.process.Kill() }

func (p historyFixtureStop) Close() {}

func startHistoryFixtureProcess(command *exec.Cmd) (executionbackend.StopHandle, error) {
	if err := command.Start(); err != nil {
		return nil, err
	}
	return historyFixtureStop{process: command.Process}, nil
}

func historyFixtureTurn(turnID, itemID, phase, text string) map[string]any {
	return map[string]any{"id": turnID, "itemsView": "full", "status": "completed", "completedAt": 1,
		"items": []any{map[string]any{"id": itemID, "type": "agentMessage", "phase": phase, "text": text}}}
}

func historyFixtureBinding(workspace, workflow, execution, attempt, session string) HistoryBinding {
	return HistoryBinding{Version: 1, ProjectID: "project", TaskID: "task", WorkflowID: workflow, ExecutionID: execution, AttemptID: attempt,
		Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalProcess, TargetID: "local", Workspace: workspace,
		SessionID: session, Command: executionbackend.Command{Executable: os.Args[0], Args: []string{"-test.run=^TestConversationHistoryProtocolHelper$"}}}
}

func TestHistoryPositionCannotCrossItsOriginalBinding(t *testing.T) {
	binding := historyFixtureBinding(t.TempDir(), strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32), "00000000-0000-0000-0000-000000000005")
	cursor := codexHistoryCursor{Cursor: "native-anchor", Sequence: 3, Pending: []codexPendingTurn{{ID: "turn", Cursor: "original-anchor", Digest: "item-digest"}}}
	encoded, err := encodeHistoryPosition(binding, cursor)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := decodeHistoryPosition(binding, encoded)
	if err != nil || restored.Sequence != cursor.Sequence || restored.Pending[0].Cursor != cursor.Pending[0].Cursor {
		t.Fatalf("position did not retain its original pending anchor: %+v %v", restored, err)
	}
	for _, field := range []string{"project", "execution", "attempt", "target", "workspace", "command"} {
		t.Run(field, func(t *testing.T) {
			changed := binding
			switch field {
			case "project":
				changed.ProjectID = "other"
			case "execution":
				changed.ExecutionID = "other"
			case "attempt":
				changed.AttemptID = "other"
			case "target":
				changed.TargetID = "other"
			case "workspace":
				changed.Workspace = t.TempDir()
			case "command":
				changed.Command.Args = append(append([]string(nil), binding.Command.Args...), "changed")
			}
			if _, err := decodeHistoryPosition(changed, encoded); err == nil {
				t.Fatal("position moved to a different source owner")
			}
		})
	}
	for _, field := range []string{"version", "codec", "purpose", "digest", "pending"} {
		t.Run(field, func(t *testing.T) {
			var value historyPositionEnvelope
			if err := json.Unmarshal([]byte(encoded), &value); err != nil {
				t.Fatal(err)
			}
			switch field {
			case "version":
				value.Version++
			case "codec":
				value.CodecVersion++
			case "purpose":
				value.Purpose = "profile-config"
			case "digest":
				value.Payload.Sequence++
			case "pending":
				value.Payload.NextPending = -1
			}
			if field != "digest" {
				value.Digest = ""
				value.Digest = digestValue(value)
			}
			data, _ := json.Marshal(value)
			if _, err := decodeHistoryPosition(binding, string(data)); err == nil {
				t.Fatal("invalid position metadata or pending anchor was accepted")
			}
		})
	}
	if _, err := decodeHistoryPosition(binding, `{"cursor":"old-unscoped-anchor"}`); err == nil {
		t.Fatal("old unscoped cursor was accepted")
	}
}

func TestHistoryReadRejectsReplacedProjectBeforeOpeningClient(t *testing.T) {
	binding := historyFixtureBinding(t.TempDir(), strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32), "00000000-0000-0000-0000-000000000006")
	path := filepath.Join(t.TempDir(), "binding.json")
	if err := WriteHistoryBinding(path, binding, func(path string, data []byte) error { return os.WriteFile(path, data, 0600) }); err != nil {
		t.Fatal(err)
	}
	source := conversationtypes.ConversationHistorySource{Identity: binding.Identity() + ":" + binding.AttemptID, BindingPath: path, Workspace: binding.Workspace, Event: binding.Event()}
	binding.ProjectID = "different-project"
	data, _ := json.Marshal(binding)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := NewConversationAdapter().ReadHistory(context.Background(), conversationtypes.ConversationHistoryReadRequest{Source: source}, func(*exec.Cmd) (executionbackend.StopHandle, error) {
		t.Fatal("replaced source opened a reader process")
		return nil, nil
	})
	if err == nil {
		t.Fatal("replaced project binding was accepted")
	}
}
