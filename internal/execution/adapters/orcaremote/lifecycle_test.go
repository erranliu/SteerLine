package orca_remote_worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os/exec"
	"testing"
	"time"

	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type lifecycleTestHost struct{}

func (lifecycleTestHost) StartManaged(*exec.Cmd) (executionbackend.StopHandle, error) {
	panic("not used")
}

func (lifecycleTestHost) InspectProcess(*executionbackend.ProcessIdentity) string { panic("not used") }

func (lifecycleTestHost) NewProcessGeneration() string { panic("not used") }

func (lifecycleTestHost) WriteExecutionFile(string, []byte) error { panic("not used") }

func (lifecycleTestHost) OutputCommand(context.Context, executionbackend.LaunchCommand) ([]byte, error) {
	panic("not used")
}

func remoteLifecycleSetup(t *testing.T) (executionbackend.LifecycleSetup, executionbackend.LifecycleSnapshot) {
	t.Helper()
	const accepted = "accepted-profile-v1"
	remotePayload, err := EncodeProfileConfig(ProfileConfig{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "2m", Model: "pinned-model", ReasoningEffort: "high", ConnectionID: "connection", EnvironmentID: "environment", RepositorySelector: "repo", CoordinatorTerminal: "configured-coordinator", PollInterval: "1s", CapabilityReportVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	data := make([]executionbackend.ImplementationDataEnvelope, 0, 3)
	for _, item := range []struct {
		key     string
		payload []byte
	}{{ProfileCodecKey, remotePayload}} {
		version := ProfileCodecVersion
		envelope, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{
			CodecVersion: version, Purpose: executionbackend.ImplementationConfigPurpose, ImplementationKey: item.key,
			ProjectID: "project", ProfileID: "remote", AcceptedConfigVersion: accepted,
		}, item.payload)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, envelope)
	}
	target := executionbackend.ExecutionTarget{ID: "orca-supervised:connection", ConnectionID: "connection", ExternalRuntime: "environment"}
	ref := executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendOrca,
		Protocol: executionbackend.ProtocolOrcaSupervised, Target: target, Agent: executionbackend.AgentSpec{Name: "codex"},
		RunID: "00000000000000000000000000000001", AttemptID: "00000000000000000000000000000002", BaseCommit: "0123456789012345678901234567890123456789"}
	session := executionbackend.SessionContext{ProjectID: "project", TaskID: "901", WorkflowRunID: ref.RunID,
		ExecutionID: "00000000000000000000000000000003", AttemptID: ref.AttemptID, Agent: ref.Agent, Target: target}
	snapshot := executionbackend.LifecycleSnapshot{Ref: ref, Session: session, ConfigurationDigest: "profile-digest", ResourceGeneration: "resource-generation",
		WorkspacePlan: executionbackend.WorkspaceIntent{Path: `C:\worktrees\planned`, Branch: "codex/901", BaseCommit: ref.BaseCommit}}
	setup := executionbackend.LifecycleSetup{Snapshot: snapshot, ProfileData: data, ProfileID: "remote", AcceptedConfigVersion: accepted,
		ProcessHost: lifecycleTestHost{}, RepositoryPath: `C:\repo`, RunDirectory: `C:\state`, TargetBranch: "main", RemoteName: "origin"}
	return setup, snapshot
}

