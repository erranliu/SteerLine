package orca_remote_worker

import (
	"context"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestRemoteResourceFreezesHostAuthorityWithoutLocalProbe(t *testing.T) {
	setup, snapshot := remoteLifecycleSetup(t)
	identity := PolicyResourceIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID, Generation: snapshot.ResourceGeneration, ConfigDigest: snapshot.ConfigurationDigest}
	envelope, err := BuildNativeResource(identity, setup.ProfileID, setup.AcceptedConfigVersion, "D:/remote/worktree", "codex", setup.ProfileData)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := DecodeNativeResource(envelope, identity)
	if err != nil || payload.EnvironmentID != "environment" || payload.ConnectionID != "connection" || payload.Model != "pinned-model" || payload.Workspace != "D:/remote/worktree" || payload.LaunchSource != "remote-orca-native" {
		t.Fatalf("remote authority not frozen: %+v / %v", payload, err)
	}
	identity.Generation = "replacement"
	if _, err := DecodeNativeResource(envelope, identity); err == nil {
		t.Fatal("replacement generation accepted original remote resources")
	}
}

func TestRemoteInputAcceptanceDoesNotManufactureStartedEvent(t *testing.T) {
	snapshot, state := remoteSnapshotForTest(t)
	state.Turns = []orcaworker.NativeTurnState{{TurnID: "turn", TaskID: "task", DispatchID: "dispatch"}}
	envelope, err := orcaworker.NewExecutionStateEnvelope(stateIdentity(snapshot), state)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Implementation = []executionbackend.ImplementationDataEnvelope{envelope}
	entered := make(chan struct{}, 1)
	config := lifecycleRemoteTestConfig(func(ctx context.Context, _ time.Duration, args ...string) ([]byte, error) {
		if strings.Join(args[:2], " ") == "orchestration run-show" {
			return remoteEnvelope("center-runtime", map[string]any{"run": map[string]any{"id": "run", "coordinator_handle": "coordinator"}}), nil
		}
		if hasArg(args, "check") {
			entered <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		t.Fatalf("unexpected observer command: %v", args)
		return nil, nil
	})
	lifecycle := &ExecutionLifecycle{setup: executionbackend.LifecycleSetup{Snapshot: snapshot}, config: config, client: New(config)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observer := newLifecycleObserver(ctx, lifecycle, snapshot)
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
		defer closeCancel()
		if err := observer.Close(closeCtx); err != nil {
			t.Error(err)
		}
	}()
	select {
	case event := <-observer.Events():
		t.Fatalf("input acceptance emitted a started fact before worker evidence: %+v", event.Fact)
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("original inbox was not observed")
	}
	select {
	case event := <-observer.Events():
		t.Fatalf("pending inbox fabricated an execution event: %+v", event.Fact)
	default:
	}
	cancel()
	select {
	case <-observer.Done():
		t.Fatal("mutation context cancellation detached accepted worker observation")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestRemoteFeedbackSelectsPendingCommandBeforePreviousTurn(t *testing.T) {
	snapshot, state := remoteSnapshotForTest(t)
	state.Turns = []orcaworker.NativeTurnState{{TurnID: "old", TaskID: "old-task", DispatchID: "old-dispatch", Settled: true}, {TurnID: "feedback", TaskID: "new-task", DispatchID: "new-dispatch"}}
	snapshot.Session.CurrentTurnID = "old"
	snapshot.Session.CurrentCommandID = "old"
	snapshot.Session.PendingCommandID = "feedback"
	snapshot.Session.PendingTurnID = ""
	turn := currentTurn(&state, snapshot)
	if turn == nil || turn.TurnID != "feedback" {
		t.Fatal("new feedback observer selected prior settled Dispatch")
	}
	bound := lifecycleSnapshotWithBinding(snapshot, snapshot.Ref.Target, snapshot.Ref.External, state)
	if bound.Session.CurrentTurnID != "feedback" || bound.Session.CurrentCommandID != "feedback" {
		t.Fatal("feedback observer did not bind its own pending input")
	}
}

func TestRemoteCapabilityReportPreservesClaimedBranchAndBase(t *testing.T) {
	target := executionbackend.ExecutionTarget{ID: "orca-supervised:connection", ConnectionID: "connection", ExternalRuntime: "environment", Branch: "test/base", BaseRevision: "fixed-commit"}
	report := NewBackend(Config{Settings: Settings{ConnectionID: "connection", EnvironmentID: "environment"}}).report(target, executionbackend.AgentSpec{Name: "codex"}, PreflightResult{CenterVersion: "1.4.209", RemoteVersion: "1.4.215"})
	if report.Target != target {
		t.Fatal("remote report erased claimed branch/base")
	}
	// Delivery alone never authorizes implicit closing of a remote session.
	setup, snapshot := remoteLifecycleSetup(t)
	snapshot.WorkflowPhase = "completed"
	snapshot.TaskDelivered = true
	lifecycle, err := NewExecutionLifecycle(setup)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := lifecycle.Next(context.Background(), snapshot)
	if err != nil || !candidate.Done {
		t.Fatalf("remote delivery created a cleanup intent: %+v / %v", candidate, err)
	}
}
