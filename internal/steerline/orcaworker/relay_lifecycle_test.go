package orcaworker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestRelayReconciliationIgnoresOtherTurnHistory(t *testing.T) {
	call := func(context.Context, ...string) (json.RawMessage, error) {
		return json.RawMessage(`{"messages":[{"id":"done-old","run_id":"run","from_handle":"agent","to_handle":"run:run","type":"worker_done","task_id":"old-task","dispatch_id":"old-dispatch","subject":"old","body":"old","outcome":"failed"},{"id":"done-current","run_id":"run","from_handle":"agent","to_handle":"run:run","type":"worker_done","task_id":"task","dispatch_id":"dispatch","subject":"subject","body":"body","outcome":"succeeded"}]}`), nil
	}
	id, err := FindDone(context.Background(), call, "run", "agent", "task", "dispatch", "subject", "body", "succeeded")
	if err != nil || id != "done-current" {
		t.Fatalf("previous turn interfered with original report reconciliation: %q / %v", id, err)
	}
}

func TestSecondTurnRelayIsStillProposedAfterFirstReport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	workflow, execution, attempt := stableIDValue(1), stableIDValue(2), stableIDValue(3)
	first, second := stableIDValue(4), stableIDValue(5)
	worktree := t.TempDir()
	process := executionbackend.ProcessIdentity{Kind: "orca", Terminal: "agent", RuntimeID: "runtime", Incarnation: "inc", WorktreeID: "worktree"}
	snapshot := executionbackend.LifecycleSnapshot{
		Ref:                 executionbackend.ExecutionRef{Protocol: executionbackend.ProtocolOrcaTerminalV2, Target: executionbackend.ExecutionTarget{OrchestrationRun: "native-run", CoordinatorTerminal: "coordinator"}, External: process},
		Session:             executionbackend.SessionContext{ProjectID: "project", WorkflowRunID: workflow, ExecutionID: execution, AttemptID: attempt, CurrentTurnID: second},
		ConfigurationDigest: "config", ResourceGeneration: "resources", Workspace: executionbackend.WorkspaceIntent{Path: worktree},
	}
	state := NativeExecutionState{Coordinator: executionbackend.ProcessIdentity{Terminal: "coordinator", RuntimeID: "runtime"},
		Turns: []NativeTurnState{{TurnID: first, TaskID: "old-task", DispatchID: "old-dispatch", Purpose: "business", Settled: true, RelayReported: true},
			{TurnID: second, TaskID: "task", DispatchID: "dispatch", Purpose: "business"}}}
	path, err := RelayPath(worktree, workflow, second)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	wire, _ := json.Marshal(DoneRequest{WorkflowRunID: workflow, TurnID: second, TaskID: "task", DispatchID: "dispatch", DispatchCapability: "dcap_fixture", Outcome: "succeeded", Subject: "second report", Body: "second body"})
	if err := os.WriteFile(path, wire, 0600); err != nil {
		t.Fatal(err)
	}
	lifecycle := &ExecutionLifecycle{client: New(Config{Output: func(context.Context, time.Duration, ...string) ([]byte, error) {
		return []byte(`{"ok":true,"result":{"runId":"native-run","messages":[]},"_meta":{"runtimeId":"runtime"}}`), nil
	}})}
	observer := &nativeLifecycleObserver{lifecycle: lifecycle, snapshot: snapshot, state: state, turn: state.Turns[1], cancel: cancel,
		events: make(chan executionbackend.LifecycleEvent, 1), done: make(chan error, 1), closed: make(chan struct{})}
	go observer.observe(ctx)
	select {
	case event := <-observer.events:
		if event.Candidate == nil || event.Candidate.Action != executionbackend.LifecycleReportCompletion || event.Candidate.Input.Completion.TurnID != second {
			t.Fatalf("second relay was skipped or redirected: %+v", event.Candidate)
		}
		event.Persisted <- nil
		event.Processed <- nil
		cancel()
	case <-ctx.Done():
		t.Fatal("first report suppressed the second turn relay")
	}
	select {
	case <-observer.closed:
	case <-time.After(time.Second):
		t.Fatal("relay observer did not close")
	}
}