func remoteLifecycleRequest(snapshot executionbackend.LifecycleSnapshot, action executionbackend.LifecycleAction, input executionbackend.LifecycleInput) executionbackend.LifecycleRequest {
	var value any
	switch action {
	case executionbackend.LifecycleDeliverFeedback:
		value = input.Feedback
	case executionbackend.LifecycleAcknowledge:
		value = input.Ack
	}
	wire, _ := json.Marshal(value)
	digest := sha256.Sum256(wire)
	operation := executionbackend.Operation{Version: 1, ID: "operation-id", RunID: snapshot.Session.WorkflowRunID, AttemptID: snapshot.Session.AttemptID,
		Backend: snapshot.Ref.Backend, Protocol: snapshot.Ref.Protocol, Target: snapshot.Ref.Target, Action: string(action), IdempotencyKey: "operation-key",
		PayloadDigest: hex.EncodeToString(digest[:]), Status: executionbackend.OperationInFlight, Ref: &snapshot.Ref}
	operation.LifecycleIdentity = &executionbackend.LifecycleOperationIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID, ConfigurationDigest: snapshot.ConfigurationDigest, ResourceGeneration: snapshot.ResourceGeneration, InputBindingDigest: executionbackend.LifecycleBindingDigest(snapshot), Ref: snapshot.Ref}
	return executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: operation, Input: input}
}

func TestLifecyclePlanPersistsWorkspaceAndResourceBoundaries(t *testing.T) {
	setup, snapshot := remoteLifecycleSetup(t)
	config := Config{Repository: setup.RepositoryPath, ProjectID: snapshot.Session.ProjectID, Agent: snapshot.Ref.Agent.Name,
		Model: "pinned-model", ReasoningEffort: "high", CommandTimeout: "30s",
		Settings: Settings{ConnectionID: "connection", EnvironmentID: "environment", RepositorySelector: "repo", CoordinatorTerminal: "configured-coordinator", PollInterval: "1s"},
		Call:     func(context.Context, time.Duration, ...string) ([]byte, error) { return nil, nil },
		Origin:   func(context.Context) ([]byte, error) { return []byte("https://example.invalid/repo.git"), nil }}
	port := &ExecutionLifecycle{setup: setup, config: config, client: New(config)}
	candidate, err := port.Next(context.Background(), snapshot)
	if err != nil || candidate.Validate() != nil || candidate.Action != executionbackend.LifecyclePrepareWorkspace || candidate.Input.Workspace == nil || *candidate.Input.Workspace != snapshot.WorkspacePlan {
		t.Fatalf("remote planner did not propose the fixed workspace intent: %+v / %v", candidate, err)
	}
	snapshot.Workspace = executionbackend.WorkspaceIntent{Path: `/remote/worktree`, Branch: "sl-901-run", BaseCommit: snapshot.Ref.BaseCommit}
	snapshot.Ref.WorkspacePath, snapshot.Ref.WorkspaceBranch = snapshot.Workspace.Path, snapshot.Workspace.Branch
	candidate, err = port.Next(context.Background(), snapshot)
	if err != nil || candidate.Validate() != nil || candidate.Action != executionbackend.LifecycleCreateResource || candidate.Input.Resource == nil || candidate.Input.Resource.Purpose != "coordinator" {
		t.Fatalf("remote planner did not request a distinct coordinator creation step: %+v / %v", candidate, err)
	}
	state := orcaworker.NativeExecutionState{RemoteRuntimeID: "remote-runtime", RepositoryID: "remote-repo",
		Coordinator: executionbackend.ProcessIdentity{Terminal: "coordinator", RuntimeID: "center-runtime"}}
	snapshot.Ref.Target.CoordinatorTerminal = state.Coordinator.Terminal
	snapshot.Session.Target.CoordinatorTerminal = state.Coordinator.Terminal
	envelope, err := orcaworker.NewExecutionStateEnvelope(orcaworker.ExecutionStateIdentity{ProjectID: snapshot.Session.ProjectID,
		ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID, Generation: snapshot.ResourceGeneration,
		ConfigDigest: snapshot.ConfigurationDigest, Protocol: snapshot.Ref.Protocol, CoordinatorTerminal: state.Coordinator.Terminal}, state)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Implementation = []executionbackend.ImplementationDataEnvelope{envelope}
	snapshot.Ref.Target.OrchestrationRun = "run-original"
	snapshot.PendingTurn = &executionbackend.TurnInput{CommandID: "00000000000000000000000000000004", Prompt: "original prompt"}
	candidate, err = port.Next(context.Background(), snapshot)
	if err != nil || candidate.Validate() != nil || candidate.Action != executionbackend.LifecycleCreateResource || candidate.Input.Resource == nil || candidate.Input.Resource.Purpose != "conversation" || candidate.Input.Resource.TurnID != snapshot.PendingTurn.CommandID {
		t.Fatalf("remote planner did not persist conversation creation separately: %+v / %v", candidate, err)
	}
	state.Turns = append(state.Turns, orcaworker.NativeTurnState{TurnID: snapshot.PendingTurn.CommandID, TaskID: "task-original"})
	envelope, err = orcaworker.NewExecutionStateEnvelope(orcaworker.ExecutionStateIdentity{ProjectID: snapshot.Session.ProjectID,
		ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID, Generation: snapshot.ResourceGeneration,
		ConfigDigest: snapshot.ConfigurationDigest, Protocol: snapshot.Ref.Protocol, CoordinatorTerminal: state.Coordinator.Terminal}, state)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Implementation[0] = envelope
	candidate, err = port.Next(context.Background(), snapshot)
	if err != nil || candidate.Validate() != nil || candidate.Action != executionbackend.LifecycleStartTurn || candidate.Input.Turn == nil || candidate.Input.Turn.CommandID != snapshot.PendingTurn.CommandID {
		t.Fatalf("remote planner did not separate Dispatch from saved conversation creation: %+v / %v", candidate, err)
	}
}

