package steerline

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func TestCodexHistoryServerHelper(t *testing.T) {
	if os.Getenv("STEERLINE_CODEX_HISTORY_MOCK") != "1" {
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
		result := any(map[string]any{})
		switch request.Method {
		case "initialize":
			result = map[string]any{"userAgent": "mock-codex/1", "codexHome": codexEnvironmentHome(), "platformOs": "windows"}
		case "thread/read":
			result = map[string]any{"thread": map[string]any{"id": request.Params.ThreadID, "cwd": os.Getenv("STEERLINE_CODEX_HISTORY_MOCK_WORKSPACE")}}
		case "thread/turns/list":
			stage, _ := os.ReadFile(os.Getenv("STEERLINE_CODEX_HISTORY_MOCK_STATE"))
			if strings.TrimSpace(string(stage)) == "disconnect" {
				os.Exit(8)
			}
			if strings.TrimSpace(string(stage)) == "empty" {
				result = map[string]any{"data": []any{}, "nextCursor": nil, "backwardsCursor": nil}
				break
			}
			if path := os.Getenv("STEERLINE_CODEX_HISTORY_MOCK_CALLS"); path != "" {
				f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					os.Exit(6)
				}
				_, err = fmt.Fprintln(f, request.Params.ThreadID)
				_ = f.Close()
				if err != nil {
					os.Exit(7)
				}
			}
			first := map[string]any{"id": "first-turn", "itemsView": "full", "status": "completed", "completedAt": 1, "items": []any{map[string]any{"id": "first-message", "type": "agentMessage", "phase": "commentary", "text": "progress"}}}
			if strings.TrimSpace(string(stage)) == "missing-final-text" {
				first["items"] = append(first["items"].([]any), map[string]any{"id": "late-final", "type": "agentMessage", "phase": "final_answer"})
			}
			if strings.TrimSpace(string(stage)) == "late" {
				first["items"] = append(first["items"].([]any), map[string]any{"id": "late-final", "type": "agentMessage", "phase": "final_answer", "text": "late complete"})
			}
			second := map[string]any{"id": "second-turn", "itemsView": "full", "status": "completed", "completedAt": 2, "items": []any{map[string]any{"id": "second-final", "type": "agentMessage", "phase": "final_answer", "text": "second complete"}}}
			switch request.Params.Cursor {
			case "", "first-anchor":
				result = map[string]any{"data": []any{first}, "nextCursor": "after-first", "backwardsCursor": "first-anchor"}
			case "after-first", "second-anchor":
				result = map[string]any{"data": []any{second}, "nextCursor": nil, "backwardsCursor": "second-anchor"}
			default:
				os.Exit(3)
			}
			if strings.TrimSpace(string(stage)) == "cursor-too-long" {
				result = map[string]any{"data": []any{first}, "nextCursor": nil, "backwardsCursor": strings.Repeat("x", maxCodexHistoryCursorBytes+1)}
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

func TestCodexHistoryReconcilesLateFinalInEarlierTurn(t *testing.T) {
	workspace, root := t.TempDir(), t.TempDir()
	stage := filepath.Join(root, "stage")
	require(t, os.WriteFile(stage, []byte("initial"), 0600))
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK", "1")
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK_WORKSPACE", workspace)
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK_STATE", stage)
	store, err := OpenSQLiteStore(filepath.Join(root, "history.db"))
	require(t, err)
	defer store.Close()
	b := codexHistoryBinding{Version: 1, ProjectID: "project", WorkflowID: "workflow", ExecutionID: "execution", AttemptID: "attempt", Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, TargetID: "local", Workspace: workspace, SessionID: "00000000-0000-0000-0000-000000000001", Command: Command{Executable: os.Args[0], Args: []string{"-test.run=^TestCodexHistoryServerHelper$"}}}
	s := &Service{domainStore: store}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for i := 0; i < 2; i++ {
		_, err = s.collectCodexHistoryPage(ctx, b)
		require(t, err)
	}
	cp, err := store.ConversationCheckpoint(ctx, b.ProjectID, b.ExecutionID, b.AttemptID, "codex-app-server", b.identity())
	require(t, err)
	var saved codexHistoryCursor
	require(t, json.Unmarshal([]byte(cp.Cursor), &saved))
	if len(saved.Pending) != 1 || saved.Pending[0].ID != "first-turn" {
		t.Fatalf("first turn was not kept for late final: %+v", saved)
	}
	require(t, os.WriteFile(stage, []byte("late"), 0600))
	for i := 0; i < 4; i++ {
		_, err = s.collectCodexHistoryPage(ctx, b)
		require(t, err)
	}
	cp, err = store.ConversationCheckpoint(ctx, b.ProjectID, b.ExecutionID, b.AttemptID, "codex-app-server", b.identity())
	require(t, err)
	saved = codexHistoryCursor{}
	require(t, json.Unmarshal([]byte(cp.Cursor), &saved))
	if len(saved.Pending) != 0 {
		t.Fatalf("resolved native turn remained pending: %+v", saved)
	}
	query := ConversationQueryService{Store: store, Project: b.ProjectID}
	page, err := query.ReadConversation(ctx, ConversationFilter{ExecutionID: b.ExecutionID, AttemptID: b.AttemptID}, "", 100)
	require(t, err)
	turns, lateFinal := map[string]int{}, 0
	for _, event := range page.Events {
		if event.Type == ConversationTurnStarted {
			turns[event.TurnID]++
		}
		if event.TurnID == "first-turn" && event.Phase == MessagePhaseFinalAnswer {
			lateFinal++
		}
	}
	if len(turns) != 2 || turns["first-turn"] != 1 || lateFinal != 1 {
		t.Fatal(fmt.Sprintf("late final changed turn identity or was lost: turns=%v final=%d", turns, lateFinal))
	}
	reports, err := store.ConversationCapabilityReports(ctx, executionapi.BackendLocal, executionapi.ProtocolLocalProcess, "local", 10)
	require(t, err)
	if len(reports) < 2 || len(reports) > 4 || reports[0].AgentVersion != "mock-codex/1" {
		t.Fatalf("capability evidence was not versioned or deduplicated: %+v", reports)
	}
	statuses := map[string]executionapi.CapabilityStatus{}
	for _, cap := range reports[0].Capabilities {
		statuses[cap.Name] = cap.Status
	}
	for _, name := range []string{"turn_boundaries", "message_phase", "final_content", "stable_item_identity", "incremental_read", "history_replay"} {
		if statuses[name] != executionapi.CapabilityVerified {
			t.Fatalf("native capability %s was not reported: %+v", name, statuses)
		}
	}
}

func TestCodexHistoryReconcilesFinalTextArrivingAfterTerminalStatus(t *testing.T) {
	workspace, root := t.TempDir(), t.TempDir()
	stage := filepath.Join(root, "stage")
	require(t, os.WriteFile(stage, []byte("missing-final-text"), 0600))
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK", "1")
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK_WORKSPACE", workspace)
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK_STATE", stage)
	store, err := OpenSQLiteStore(filepath.Join(root, "history.db"))
	require(t, err)
	defer store.Close()
	binding := codexHistoryBinding{Version: 1, ProjectID: "project", WorkflowID: "workflow", ExecutionID: "execution", AttemptID: "attempt", Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, TargetID: "local", Workspace: workspace, SessionID: "00000000-0000-0000-0000-000000000001", Command: Command{Executable: os.Args[0], Args: []string{"-test.run=^TestCodexHistoryServerHelper$"}}}
	service := &Service{domainStore: store}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = service.collectCodexHistoryPage(ctx, binding)
	require(t, err)
	checkpoint, err := store.ConversationCheckpoint(ctx, binding.ProjectID, binding.ExecutionID, binding.AttemptID, "codex-app-server", binding.identity())
	require(t, err)
	var saved codexHistoryCursor
	require(t, json.Unmarshal([]byte(checkpoint.Cursor), &saved))
	if len(saved.Pending) != 1 || saved.Pending[0].ID != "first-turn" {
		t.Fatalf("final without text was dropped from reconciliation: %+v", saved)
	}
	reports, err := store.ConversationCapabilityReports(ctx, executionapi.BackendLocal, executionapi.ProtocolLocalProcess, "local", 1)
	require(t, err)
	if len(reports) != 1 {
		t.Fatal("missing-text source has no capability report")
	}
	for _, capability := range reports[0].Capabilities {
		if capability.Name == "final_content" && capability.Status != executionapi.CapabilityNotTested {
			t.Fatalf("missing text falsely verified final content: %+v", capability)
		}
	}
	query := ConversationQueryService{Store: store, Project: "project"}
	filter := ConversationFilter{ExecutionID: binding.ExecutionID, AttemptID: binding.AttemptID}
	page, err := query.ReadConversation(ctx, filter, "", 100)
	require(t, err)
	for _, event := range page.Events {
		if event.Phase == MessagePhaseFinalAnswer && (event.Body != nil || event.FinalAvailability == FinalAvailable) {
			t.Fatalf("missing final text was presented as complete: %+v", event)
		}
	}
	require(t, os.WriteFile(stage, []byte("late"), 0600))
	for i := 0; i < 4; i++ {
		_, err = service.collectCodexHistoryPage(ctx, binding)
		require(t, err)
	}
	checkpoint, err = store.ConversationCheckpoint(ctx, binding.ProjectID, binding.ExecutionID, binding.AttemptID, "codex-app-server", binding.identity())
	require(t, err)
	saved = codexHistoryCursor{}
	require(t, json.Unmarshal([]byte(checkpoint.Cursor), &saved))
	if len(saved.Pending) != 0 {
		t.Fatalf("late final text left a pending turn: %+v", saved)
	}
	page, err = query.ReadConversation(ctx, filter, "", 100)
	require(t, err)
	finals := 0
	for _, event := range page.Events {
		if event.TurnID == "first-turn" && event.Phase == MessagePhaseFinalAnswer && event.FinalAvailability == FinalAvailable {
			finals++
			if event.Body == nil {
				t.Fatal("reconciled final has no full body reference")
			}
		}
	}
	if finals != 1 {
		t.Fatalf("late final text was not reconciled exactly once: %+v", page.Events)
	}
	reports, err = store.ConversationCapabilityReports(ctx, executionapi.BackendLocal, executionapi.ProtocolLocalProcess, "local", 1)
	require(t, err)
	if len(reports) != 1 {
		t.Fatal("reconciled source lost its capability report")
	}
	verified := false
	for _, capability := range reports[0].Capabilities {
		if capability.Name == "final_content" && capability.Status == executionapi.CapabilityVerified {
			verified = true
		}
	}
	if !verified {
		t.Fatal("reconciled final text did not verify source capability")
	}
}

func TestCodexHistoryEmptyPageDoesNotHideRawProgress(t *testing.T) {
	workspace, root := t.TempDir(), t.TempDir()
	stage := filepath.Join(root, "stage")
	require(t, os.WriteFile(stage, []byte("empty"), 0600))
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK", "1")
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK_WORKSPACE", workspace)
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK_STATE", stage)
	store, err := OpenSQLiteStore(filepath.Join(root, "history.db"))
	require(t, err)
	defer store.Close()
	raw := conversationTestWrite("raw-progress", 1, "complete", []byte("progress before native history"), []byte("raw-progress"))
	_, err = store.CommitConversation(context.Background(), []ConversationWrite{raw}, ConversationCheckpoint{})
	require(t, err)
	binding := codexHistoryBinding{Version: 1, ProjectID: "project", WorkflowID: "workflow", ExecutionID: "execution", AttemptID: "attempt", Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, TargetID: "local", Workspace: workspace, SessionID: "00000000-0000-0000-0000-000000000001", Command: Command{Executable: os.Args[0], Args: []string{"-test.run=^TestCodexHistoryServerHelper$"}}}
	service := &Service{domainStore: store}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err = service.collectCodexHistoryPage(ctx, binding)
	require(t, err)
	query := ConversationQueryService{Store: store, Project: "project"}
	filter := ConversationFilter{ExecutionID: binding.ExecutionID, AttemptID: binding.AttemptID}
	page, err := query.ReadConversation(ctx, filter, "", 100)
	require(t, err)
	if len(page.Events) != 1 || page.Events[0].Source != "codex-jsonl" {
		t.Fatalf("empty native history hid existing raw progress: %+v", page.Events)
	}
	require(t, os.WriteFile(stage, []byte("late"), 0600))
	_, err = service.collectCodexHistoryPage(ctx, binding)
	require(t, err)
	page, err = query.ReadConversation(ctx, filter, "", 100)
	require(t, err)
	sessions, changed, rawShown := 0, 0, 0
	for _, event := range page.Events {
		if event.Source == "codex-jsonl" {
			rawShown++
		}
		if event.Source == "codex-app-server" && event.Type == ConversationSession {
			sessions++
		}
		if event.Source == "codex-app-server" && event.Type == ConversationGap && event.Status == "source_changed" {
			changed++
		}
	}
	if sessions != 1 || changed != 1 || rawShown != 0 {
		t.Fatalf("first native turn did not switch projections exactly once: %+v", page.Events)
	}
	rawPage, err := query.ReadConversation(ctx, ConversationFilter{ExecutionID: binding.ExecutionID, AttemptID: binding.AttemptID, Source: "codex-jsonl"}, "", 100)
	require(t, err)
	if len(rawPage.Events) != 1 {
		t.Fatal("explicit raw source history disappeared after native selection")
	}
}

func TestCodexHistoryDisconnectKeepsVisibleGapAndCheckpoint(t *testing.T) {
	workspace, root := t.TempDir(), t.TempDir()
	stage := filepath.Join(root, "stage")
	require(t, os.WriteFile(stage, []byte("disconnect"), 0600))
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK", "1")
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK_WORKSPACE", workspace)
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK_STATE", stage)
	store, err := OpenSQLiteStore(filepath.Join(root, "history.db"))
	require(t, err)
	defer store.Close()
	binding := codexHistoryBinding{Version: 1, ProjectID: "project", WorkflowID: "workflow", ExecutionID: "execution", AttemptID: "attempt", Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, TargetID: "local", Workspace: workspace, SessionID: "00000000-0000-0000-0000-000000000001", Command: Command{Executable: os.Args[0], Args: []string{"-test.run=^TestCodexHistoryServerHelper$"}}}
	service := &Service{domainStore: store}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	requests := make(chan []codexHistoryBinding, 4)
	errorsOut := make(chan error, 4)
	go service.runCodexHistoryCollector(ctx, requests, errorsOut)
	requests <- []codexHistoryBinding{binding}
	select {
	case err := <-errorsOut:
		if err == nil {
			t.Fatal("disconnected history source reported no error")
		}
	case <-ctx.Done():
		t.Fatal("disconnected history source did not report failure")
	}
	query := ConversationQueryService{Store: store, Project: "project"}
	filter := ConversationFilter{ExecutionID: binding.ExecutionID, AttemptID: binding.AttemptID, Source: "codex-app-server"}
	page, err := query.ReadConversation(ctx, filter, "", 100)
	require(t, err)
	if len(page.Events) != 1 || page.Events[0].Type != ConversationGap || page.Events[0].Status != "source_unavailable" {
		t.Fatalf("disconnect did not leave a visible source gap: %+v", page.Events)
	}
	if _, err = store.ConversationCheckpoint(ctx, binding.ProjectID, binding.ExecutionID, binding.AttemptID, "codex-app-server", binding.identity()); err == nil {
		t.Fatal("disconnect advanced the conversation checkpoint")
	}
	require(t, os.WriteFile(stage, []byte("late"), 0600))
	for i := 0; i < 3; i++ {
		requests <- []codexHistoryBinding{binding}
	}
	deadline := time.Now().Add(5 * time.Second)
	finals := 0
	for time.Now().Before(deadline) {
		page, err = query.ReadConversation(ctx, filter, "", 100)
		require(t, err)
		finals = 0
		for _, event := range page.Events {
			if event.Type == ConversationMessageComplete && event.Phase == MessagePhaseFinalAnswer {
				finals++
			}
		}
		if finals == 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if finals != 2 || len(page.Events) < 7 || page.Events[0].Type != ConversationGap {
		t.Fatalf("recovered history did not preserve the old gap: %+v", page.Events)
	}
	checkpoint, err := store.ConversationCheckpoint(ctx, binding.ProjectID, binding.ExecutionID, binding.AttemptID, "codex-app-server", binding.identity())
	require(t, err)
	if checkpoint.Cursor == "" {
		t.Fatal("recovered history did not advance its checkpoint")
	}
}

func TestCodexHistoryRejectsOversizedOpaqueCursor(t *testing.T) {
	workspace, root := t.TempDir(), t.TempDir()
	stage := filepath.Join(root, "stage")
	require(t, os.WriteFile(stage, []byte("cursor-too-long"), 0600))
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK", "1")
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK_WORKSPACE", workspace)
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK_STATE", stage)
	store, err := OpenSQLiteStore(filepath.Join(root, "history.db"))
	require(t, err)
	defer store.Close()
	b := codexHistoryBinding{Version: 1, ProjectID: "project", WorkflowID: "workflow", ExecutionID: "execution", AttemptID: "attempt", Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, TargetID: "local", Workspace: workspace, SessionID: "00000000-0000-0000-0000-000000000001", Command: Command{Executable: os.Args[0], Args: []string{"-test.run=^TestCodexHistoryServerHelper$"}}}
	if _, err = (&Service{domainStore: store}).collectCodexHistoryPage(context.Background(), b); err == nil || !strings.Contains(err.Error(), "bounded source limit") {
		t.Fatalf("oversized cursor was accepted: %v", err)
	}
	checkpoints, err := store.ConversationSourceCheckpoints(context.Background(), b.ProjectID, b.ExecutionID, b.AttemptID, "codex-app-server")
	require(t, err)
	if len(checkpoints) != 0 {
		t.Fatal("oversized native cursor advanced a checkpoint")
	}
}

func TestCodexHistoryCollectorPrioritizesActiveAndBoundsHistoricalWork(t *testing.T) {
	workspace, root := t.TempDir(), t.TempDir()
	stage, calls := filepath.Join(root, "stage"), filepath.Join(root, "calls")
	require(t, os.WriteFile(stage, []byte("initial"), 0600))
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK", "1")
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK_WORKSPACE", workspace)
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK_STATE", stage)
	t.Setenv("STEERLINE_CODEX_HISTORY_MOCK_CALLS", calls)
	store, err := OpenSQLiteStore(filepath.Join(root, "history.db"))
	require(t, err)
	defer store.Close()
	s := &Service{domainStore: store}
	sources := make([]codexHistoryBinding, 0, 6)
	for i := 0; i < 5; i++ {
		sources = append(sources, codexHistoryBinding{Version: 1, ProjectID: "project", WorkflowID: "workflow", ExecutionID: fmt.Sprintf("execution-%d", i), AttemptID: fmt.Sprintf("attempt-%d", i), Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, TargetID: "local", Workspace: workspace, SessionID: fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1), Command: Command{Executable: os.Args[0], Args: []string{"-test.run=^TestCodexHistoryServerHelper$"}}, Ended: true})
	}
	active := sources[0]
	active.ExecutionID, active.AttemptID, active.SessionID, active.Ended = "execution-active", "attempt-active", "00000000-0000-0000-0000-000000000099", false
	sources = append(sources, active)
	ctx, cancel := context.WithCancel(context.Background())
	requests := make(chan []codexHistoryBinding, 1)
	errorsOut := make(chan error, 10)
	done := make(chan struct{})
	go func() { defer close(done); s.runCodexHistoryCollector(ctx, requests, errorsOut) }()
	requests <- sources
	deadline := time.Now().Add(10 * time.Second)
	var lines []string
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(calls); err == nil {
			lines = strings.Fields(string(raw))
			if len(lines) >= 2 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	select {
	case err := <-errorsOut:
		t.Fatal(err)
	default:
	}
	if len(lines) != 2 || lines[0] != active.SessionID || lines[1] != sources[4].SessionID {
		t.Fatalf("collector blocked the active source behind historical work: %v", lines)
	}
}

func TestCodexHistoryNativeFixtures(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "validation", "pr-04b-public-history-native-2026-09-25.json"))
	require(t, err)
	var fixture struct {
		Samples []struct {
			Backend executionapi.BackendKind `json:"backend"`
			Thread  codexHistoryThread       `json:"thread"`
		} `json:"samples"`
	}
	require(t, json.Unmarshal(raw, &fixture))
	if len(fixture.Samples) != 2 {
		t.Fatalf("need both native source fixtures, got %d", len(fixture.Samples))
	}
	for _, sample := range fixture.Samples {
		t.Run(string(sample.Backend), func(t *testing.T) {
			store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "history.db"))
			require(t, err)
			defer store.Close()
			base := ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Backend: sample.Backend, Protocol: executionapi.ProtocolLocalProcess, Agent: "codex", Source: "codex-app-server", SourceSegment: "native", SourceIdentity: "native", SessionID: sample.Thread.ID}
			if sample.Backend == executionapi.BackendOrca {
				base.Protocol = executionapi.ProtocolOrcaTerminalV2
			}
			var all []ConversationWrite
			for _, turn := range sample.Thread.Turns {
				writes, mapErr := mapCodexHistoryTurn(base, turn, uint64(len(all)+1))
				require(t, mapErr)
				all = append(all, writes...)
			}
			first, err := store.CommitConversation(context.Background(), all, ConversationCheckpoint{})
			require(t, err)
			second, err := store.CommitConversation(context.Background(), all, ConversationCheckpoint{})
			require(t, err)
			starts, ends, commentary, finals, tools := 0, 0, 0, 0, 0
			for i, e := range first {
				if e.EventID != second[i].EventID {
					t.Fatal("replay duplicated a native event")
				}
				switch e.Type {
				case ConversationTurnStarted:
					starts++
				case ConversationTurnCompleted:
					ends++
					if e.FinalAvailability != FinalAvailable {
						t.Fatal("native final evidence lost")
					}
				case ConversationToolCompleted:
					tools++
				}
				if e.Type == ConversationMessageComplete {
					if e.Phase == MessagePhaseCommentary {
						commentary++
					}
					if e.Phase == MessagePhaseFinalAnswer {
						finals++
					}
					if e.Body == nil || e.ItemQuality != IdentityNative || e.TurnQuality != IdentityNative {
						t.Fatalf("native message identity/body lost: %+v", e)
					}
				}
			}
			if starts != 2 || ends != 2 || commentary < 1 || finals != 2 {
				t.Fatalf("native facts lost: starts=%d ends=%d commentary=%d finals=%d", starts, ends, commentary, finals)
			}
			if sample.Backend == executionapi.BackendOrca && tools < 2 {
				t.Fatalf("Orca native tool events lost: %d", tools)
			}
		})
	}
}

