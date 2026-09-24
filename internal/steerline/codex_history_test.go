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
			stage, _ := os.ReadFile(os.Getenv("STEERLINE_CODEX_HISTORY_MOCK_STATE"))
			first := map[string]any{"id": "first-turn", "itemsView": "full", "status": "completed", "completedAt": 1, "items": []any{map[string]any{"id": "first-message", "type": "agentMessage", "phase": "commentary", "text": "progress"}}}
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
	b := codexHistoryBinding{Version: 1, ProjectID: "project", WorkflowID: "workflow", ExecutionID: "execution", AttemptID: "attempt", Backend: BackendLocal, Protocol: ProtocolLocalProcess, TargetID: "local", Workspace: workspace, SessionID: "00000000-0000-0000-0000-000000000001", Command: Command{Executable: os.Args[0], Args: []string{"-test.run=^TestCodexHistoryServerHelper$"}}}
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
	reports, err := store.ConversationCapabilityReports(ctx, BackendLocal, ProtocolLocalProcess, "local", 10)
	require(t, err)
	if len(reports) < 2 || len(reports) > 4 || reports[0].AgentVersion != "mock-codex/1" {
		t.Fatalf("capability evidence was not versioned or deduplicated: %+v", reports)
	}
	statuses := map[string]CapabilityStatus{}
	for _, cap := range reports[0].Capabilities {
		statuses[cap.Name] = cap.Status
	}
	for _, name := range []string{"turn_boundaries", "message_phase", "final_content", "stable_item_identity", "incremental_read", "history_replay"} {
		if statuses[name] != CapabilityVerified {
			t.Fatalf("native capability %s was not reported: %+v", name, statuses)
		}
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
	b := codexHistoryBinding{Version: 1, ProjectID: "project", WorkflowID: "workflow", ExecutionID: "execution", AttemptID: "attempt", Backend: BackendLocal, Protocol: ProtocolLocalProcess, TargetID: "local", Workspace: workspace, SessionID: "00000000-0000-0000-0000-000000000001", Command: Command{Executable: os.Args[0], Args: []string{"-test.run=^TestCodexHistoryServerHelper$"}}}
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
		sources = append(sources, codexHistoryBinding{Version: 1, ProjectID: "project", WorkflowID: "workflow", ExecutionID: fmt.Sprintf("execution-%d", i), AttemptID: fmt.Sprintf("attempt-%d", i), Backend: BackendLocal, Protocol: ProtocolLocalProcess, TargetID: "local", Workspace: workspace, SessionID: fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1), Command: Command{Executable: os.Args[0], Args: []string{"-test.run=^TestCodexHistoryServerHelper$"}}, Ended: true})
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
			Backend BackendKind        `json:"backend"`
			Thread  codexHistoryThread `json:"thread"`
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
			base := ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Backend: sample.Backend, Protocol: ProtocolLocalProcess, Agent: "codex", Source: "codex-app-server", SourceSegment: "native", SourceIdentity: "native", SessionID: sample.Thread.ID}
			if sample.Backend == BackendOrca {
				base.Protocol = ProtocolOrcaTerminalV2
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
			if sample.Backend == BackendOrca && tools < 2 {
				t.Fatalf("Orca native tool events lost: %d", tools)
			}
		})
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
	b := codexHistoryBinding{Version: 1, ProjectID: "p", WorkflowID: "w", ExecutionID: "e", AttemptID: "a", Backend: BackendLocal, Protocol: ProtocolLocalProcess, TargetID: "local", Workspace: t.TempDir(), SessionID: "00000000-0000-0000-0000-000000000001", Command: Command{Executable: "codex"}}
	path := filepath.Join(t.TempDir(), codexHistoryBindingFile)
	require(t, writeCodexHistoryBinding(path, b))
	b.AttemptID = "replacement"
	if err := writeCodexHistoryBinding(path, b); err == nil {
		t.Fatal("a native session binding crossed attempts")
	}
}

func TestCodexHistorySourceDiscoveryRejectsWrongWorkspace(t *testing.T) {
	root, workspace := t.TempDir(), t.TempDir()
	logPath := filepath.Join(root, "runs", "workflow", "events.jsonl")
	require(t, os.MkdirAll(filepath.Dir(logPath), 0700))
	b := codexHistoryBinding{Version: 1, ProjectID: "p", WorkflowID: "workflow", ExecutionID: "e", AttemptID: "a", Backend: BackendLocal, Protocol: ProtocolLocalProcess, TargetID: "local", Workspace: workspace, SessionID: "00000000-0000-0000-0000-000000000001", Command: Command{Executable: "codex"}}
	require(t, writeCodexHistoryBinding(localCodexHistoryBindingPath(filepath.Dir(logPath), b.SessionID), b))
	s := &Service{Config: Config{RunDir: root, identity: "p"}, State: Runtime{Runs: []*Run{{RunID: "workflow", ExecutionID: "e", AttemptID: "a", Agent: "codex", Backend: string(BackendLocal), Protocol: string(ProtocolLocalProcess), Target: ExecutionTarget{ID: "local"}, WorktreePath: &workspace, LogPath: &logPath}}}}
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
			if binding.Backend == BackendOrca {
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
			if binding.Backend == BackendOrca && tools < 2 {
				t.Fatal("Orca native tool history missing")
			}
			reports, reportErr := store.ConversationCapabilityReports(ctx, binding.Backend, binding.Protocol, binding.TargetID, 1)
			require(t, reportErr)
			if len(reports) != 1 || reports[0].AgentVersion == "" || reports[0].PlatformVersion == "" || reports[0].PlatformVersion == "windows" || reports[0].ExecutionID != binding.ExecutionID || reports[0].AttemptID != binding.AttemptID || reports[0].SourceIdentity != binding.identity() {
				t.Fatal("native capability report lacks actual agent version")
			}
			statuses := map[string]CapabilityStatus{}
			for _, cap := range reports[0].Capabilities {
				statuses[cap.Name] = cap.Status
			}
			for _, name := range []string{"turn_boundaries", "message_phase", "final_content", "stable_item_identity", "incremental_read", "history_replay"} {
				if statuses[name] != CapabilityVerified {
					t.Fatalf("native report did not verify %s: %+v", name, statuses)
				}
			}
			if binding.Backend == BackendOrca && statuses["input_wait"] != CapabilityNotTested {
				t.Fatal("unobserved Orca wait was overclaimed")
			}
			if binding.Backend == BackendLocal && statuses["input_wait"] != CapabilityUnsupported {
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