func TestLifecyclePreflightChecksRemoteBeforeLaunching(t *testing.T) {
	setup, _ := remoteLifecycleSetup(t)
	port, err := NewExecutionLifecycle(setup)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := port.(*ExecutionLifecycle)
	calls := 0
	lifecycle.config.Call = func(context.Context, time.Duration, ...string) ([]byte, error) {
		calls++
		return nil, errors.New("remote unavailable")
	}
	_, err = port.PreflightLifecycle(context.Background(), setup)
	if err == nil || calls != 1 {
		t.Fatalf("preflight failed to probe fixed remote: calls=%d err=%v", calls, err)
	}
}

func TestLifecycleReconcilesUnknownQuestionAnswerWithoutSending(t *testing.T) {
	snapshot, state := remoteSnapshotForTest(t)
	state.Turns = []orcaworker.NativeTurnState{{TurnID: "turn", TaskID: "task", DispatchID: "dispatch"}}
	envelope, err := orcaworker.NewExecutionStateEnvelope(stateIdentity(snapshot), state)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Implementation = []executionbackend.ImplementationDataEnvelope{envelope}
	routeData, _ := json.Marshal(orcaworker.QuestionRouteData{RunID: "run", TaskID: "task", DispatchID: "dispatch", MessageID: "question", Terminal: "agent-terminal"})
	route, err := executionbackend.NewQuestionRoute(orcaworker.QuestionRouteImplementationKey, 1, executionbackend.QuestionRouteIdentity{
		ProjectID: snapshot.Session.ProjectID, WorkflowRunID: snapshot.Session.WorkflowRunID, ExecutionID: snapshot.Session.ExecutionID,
		AttemptID: snapshot.Session.AttemptID, Generation: snapshot.Session.Generation, TurnID: "turn", QuestionID: "question",
	}, routeData)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.QuestionRoute = &route
	callCount := 0
	config := lifecycleRemoteTestConfig(func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
		callCount++
		if len(args) >= 2 && args[0] == "orchestration" && args[1] == "inbox" {
			return remoteEnvelope("center-runtime", map[string]any{"messages": []any{}}), nil
		}
		if len(args) >= 2 && args[0] == "orchestration" && args[1] == "reply" {
			t.Fatal("unknown answer reconciliation resent a reply")
		}
		return nil, errors.New("unexpected remote call")
	})
	lifecycle := &ExecutionLifecycle{setup: executionbackend.LifecycleSetup{Snapshot: snapshot}, config: config, client: New(config)}
	request := remoteLifecycleRequest(snapshot, executionbackend.LifecycleDeliverFeedback, executionbackend.LifecycleInput{Feedback: &executionbackend.FeedbackInput{
		DecisionID: "decision", QuestionID: "question", Message: "answer", Answer: &executionbackend.Answer{Answers: map[string][]string{"question": {"answer"}}},
	}})
	receipt, observer, err := lifecycle.Reconcile(context.Background(), request)
	if observer != nil || err == nil || receipt.Disposition != executionbackend.OperationUnknown || callCount != 1 {
		t.Fatalf("unknown answer did not remain read-only: receipt=%+v calls=%d err=%v", receipt, callCount, err)
	}
}