func TestCodexHistoryRealLocalToolInterleaveFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "validation", "pr-04b-local-tool-interleave-2026-09-25.json"))
	require(t, err)
	var fixture struct {
		Thread codexHistoryThread `json:"thread"`
	}
	require(t, json.Unmarshal(raw, &fixture))
	if len(fixture.Thread.Turns) != 1 {
		t.Fatal("real local tool fixture must contain exactly one turn")
	}
	base := ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, Agent: "codex", SourceSegment: "native", SourceIdentity: "native", SessionID: fixture.Thread.ID}
	writes, err := mapCodexHistoryTurn(base, fixture.Thread.Turns[0], 1)
	require(t, err)
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "history.db"))
	require(t, err)
	defer store.Close()
	first, err := store.CommitConversation(context.Background(), writes, ConversationCheckpoint{})
	require(t, err)
	second, err := store.CommitConversation(context.Background(), writes, ConversationCheckpoint{})
	require(t, err)
	if len(first) != 6 || len(second) != len(first) {
		t.Fatalf("real interleaved turn was not preserved: %+v", first)
	}
	for i := range first {
		if first[i].EventID != second[i].EventID || first[i].TurnID != fixture.Thread.Turns[0].ID {
			t.Fatalf("interleaved replay duplicated or reassigned event %d: %+v", i, first[i])
		}
	}
	if first[0].Type != ConversationTurnStarted || first[2].Type != ConversationMessageComplete || first[2].Phase != MessagePhaseCommentary || first[3].Type != ConversationToolCompleted || first[3].Status != "completed" || first[4].Type != ConversationMessageComplete || first[4].Phase != MessagePhaseFinalAnswer || first[5].Type != ConversationTurnCompleted || first[5].FinalAvailability != FinalAvailable {
		t.Fatalf("real commentary/tool/final order or turn completion was lost: %+v", first)
	}
	query := ConversationQueryService{Store: store, Project: "project"}
	for _, tc := range []struct {
		index int
		want  string
	}{{2, "I’ll read `probe.txt` from the workspace and return its marker unchanged."}, {3, "PR04B_NATIVE_PROBE_ALPHA\r\n"}, {4, "PR04B_NATIVE_PROBE_ALPHA"}} {
		if first[tc.index].Body == nil || first[tc.index].Body.Completeness != BodyComplete {
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
func TestCodexHistoryRealSourceReplacement(t *testing.T) {
	path := os.Getenv("STEERLINE_CODEX_HISTORY_REPLACEMENT_BINDINGS")
	if path == "" {
		t.Skip("set exact isolated source-replacement bindings to run acceptance")
	}
	raw, err := os.ReadFile(path)
	require(t, err)
	var bindings []codexHistoryBinding
	require(t, json.Unmarshal(raw, &bindings))
	if len(bindings) != 2 || bindings[0].Backend != executionapi.BackendLocal || bindings[1].Backend != executionapi.BackendLocal || bindings[0].ProjectID != bindings[1].ProjectID || bindings[0].ExecutionID != bindings[1].ExecutionID || bindings[0].AttemptID != bindings[1].AttemptID || canonical(bindings[0].Workspace) != canonical(bindings[1].Workspace) || bindings[0].SessionID == bindings[1].SessionID {
		t.Fatal("replacement acceptance needs two distinct real sessions bound to one local attempt and workspace")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "replacement.db"))
	require(t, err)
	defer store.Close()
	service := &Service{domainStore: store}
	for _, binding := range bindings {
		settled := false
		for i := 0; i < 8 && !settled; i++ {
			settled, err = service.collectCodexHistoryPage(ctx, binding)
			require(t, err)
		}
		if !settled {
			t.Fatalf("real source %s did not reach its final page", binding.SessionID)
		}
	}
	query := ConversationQueryService{Store: store, Project: bindings[0].ProjectID}
	filter := ConversationFilter{ExecutionID: bindings[0].ExecutionID, AttemptID: bindings[0].AttemptID, Source: "codex-app-server"}
	page, err := query.ReadConversation(ctx, filter, "", 100)
	require(t, err)
	sessions, gaps, finals := map[string]int{}, map[string]int{}, map[string]int{}
	for _, event := range page.Events {
		switch {
		case event.Type == ConversationSession:
			sessions[event.SessionID]++
		case event.Type == ConversationGap && event.Status == "source_changed":
			gaps[event.SessionID]++
		case event.Type == ConversationMessageComplete && event.Phase == MessagePhaseFinalAnswer:
			finals[event.SessionID]++
			if event.Body == nil {
				t.Fatal("real replacement lost a final body")
			}
			body, readErr := query.ReadMessageBody(ctx, *event.Body, 0, 256<<10)
			require(t, readErr)
			if !body.Complete || body.SHA256 != event.Body.SHA256 {
				t.Fatal("real replacement changed the final body")
			}
		}
	}
	for _, binding := range bindings {
		if sessions[binding.SessionID] != 1 || gaps[binding.SessionID] != 1 || finals[binding.SessionID] < 1 {
			t.Fatalf("real source %s was hidden or crossed: sessions=%v gaps=%v finals=%v", binding.SessionID, sessions, gaps, finals)
		}
		cp, cpErr := store.ConversationCheckpoint(ctx, binding.ProjectID, binding.ExecutionID, binding.AttemptID, "codex-app-server", binding.identity())
		require(t, cpErr)
		if cp.Cursor == "" {
			t.Fatalf("real source %s has no own checkpoint", binding.SessionID)
		}
	}
	t.Logf("real replacement: events=%d, old_finals=%d, new_finals=%d, source_changed=2, independent_checkpoints=2", len(page.Events), finals[bindings[0].SessionID], finals[bindings[1].SessionID])
	before := len(page.Events)
	for _, binding := range bindings {
		_, err = service.collectCodexHistoryPage(ctx, binding)
		require(t, err)
	}
	page, err = query.ReadConversation(ctx, filter, "", 100)
	require(t, err)
	if len(page.Events) != before {
		t.Fatalf("real source replacement replay duplicated events: %d -> %d", before, len(page.Events))
	}
}

func TestCodexHistoryRealCancelledTurnFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "validation", "pr-04b-native-cancelled-2026-09-25.json"))
	require(t, err)
	var fixture struct {
		Thread codexHistoryThread `json:"thread"`
	}
	require(t, json.Unmarshal(raw, &fixture))
	if len(fixture.Thread.Turns) != 1 || fixture.Thread.Turns[0].CompletedAt == nil {
		t.Fatal("real cancelled turn lacks a native completion timestamp")
	}
	base := ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, Agent: "codex", SourceSegment: "native", SourceIdentity: "native", SessionID: fixture.Thread.ID}
	writes, err := mapCodexHistoryTurn(base, fixture.Thread.Turns[0], 1)
	require(t, err)
	if len(writes) != 2 || writes[0].Event.Type != ConversationTurnStarted || writes[1].Event.Type != ConversationTurnCancelled || writes[1].Event.Status != "interrupted" || writes[1].Event.FinalAvailability != FinalAbsent || writes[1].Event.TurnID != writes[0].Event.TurnID {
		t.Fatalf("real terminal cancellation or absent final was misrepresented: %+v", writes)
	}
}

func TestCodexHistoryRealFailedTurnFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "validation", "pr-04b-native-failed-2026-09-25.json"))
	require(t, err)
	var fixture struct {
		Thread codexHistoryThread `json:"thread"`
	}
	require(t, json.Unmarshal(raw, &fixture))
	if len(fixture.Thread.Turns) != 1 || fixture.Thread.Turns[0].CompletedAt == nil || fixture.Thread.Turns[0].Status != "failed" {
		t.Fatal("real failed turn lacks native terminal evidence")
	}
	base := ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, Agent: "codex", SourceSegment: "native", SourceIdentity: "native", SessionID: fixture.Thread.ID}
	writes, err := mapCodexHistoryTurn(base, fixture.Thread.Turns[0], 1)
	require(t, err)
	if len(writes) != 3 || writes[0].Event.Type != ConversationTurnStarted || writes[1].Event.Type != ConversationObservation || writes[2].Event.Type != ConversationTurnFailed || writes[2].Event.Status != "failed" || writes[2].Event.FinalAvailability != FinalAbsent {
		t.Fatalf("real native failure or missing final was misrepresented: %+v", writes)
	}
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "failure.db"))
	require(t, err)
	defer store.Close()
	events, err := store.CommitConversation(context.Background(), writes, ConversationCheckpoint{})
	require(t, err)
	if events[2].Raw == nil {
		t.Fatal("real failure lost its raw error evidence")
	}
	page, err := (ConversationQueryService{Store: store, Project: "project"}).ReadRaw(context.Background(), *events[2].Raw, 0, 4096)
	require(t, err)
	if !page.Complete || !strings.Contains(string(page.Data), "401 Unauthorized") {
		t.Fatalf("real failure error evidence changed: %+v", page)
	}
}

