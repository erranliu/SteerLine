package steerline

import (
	"context"
	"fmt"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCD65SourceCommitAndHostGate(t *testing.T) {
	t.Run("failed_live_source_does_not_advance_committed_mapper_or_cursor", TestConversationLiveSourceFailureKeepsGapAndMapperPosition)
	t.Run("host_fact_becomes_a_center_manual_gate", testCD65HostGateKeepsTrustedActionSeparate)
	t.Run("active_and_historical_source_budgets_rotate", TestCD65HistoryBudgetRotatesSources)
	t.Run("discovery_cannot_write_a_different_owner", testCD65DiscoveryRejectsForeignOwner)
	t.Run("native_source_replacement_retains_attempt_evidence", TestCD64SourceReplacementRetainsEvidence)
}

func TestCD65HistoryAndPreparation(t *testing.T) {
	t.Run("history_cursor_commits_with_the_conversation_batch", TestCD65HistoryCursorCommitsWithItsConversationBatch)
	t.Run("readiness_probe_requires_original_tool_receipt", TestWorkerEnvironmentER1ProbeAndSettlementGate)
	t.Run("recheck_preserves_original_dispatch", TestWorkerEnvironmentER3RecheckKeepsOriginalDispatch)
	t.Run("deadline_and_restart_keep_manual_gate", TestWorkerEnvironmentER4DeadlineRestartAndLostReceipt)
}

func testCD65HostGateKeepsTrustedActionSeparate(t *testing.T) {
	run := &Run{RunID: "workflow", ExecutionID: "execution", AttemptID: "attempt"}
	request := DecisionRequest{Question: "Worker environment requires review", AllowFreeText: true, HostTarget: &HostActionTarget{ExecutionID: "execution", TurnID: "turn", BlockerID: "blocker"},
		Options: []executionapi.DecisionOption{{ID: "open_terminal", Label: "Open original terminal", Receiver: "center", Action: "open_terminal"},
			{ID: "recheck_environment", Label: "Recheck environment", Receiver: "center", Action: "recheck_environment"}}}
	decision, err := WriteDecisionRequest(run, request)
	require(t, err)
	if decision.Status != "pending" || decision.Request.HostTarget == nil || decision.Request.HostTarget.ExecutionID != run.ExecutionID || len(decision.Request.Options) != 2 {
		t.Fatal("host action request lost its fixed center-owned target or trusted options")
	}
	for _, option := range decision.Request.Options {
		if option.Receiver != "center" || option.Action != option.ID {
			t.Fatalf("host action was not expressed as a trusted center option: %+v", option)
		}
	}
	process := executionapi.ProcessIdentity{Kind: "fixture", Terminal: "terminal", RuntimeID: "runtime", Incarnation: "generation"}
	authorization := &executionapi.HostActionAuthorization{ExecutionID: "execution", AttemptID: "attempt", TurnID: "turn", DecisionID: decision.ID, BlockerID: "blocker", DecisionVersion: decision.Version}
	if err := (executionapi.HostActionInput{Kind: executionapi.HostOpenOriginal, Process: &process, Authorization: authorization}).Validate(); err != nil {
		t.Fatalf("original host action was not accepted by its durable value contract: %v", err)
	}
	if err := (executionapi.HostActionInput{Kind: executionapi.HostOpenOriginal, Process: &executionapi.ProcessIdentity{Terminal: "replacement"}}).Validate(); err == nil {
		t.Fatal("host action without the original runtime/incarnation was accepted")
	}
}

type testConversationHistoryPort struct {
	contracts.ConversationSourcePort
	mu      sync.Mutex
	visits  []string
	cursors []string
}

func (p *testConversationHistoryPort) ReadHistory(_ context.Context, request contracts.ConversationHistoryReadRequest, _ func(*exec.Cmd) (executionapi.StopHandle, error)) (contracts.ConversationHistoryBatch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.visits = append(p.visits, request.Source.Identity)
	p.cursors = append(p.cursors, request.Cursor)
	sequence := uint64(1)
	if request.Cursor != "" {
		value, err := strconv.ParseUint(strings.TrimPrefix(request.Cursor, "cursor:"), 10, 64)
		if err != nil {
			return contracts.ConversationHistoryBatch{}, err
		}
		sequence = value + 1
	}
	event := request.Source.Event
	event.Type, event.SourceEventID, event.SourceEventQuality, event.SourceSeq = ConversationObservation, fmt.Sprintf("%s:%d", request.Source.Identity, sequence), IdentityDerived, sequence
	event.ReceivedAt = now()
	return contracts.ConversationHistoryBatch{Writes: []ConversationWrite{{Event: event}}, Cursor: fmt.Sprintf("cursor:%d", sequence), Sequence: sequence}, nil
}

func TestCD65HistoryBudgetRotatesSources(t *testing.T) {
	databasePath := t.TempDir() + "/history-budget.db"
	// Close the collector/store first; tolerate only a short Windows close
	// handoff on this fixture-owned file before TempDir removes its parent.
	t.Cleanup(func() {
		deadline := time.Now().Add(2 * time.Second)
		for {
			err := os.Remove(databasePath)
			if err == nil || os.IsNotExist(err) {
				return
			}
			if time.Now().After(deadline) {
				t.Errorf("fixture database cleanup: %v", err)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	store, err := OpenSQLiteStore(databasePath)
	require(t, err)
	defer store.Close()
	port := &testConversationHistoryPort{}
	service := &Service{Config: Config{identity: "project", ProcessHost: testProcessHost{}}, domainStore: store, commandStore: store, conversationStore: store, conversationSources: port}
	active := testHistorySource("active", false)
	historical := make([]contracts.ConversationHistorySource, 5)
	for i := range historical {
		historical[i] = testHistorySource(fmt.Sprintf("history-%d", i), true)
	}
	sources := append([]contracts.ConversationHistorySource{active}, historical...)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make(chan []contracts.ConversationHistorySource, 1)
	errorsOut := make(chan error, 4)
	done := make(chan struct{})
	go func() { defer close(done); service.runConversationHistoryCollector(ctx, requests, errorsOut) }()
	for i := 0; i < 5; i++ {
		requests <- sources
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			port.mu.Lock()
			count := len(port.visits)
			port.mu.Unlock()
			if count >= (i+1)*2 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	port.mu.Lock()
	visits := append([]string(nil), port.visits...)
	port.mu.Unlock()
	cancel()
	<-done
	if len(visits) != 10 {
		t.Fatalf("history collector read %d sources, want ten bounded pages: %v", len(visits), visits)
	}
	seen := map[string]int{}
	for i, source := range visits {
		seen[source]++
		if i%2 == 0 && source != "active" {
			t.Fatalf("active source lost the per-page priority: %v", visits)
		}
	}
	if seen["active"] != 5 {
		t.Fatalf("active history did not retain its one-source budget per poll: %v", visits)
	}
	for _, source := range historical {
		if seen[source.Identity] == 0 {
			t.Fatalf("historical source was starved: %s visits=%v", source.Identity, visits)
		}
	}
}

func TestCD65HistoryCursorCommitsWithItsConversationBatch(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(t.TempDir() + "/history-commit.db")
	require(t, err)
	defer store.Close()
	port := &testConversationHistoryPort{}
	service := &Service{Config: Config{identity: "project", ProcessHost: testProcessHost{}}, domainStore: store, commandStore: store, conversationStore: store, conversationSources: port}
	source := testHistorySource("commit-source", false)
	_, err = store.db.Exec(`CREATE TRIGGER reject_history_batch BEFORE INSERT ON conversation_events BEGIN SELECT RAISE(FAIL, 'history batch commit failed'); END`)
	require(t, err)
	if _, err = service.collectConversationHistoryPage(ctx, source); err == nil {
		t.Fatal("history content committed despite an injected batch failure")
	}
	if _, err = store.ConversationCheckpoint(ctx, source.Event.ProjectID, source.Event.ExecutionID, source.Event.AttemptID, source.Event.Source, source.Event.SourceSegment); err == nil {
		t.Fatal("failed conversation batch advanced its candidate source cursor")
	}
	_, err = store.db.Exec(`DROP TRIGGER reject_history_batch`)
	require(t, err)
	if _, err = service.collectConversationHistoryPage(ctx, source); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := store.ConversationCheckpoint(ctx, source.Event.ProjectID, source.Event.ExecutionID, source.Event.AttemptID, source.Event.Source, source.Event.SourceSegment)
	require(t, err)
	if checkpoint.Cursor != "cursor:1" || checkpoint.LocalSeq != 1 {
		t.Fatalf("commit did not publish the original candidate cursor: %+v", checkpoint)
	}
	port.mu.Lock()
	cursors := append([]string(nil), port.cursors...)
	port.mu.Unlock()
	if len(cursors) != 2 || cursors[0] != "" || cursors[1] != "" {
		t.Fatalf("failed batch changed the checkpoint used by its retry: %v", cursors)
	}
}

func testHistorySource(identity string, ended bool) contracts.ConversationHistorySource {
	event := ConversationEvent{Version: ConversationVersion, ProjectID: "project", TaskID: "task", WorkflowID: "workflow", ExecutionID: "execution-" + identity,
		AttemptID: "attempt-" + identity, Source: "fixture-history", SourceRole: contracts.ConversationSourceFallback, SourceSegment: identity, SourceIdentity: identity, SourceEventQuality: IdentityDerived,
		SessionQuality: IdentityUnknown, TurnQuality: IdentityUnknown, ItemQuality: IdentityUnknown, Phase: MessagePhaseUnknown, Type: ConversationSession, SourceEventID: "session:" + identity, ReceivedAt: now()}
	return contracts.ConversationHistorySource{Identity: identity, Event: event, Ended: ended}
}

type foreignConversationDiscovery struct {
	contracts.ConversationSourcePort
	event ConversationEvent
}

func (p foreignConversationDiscovery) DiscoverHistory(context.Context, contracts.ConversationDiscovery) (contracts.ConversationDiscoveryResult, error) {
	return contracts.ConversationDiscoveryResult{Writes: []ConversationWrite{{Event: p.event}}}, nil
}
func testCD65DiscoveryRejectsForeignOwner(t *testing.T) {
	store, err := OpenSQLiteStore(t.TempDir() + "/discovery-owner.db")
	require(t, err)
	defer store.Close()
	id, attempt := strings.Repeat("a", 32), strings.Repeat("b", 32)
	event := testHistorySource("foreign", false).Event
	event.ProjectID = "other-project"
	event.ExecutionID = id
	event.AttemptID = attempt
	service := &Service{Config: Config{identity: "project", RunDir: t.TempDir()}, domainStore: store, commandStore: store, conversationStore: store, conversationSources: foreignConversationDiscovery{event: event}, runs: []*Run{{RunID: "workflow", ExecutionID: id, AttemptID: attempt, DomainTaskID: "task"}}}
	if _, err := service.discoverConversationHistory(context.Background()); err == nil {
		t.Fatal("discovery committed a foreign project's diagnostic")
	}
	var count int
	require(t, store.db.QueryRow(`SELECT COUNT(*) FROM conversation_events`).Scan(&count))
	if count != 0 {
		t.Fatal("rejected discovery leaked a write before its owner was checked")
	}
	event.ProjectID = "project"
	event.AttemptID = "unknown-attempt"
	service.conversationSources = foreignConversationDiscovery{event: event}
	if _, err := service.discoverConversationHistory(context.Background()); err == nil {
		t.Fatal("discovery accepted an unowned attempt")
	}
}

func TestConversationHistoryMissingInjectedHostReturnsError(t *testing.T) {
	service := &Service{Config: Config{identity: "project"}, conversationSources: &testConversationHistoryPort{}}
	if _, err := service.collectConversationHistoryPage(context.Background(), testHistorySource("missing-host", false)); err == nil || !strings.Contains(err.Error(), "managed process host") {
		t.Fatalf("missing host did not fail before reading or writing history: %v", err)
	}
}