func TestLifecycleConfirmsOriginalDeliveryAckByReadOnlyInbox(t *testing.T) {
	snapshot, _ := remoteSnapshotForTest(t)
	snapshot.PendingDeliveryID = "delivery-original"
	callCount := 0
	config := lifecycleRemoteTestConfig(func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
		callCount++
		if len(args) >= 2 && args[0] == "orchestration" && args[1] == "run-show" {
			return remoteEnvelope("center-runtime", map[string]any{"run": map[string]any{"id": "run", "coordinator_handle": "coordinator"}}), nil
		}
		if len(args) >= 2 && args[0] == "orchestration" && args[1] == "check" {
			if hasArg(args, "--ack") {
				t.Fatal("unknown acknowledgement reconciliation issued an ack")
			}
			return remoteEnvelope("center-runtime", map[string]any{"runId": "run", "deliveryId": "delivery-next", "messages": []any{}, "timedOut": false}), nil
		}
		return nil, errors.New("unexpected remote call")
	})
	lifecycle := &ExecutionLifecycle{setup: executionbackend.LifecycleSetup{Snapshot: snapshot}, config: config, client: New(config)}
	request := remoteLifecycleRequest(snapshot, executionbackend.LifecycleAcknowledge, executionbackend.LifecycleInput{Ack: &executionbackend.AcknowledgeInput{DeliveryID: "delivery-original"}})
	receipt, observer, err := lifecycle.Reconcile(context.Background(), request)
	if err != nil || observer != nil || receipt.Disposition != executionbackend.OperationConfirmed || receipt.DeliveryID != "delivery-original" || callCount != 2 {
		t.Fatalf("original ack was not confirmed read-only: receipt=%+v calls=%d err=%v", receipt, callCount, err)
	}
}

func TestLifecycleReconcilesReleasedDispatchWithDistinctTurnID(t *testing.T) {
	snapshot, state := remoteSnapshotForTest(t)
	state.Turns = []orcaworker.NativeTurnState{{TurnID: "turn", TaskID: "task", DispatchID: "dispatch", Settled: true}}
	envelope, err := orcaworker.NewExecutionStateEnvelope(stateIdentity(snapshot), state)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Implementation = []executionbackend.ImplementationDataEnvelope{envelope}
	config := lifecycleRemoteTestConfig(func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "orchestration" && args[1] == "worker-list" {
			return remoteEnvelope("center-runtime", map[string]any{"workers": []any{map[string]any{"RunID": "run", "TaskID": "task", "DispatchID": "dispatch", "WorktreeID": "repo::worktree", "State": "released"}}}), nil
		}
		t.Fatalf("release reconciliation attempted a mutation: %v", args)
		return nil, errors.New("unexpected call")
	})
	lifecycle := &ExecutionLifecycle{setup: executionbackend.LifecycleSetup{Snapshot: snapshot}, config: config, client: New(config)}
	request := remoteLifecycleRequest(snapshot, executionbackend.LifecycleCleanup, executionbackend.LifecycleInput{Cleanup: &executionbackend.CleanupInput{Resources: []executionbackend.ProcessIdentity{snapshot.Ref.External}}})
	receipt, _, err := lifecycle.Reconcile(context.Background(), request)
	if err != nil || receipt.Disposition != executionbackend.OperationConfirmed || len(receipt.Implementation) != 1 {
		t.Fatalf("released Dispatch could not be reconciled: %+v / %v", receipt, err)
	}
	updated, err := orcaworker.DecodeExecutionStateEnvelope(receipt.Implementation[0], stateIdentity(snapshot))
	if err != nil || !updated.Turns[0].Released {
		t.Fatalf("release receipt did not advance the original turn: %+v / %v", updated, err)
	}
}