// TestCodexHistoryRealFailedAcceptance reads an exact isolated unauthenticated
// session through the production public-history collector.
func TestCodexHistoryRealFailedAcceptance(t *testing.T) {
	path := os.Getenv("STEERLINE_CODEX_HISTORY_FAILED_BINDING")
	if path == "" {
		t.Skip("set an exact isolated failed-turn binding to run acceptance")
	}
	raw, err := os.ReadFile(path)
	require(t, err)
	var binding codexHistoryBinding
	require(t, json.Unmarshal(raw, &binding))
	if binding.Backend != executionapi.BackendLocal || binding.Home == "" {
		t.Fatal("failed-turn acceptance requires an isolated local Codex home")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "failed.db"))
	require(t, err)
	defer store.Close()
	service := &Service{domainStore: store}
	_, err = service.collectCodexHistoryPage(ctx, binding)
	require(t, err)
	query := ConversationQueryService{Store: store, Project: binding.ProjectID}
	filter := ConversationFilter{ExecutionID: binding.ExecutionID, AttemptID: binding.AttemptID, Source: "codex-app-server"}
	page, err := query.ReadConversation(ctx, filter, "", 100)
	require(t, err)
	failures := 0
	for _, event := range page.Events {
		if event.Type == ConversationTurnFailed {
			failures++
			if event.Status != "failed" || event.FinalAvailability != FinalAbsent || event.Raw == nil {
				t.Fatalf("real failed turn lost native status or error reference: %+v", event)
			}
		}
		if event.Type == ConversationTurnCancelled || event.Type == ConversationMessageComplete {
			t.Fatalf("real failed turn was confused with cancellation or final: %+v", event)
		}
	}
	if failures != 1 {
		t.Fatalf("real failed turn was not collected once: %+v", page.Events)
	}
	_, err = service.collectCodexHistoryPage(ctx, binding)
	require(t, err)
	replayed, err := query.ReadConversation(ctx, filter, "", 100)
	require(t, err)
	if len(replayed.Events) != len(page.Events) {
		t.Fatalf("real failed-turn replay duplicated events: %d -> %d", len(page.Events), len(replayed.Events))
	}
}

func TestCodexHistoryTurnCompletionAndLateFinal(t *testing.T) {
	base := ConversationEvent{ProjectID: "p", ExecutionID: "e", AttemptID: "a", SourceSegment: "s", SessionID: "session"}
	completedAt := int64(1)
	turn := codexHistoryTurn{ID: "turn", Status: "completed", CompletedAt: &completedAt, Items: []json.RawMessage{json.RawMessage(`{"id":"message","type":"agentMessage","text":"unknown"}`)}}
	first, err := mapCodexHistoryTurn(base, turn, 1)
	require(t, err)
	if first[len(first)-1].Event.FinalAvailability != FinalUnknown {
		t.Fatal("missing phase was called an absent final")
	}
	turn.Items = append(turn.Items, json.RawMessage(`{"id":"final","type":"agentMessage","phase":"final_answer","text":"late final"}`))
	late, err := mapCodexHistoryTurn(base, turn, 10)
	require(t, err)
	if late[len(late)-1].Event.FinalAvailability != FinalAvailable || late[len(late)-1].Event.SourceEventID == first[len(first)-1].Event.SourceEventID {
		t.Fatal("late native final cannot reconcile its original turn")
	}
	turn.Status = "interrupted"
	cancelled, err := mapCodexHistoryTurn(base, turn, 20)
	require(t, err)
	if cancelled[len(cancelled)-1].Event.Type != ConversationTurnCancelled {
		t.Fatal("interrupted native turn was not cancelled")
	}
	turn.CompletedAt = nil
	active, err := mapCodexHistoryTurn(base, turn, 30)
	require(t, err)
	for _, write := range active {
		if write.Event.Type == ConversationTurnCancelled || write.Event.Type == ConversationTurnCompleted {
			t.Fatal("offline history's synthetic interrupted status became a terminal event")
		}
	}
}