func remoteSnapshotForTest(t *testing.T) (executionbackend.LifecycleSnapshot, orcaworker.NativeExecutionState) {
	t.Helper()
	target := executionbackend.ExecutionTarget{ID: "orca-supervised:connection", ConnectionID: "connection", ExternalRuntime: "environment",
		CoordinatorTerminal: "coordinator", OrchestrationRun: "run", ExternalTask: "task", Dispatch: "dispatch",
		ExecutionHost: "environment", WorktreeID: "repo::worktree"}
	process := executionbackend.ProcessIdentity{Kind: "orca_supervised", RuntimeID: "remote-epoch", CoordinatorRuntimeID: "center-runtime",
		Terminal: "agent-terminal", Incarnation: "incarnation", OrchestrationRun: "run", Task: "task", Dispatch: "dispatch",
		ExecutionHost: "environment", WorktreeID: "repo::worktree"}
	ref := executionbackend.ExecutionRef{Version: 1, Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaSupervised,
		Target: target, Agent: executionbackend.AgentSpec{Name: "codex"}, RunID: "00000000000000000000000000000001", AttemptID: "00000000000000000000000000000002",
		WorkspacePath: "/remote/worktree", WorkspaceBranch: "branch", BaseCommit: "0123456789012345678901234567890123456789", External: process, Generation: "incarnation"}
	session := executionbackend.SessionContext{ProjectID: "project", TaskID: "901", WorkflowRunID: ref.RunID, ExecutionID: "00000000000000000000000000000003",
		AttemptID: ref.AttemptID, Agent: ref.Agent, Target: target, SessionID: SessionID(process), Generation: process.Incarnation,
		CurrentTurnID: "turn", CurrentCommandID: "turn"}
	snapshot := executionbackend.LifecycleSnapshot{Ref: ref, Session: session, ConfigurationDigest: "config-digest", ResourceGeneration: "resource-generation",
		Workspace: executionbackend.WorkspaceIntent{Path: ref.WorkspacePath, Branch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit}}
	state := orcaworker.NativeExecutionState{RemoteRuntimeID: process.RuntimeID, RepositoryID: "repo", Coordinator: executionbackend.ProcessIdentity{Kind: "orca-remote-coordinator", Terminal: "coordinator", RuntimeID: "center-runtime"}}
	return snapshot, state
}

func lifecycleRemoteTestConfig(call func(context.Context, time.Duration, ...string) ([]byte, error)) Config {
	return Config{Settings: Settings{ConnectionID: "connection", EnvironmentID: "environment", RepositorySelector: "repo", CoordinatorTerminal: "coordinator", PollInterval: "1s"},
		Repository: "/repo", ProjectID: "project", Agent: "codex", Model: "model", ReasoningEffort: "high", CommandTimeout: "1s", Call: call,
		Origin: func(context.Context) ([]byte, error) { return []byte("https://example.invalid/repo.git"), nil }}
}

func remoteEnvelope(runtimeID string, result any) []byte {
	data, _ := json.Marshal(result)
	return []byte(`{"ok":true,"result":` + string(data) + `,"_meta":{"runtimeId":"` + runtimeID + `"}}`)
}

func hasArg(args []string, target string) bool {
	for _, value := range args {
		if value == target {
			return true
		}
	}
	return false
}