func TestCodexHistoryMissingTextDoesNotClaimFinalBody(t *testing.T) {
	base := ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", SourceSegment: "native", SessionID: "session"}
	completedAt := int64(1)
	missing := codexHistoryTurn{ID: "missing-turn", Status: "completed", CompletedAt: &completedAt, Items: []json.RawMessage{json.RawMessage(`{"id":"missing-final","type":"agentMessage","phase":"final_answer"}`)}}
	writes, err := mapCodexHistoryTurn(base, missing, 1)
	require(t, err)
	if len(writes) != 3 || writes[1].BodyMode != "" || writes[1].Event.FinalAvailability != FinalUnknown || writes[2].Event.FinalAvailability != FinalUnknown {
		t.Fatalf("native final without text was claimed as complete: %+v", writes)
	}
	empty := codexHistoryTurn{ID: "empty-turn", Status: "completed", CompletedAt: &completedAt, Items: []json.RawMessage{json.RawMessage(`{"id":"empty-final","type":"agentMessage","phase":"final_answer","text":""}`)}}
	writes, err = mapCodexHistoryTurn(base, empty, 10)
	require(t, err)
	if len(writes) != 3 || writes[1].BodyMode != "complete" || writes[1].Event.FinalAvailability != FinalAvailable || writes[2].Event.FinalAvailability != FinalAvailable {
		t.Fatalf("explicit empty native final was not complete: %+v", writes)
	}
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "history.db"))
	require(t, err)
	defer store.Close()
	events, err := store.CommitConversation(context.Background(), writes, ConversationCheckpoint{})
	require(t, err)
	if events[1].Body == nil || events[1].Body.Bytes != 0 || events[1].Body.Completeness != BodyComplete {
		t.Fatalf("explicit empty native final lacks a readable body reference: %+v", events[1])
	}
}

func TestCodexHistoryCanonicalSourceKeepsRawReadable(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "history.db"))
	require(t, err)
	defer store.Close()
	raw := conversationTestWrite("bytes:0:4", 1, "complete", []byte("raw reply"), []byte("raw"))
	_, err = store.CommitConversation(ctx, []ConversationWrite{raw}, ConversationCheckpoint{})
	require(t, err)
	native := raw.Event
	native.Source, native.SourceSegment, native.SourceEventID = "codex-app-server", "native", "session"
	native.Type, native.ItemKey = ConversationSession, ""
	_, err = store.CommitConversation(ctx, []ConversationWrite{{Event: native}}, ConversationCheckpoint{})
	require(t, err)
	query := ConversationQueryService{Store: store, Project: "project"}
	page, err := query.ReadConversation(ctx, ConversationFilter{}, "", 100)
	require(t, err)
	if len(page.Events) != 1 || page.Events[0].Source != "codex-app-server" {
		t.Fatal("raw and native message projections were both displayed")
	}
	rawPage, err := query.ReadConversation(ctx, ConversationFilter{Source: "codex-jsonl"}, "", 100)
	require(t, err)
	if len(rawPage.Events) != 1 || rawPage.Events[0].Raw == nil {
		t.Fatal("raw source evidence became inaccessible")
	}
	other := raw
	other.Event.AttemptID = "other-attempt"
	other.Event.ItemKey = conversationItemKey(other.Event, other.Event.ItemID)
	_, err = store.CommitConversation(ctx, []ConversationWrite{other}, ConversationCheckpoint{})
	require(t, err)
	page, err = query.ReadConversation(ctx, ConversationFilter{}, "", 100)
	require(t, err)
	if len(page.Events) != 2 {
		t.Fatal("native source selection crossed attempt boundaries")
	}
	gap := raw.Event
	gap.Type, gap.SourceEventID, gap.ItemKey = ConversationGap, "source-changed", ""
	wait := gap
	wait.Source, wait.Type, wait.SourceEventID = "orca-terminal-v2", ConversationInputWait, "host-wait"
	_, err = store.CommitConversation(ctx, []ConversationWrite{{Event: gap}, {Event: wait}}, ConversationCheckpoint{})
	require(t, err)
	page, err = query.ReadConversation(ctx, ConversationFilter{}, "", 100)
	require(t, err)
	if len(page.Events) != 4 || page.Events[2].Type != ConversationGap || page.Events[3].Type != ConversationInputWait {
		t.Fatal("native source selection hid a gap or host wait")
	}
}

func TestCodexHistoryBindingCannotChangeAttempt(t *testing.T) {
	b := codexHistoryBinding{Version: 1, ProjectID: "p", WorkflowID: "w", ExecutionID: "e", AttemptID: "a", Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, TargetID: "local", Workspace: t.TempDir(), SessionID: "00000000-0000-0000-0000-000000000001", Command: Command{Executable: "codex"}}
	path := filepath.Join(t.TempDir(), codexHistoryBindingFile)
	require(t, writeCodexHistoryBinding(path, b))
	b.AttemptID = "replacement"
	if err := writeCodexHistoryBinding(path, b); err == nil {
		t.Fatal("a native session binding crossed attempts")
	}
}

func TestCodexHistorySourceDiscoveryRejectsWrongWorkspace(t *testing.T) {
	root, workspace := t.TempDir(), t.TempDir()
	workflowID, executionID, attemptID := newID(), newID(), newID()
	logPath := filepath.Join(root, "runs", workflowID, "events.jsonl")
	require(t, os.MkdirAll(filepath.Dir(logPath), 0700))
	require(t, os.WriteFile(logPath, []byte("{}\n"), 0600))
	b := codexHistoryBinding{Version: 1, ProjectID: "p", WorkflowID: workflowID, ExecutionID: executionID, AttemptID: attemptID, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalProcess, TargetID: "local", Workspace: workspace, SessionID: "00000000-0000-0000-0000-000000000001", Command: Command{Executable: "codex"}}
	require(t, writeCodexHistoryBinding(localCodexHistoryBindingPath(filepath.Dir(logPath), b.SessionID), b))
	s := &Service{Config: Config{RunDir: root, identity: "p"}, State: Runtime{}, domain: DomainSnapshot{Attempts: []StepAttemptRecord{{ProjectIdentity: "p", AttemptID: attemptID, ExecutionID: executionID, StepID: "implement", EventPath: logPath}}}, runs: []*Run{{TaskID: "1", DomainTaskID: "1", RunID: workflowID, WorkflowVersion: "minimal-v1", ExecutionID: executionID, AttemptID: attemptID, Agent: "codex", Backend: string(executionapi.BackendLocal), Protocol: string(executionapi.ProtocolLocalProcess), Target: executionapi.ExecutionTarget{ID: "local"}, WorktreePath: &workspace, LogPath: &logPath}}}
	if len(s.codexHistorySources()) != 1 {
		t.Fatal("the exact source binding was not discovered")
	}
	wrong := t.TempDir()
	s.State.Runs[0].WorktreePath = &wrong
	if len(s.codexHistorySources()) != 0 || s.State.Service.Error == nil {
		t.Fatal("a mismatched workspace was read or silently ignored")
	}
	s.State.Runs[0].WorktreePath = &workspace
	b.SessionID = "00000000-0000-0000-0000-000000000002"
	require(t, writeCodexHistoryBinding(localCodexHistoryBindingPath(filepath.Dir(logPath), b.SessionID), b))
	sources := s.codexHistorySources()
	if len(sources) != 2 || sources[0].SessionID == sources[1].SessionID {
		t.Fatalf("replacement source lost the earlier bound session: %+v", sources)
	}
}

// Uses only read-only public history operations on the exact caller-supplied
// probe sessions. No live user session is discovered or resumed by this test.
func TestCodexHistoryNativeAcceptance(t *testing.T) {
	path := os.Getenv("STEERLINE_CODEX_HISTORY_NATIVE_BINDINGS")
	if path == "" {
		t.Skip("set exact isolated native probe bindings to run acceptance")
	}
	raw, err := os.ReadFile(path)
	require(t, err)
	var bindings []codexHistoryBinding
	require(t, json.Unmarshal(raw, &bindings))
	if len(bindings) != 2 {
		t.Fatal("acceptance requires local and Orca probe sessions")
	}
	var evidence []map[string]any
	for _, binding := range bindings {
		t.Run(string(binding.Backend), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			dbPath := filepath.Join(t.TempDir(), "history.db")
			store, err := OpenSQLiteStore(dbPath)
			require(t, err)
			service := &Service{domainStore: store}
			if binding.Backend == executionapi.BackendOrca {
				orcaExe, lookErr := exec.LookPath("orca")
				require(t, lookErr)
				service.Config.Orca.Executable = orcaExe
			}
			settled := false
			for n := 0; n < 8 && !settled; n++ {
				settled, err = service.collectCodexHistoryPage(ctx, binding)
				require(t, err)
			}
			if !settled {
				t.Fatal("native turn paging did not reach the final turn")
			}
			query := ConversationQueryService{Store: store, Project: binding.ProjectID}
			filter := ConversationFilter{ExecutionID: binding.ExecutionID, AttemptID: binding.AttemptID}
			page, err := query.ReadConversation(ctx, filter, "", 1000)
			require(t, err)
			starts, ends, commentary, finals, tools := 0, 0, 0, 0, 0
			digests := []string{}
			for _, e := range page.Events {
				switch e.Type {
				case ConversationTurnStarted:
					starts++
				case ConversationTurnCompleted:
					ends++
				case ConversationToolCompleted:
					tools++
				}
				if e.Type == ConversationMessageComplete {
					if e.Phase == MessagePhaseCommentary {
						commentary++
					}
					if e.Phase == MessagePhaseFinalAnswer {
						finals++
					}
					if e.Body == nil {
						t.Fatal("native full message body missing")
					}
					body, bodyErr := query.ReadMessageBody(ctx, *e.Body, 0, 256<<10)
					require(t, bodyErr)
					if !body.Complete || body.SHA256 != e.Body.SHA256 {
						t.Fatal("native message body/hash mismatch")
					}
					digests = append(digests, body.SHA256)
				}
			}
			if starts != 2 || ends != 2 || commentary < 1 || finals != 2 {
				t.Fatalf("native exact facts missing: starts=%d ends=%d commentary=%d finals=%d", starts, ends, commentary, finals)
			}
			if binding.Backend == executionapi.BackendOrca && tools < 2 {
				t.Fatal("Orca native tool history missing")
			}
			reports, reportErr := store.ConversationCapabilityReports(ctx, binding.Backend, binding.Protocol, binding.TargetID, 1)
			require(t, reportErr)
			if len(reports) != 1 || reports[0].AgentVersion == "" || reports[0].PlatformVersion == "" || reports[0].PlatformVersion == "windows" || reports[0].ExecutionID != binding.ExecutionID || reports[0].AttemptID != binding.AttemptID || reports[0].SourceIdentity != binding.identity() {
				t.Fatal("native capability report lacks actual agent version")
			}
			statuses := map[string]executionapi.CapabilityStatus{}
			for _, cap := range reports[0].Capabilities {
				statuses[cap.Name] = cap.Status
			}
			for _, name := range []string{"turn_boundaries", "message_phase", "final_content", "stable_item_identity", "incremental_read", "history_replay"} {
				if statuses[name] != executionapi.CapabilityVerified {
					t.Fatalf("native report did not verify %s: %+v", name, statuses)
				}
			}
			if binding.Backend == executionapi.BackendOrca && statuses["input_wait"] != executionapi.CapabilityNotTested {
				t.Fatal("unobserved Orca wait was overclaimed")
			}
			if binding.Backend == executionapi.BackendLocal && statuses["input_wait"] != executionapi.CapabilityUnsupported {
				t.Fatal("unsupported local reply was overclaimed")
			}
			checkpoint, err := store.ConversationCheckpoint(ctx, binding.ProjectID, binding.ExecutionID, binding.AttemptID, "codex-app-server", binding.identity())
			require(t, err)
			require(t, store.Close())
			store, err = OpenSQLiteStore(dbPath)
			require(t, err)
			defer store.Close()
			service.domainStore = store
			query.Store = store
			for i := 0; i < 2; i++ {
				_, err = service.collectCodexHistoryPage(ctx, binding)
				require(t, err)
			}
			replayedCheckpoint, checkpointErr := store.ConversationCheckpoint(ctx, binding.ProjectID, binding.ExecutionID, binding.AttemptID, "codex-app-server", binding.identity())
			require(t, checkpointErr)
			if replayedCheckpoint.LocalSeq != checkpoint.LocalSeq || replayedCheckpoint.Cursor != checkpoint.Cursor {
				t.Fatal("unchanged native history advanced the persisted source position")
			}
			replayed, err := query.ReadConversation(ctx, filter, "", 1000)
			require(t, err)
			if len(replayed.Events) != len(page.Events) {
				t.Fatalf("native reopen replay duplicated events: %d -> %d", len(page.Events), len(replayed.Events))
			}
			wrong := binding
			wrong.Workspace = t.TempDir()
			if _, err = service.collectCodexHistoryPage(ctx, wrong); err == nil {
				t.Fatal("history read crossed workspace identity")
			}
			evidence = append(evidence, map[string]any{"backend": binding.Backend, "protocol": binding.Protocol, "session_sha256": digestValue(binding.SessionID), "events": len(page.Events), "native_turn_starts": starts, "native_turn_completions": ends, "commentary_messages": commentary, "final_messages": finals, "tool_completions": tools, "body_sha256": digests, "cursor_persisted": checkpoint.Cursor != "", "capabilities_persisted_by_target_and_version": true, "reopen_replay_without_duplicates": true, "reopen_replay_without_cursor_advance": true, "wrong_workspace_rejected": true})
		})
	}
	if out := os.Getenv("STEERLINE_CODEX_HISTORY_NATIVE_EVIDENCE_OUT"); out != "" && !t.Failed() {
		data, err := json.MarshalIndent(map[string]any{"version": 1, "captured_at": time.Now().UTC(), "samples": evidence}, "", "  ")
		require(t, err)
		require(t, AtomicWrite(out, append(data, '\n')))
	}
}
