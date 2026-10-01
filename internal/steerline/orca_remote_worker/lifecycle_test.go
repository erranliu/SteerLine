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

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

type lifecycleTestHost struct{}

func (lifecycleTestHost) StartManaged(*exec.Cmd) (executionapi.StopHandle, error) { panic("not used") }
func (lifecycleTestHost) InspectProcess(*executionapi.ProcessIdentity) string     { panic("not used") }
func (lifecycleTestHost) NewProcessGeneration() string                            { panic("not used") }
func (lifecycleTestHost) WriteExecutionFile(string, []byte) error                 { panic("not used") }
func (lifecycleTestHost) OutputCommand(context.Context, executionapi.LaunchCommand) ([]byte, error) {
	panic("not used")
}

func remoteLifecycleSetup(t *testing.T) (executionapi.LifecycleSetup, executionapi.LifecycleSnapshot) {
	t.Helper()
	const accepted = "accepted-profile-v1"
	policy := executionapi.ExecutionPolicy{Filesystem: executionapi.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace"}, Network: "allow", Escalation: "deny"}
	codexPayload, err := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: "codex", Model: "pinned-model", ReasoningEffort: "high"})
	if err != nil {
		t.Fatal(err)
	}
	orcaPayload, err := orcaworker.EncodeProfileConfig(orcaworker.ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "2m"})
	if err != nil {
		t.Fatal(err)
	}
	remotePayload, err := EncodeProfileConfig(ProfileConfig{ConnectionID: "connection", EnvironmentID: "environment", RepositorySelector: "repo", CoordinatorTerminal: "configured-coordinator", PollInterval: "1s", CapabilityReportVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	data := make([]executionapi.ImplementationDataEnvelope, 0, 3)
	for _, item := range []struct {
		key     string
		payload []byte
	}{{codexworker.ProfileCodecKey, codexPayload}, {orcaworker.ProfileCodecKey, orcaPayload}, {ProfileCodecKey, remotePayload}} {
		envelope, err := executionapi.NewImplementationDataEnvelope(executionapi.ImplementationDataEnvelope{
			CodecVersion: 1, Purpose: executionapi.ImplementationConfigPurpose, ImplementationKey: item.key,
			ProjectID: "project", ProfileID: "remote", AcceptedConfigVersion: accepted,
		}, item.payload)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, envelope)
	}
	target := executionapi.ExecutionTarget{ID: "orca-supervised:connection", ConnectionID: "connection", ExternalRuntime: "environment"}
	ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendOrca,
		Protocol: executionapi.ProtocolOrcaSupervised, Target: target, Agent: executionapi.AgentSpec{Name: "codex"},
		RunID: "00000000000000000000000000000001", AttemptID: "00000000000000000000000000000002", BaseCommit: "0123456789012345678901234567890123456789"}
	session := executionapi.SessionContext{ProjectID: "project", TaskID: "901", WorkflowRunID: ref.RunID,
		ExecutionID: "00000000000000000000000000000003", AttemptID: ref.AttemptID, Agent: ref.Agent, Target: target}
	snapshot := executionapi.LifecycleSnapshot{Ref: ref, Session: session, ConfigurationDigest: "profile-digest", ResourceGeneration: "resource-generation",
		WorkspacePlan: executionapi.WorkspaceIntent{Path: `C:\worktrees\planned`, Branch: "codex/901", BaseCommit: ref.BaseCommit}}
	setup := executionapi.LifecycleSetup{Snapshot: snapshot, ProfileData: data, ProfileID: "remote", AcceptedConfigVersion: accepted,
		Policy: policy, ProcessHost: lifecycleTestHost{}, RepositoryPath: `C:\repo`, RunDirectory: `C:\state`, TargetBranch: "main", RemoteName: "origin"}
	return setup, snapshot
}

func remoteLifecycleRequest(snapshot executionapi.LifecycleSnapshot, action executionapi.LifecycleAction, input executionapi.LifecycleInput) executionapi.LifecycleRequest {
	var value any
	switch action {
	case executionapi.LifecycleDeliverFeedback:
		value = input.Feedback
	case executionapi.LifecycleAcknowledge:
		value = input.Ack
	}
	wire, _ := json.Marshal(value)
	digest := sha256.Sum256(wire)
	operation := executionapi.Operation{Version: 1, ID: "operation-id", RunID: snapshot.Session.WorkflowRunID, AttemptID: snapshot.Session.AttemptID,
		Backend: snapshot.Ref.Backend, Protocol: snapshot.Ref.Protocol, Target: snapshot.Ref.Target, Action: string(action), IdempotencyKey: "operation-key",
		PayloadDigest: hex.EncodeToString(digest[:]), Status: executionapi.OperationInFlight, Ref: &snapshot.Ref}
	return executionapi.LifecycleRequest{Snapshot: snapshot, Operation: operation, Input: input}
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
	if err != nil || candidate.Validate() != nil || candidate.Action != executionapi.LifecyclePrepareWorkspace || candidate.Input.Workspace == nil || *candidate.Input.Workspace != snapshot.WorkspacePlan {
		t.Fatalf("remote planner did not propose the fixed workspace intent: %+v / %v", candidate, err)
	}
	snapshot.Workspace = executionapi.WorkspaceIntent{Path: `/remote/worktree`, Branch: "sl-901-run", BaseCommit: snapshot.Ref.BaseCommit}
	snapshot.Ref.WorkspacePath, snapshot.Ref.WorkspaceBranch = snapshot.Workspace.Path, snapshot.Workspace.Branch
	candidate, err = port.Next(context.Background(), snapshot)
	if err != nil || candidate.Validate() != nil || candidate.Action != executionapi.LifecycleCreateResource || candidate.Input.Resource == nil || candidate.Input.Resource.Purpose != "coordinator" {
		t.Fatalf("remote planner did not request a distinct coordinator creation step: %+v / %v", candidate, err)
	}
	state := orcaworker.NativeExecutionState{RemoteRuntimeID: "remote-runtime", RepositoryID: "remote-repo",
		Coordinator: executionapi.ProcessIdentity{Terminal: "coordinator", RuntimeID: "center-runtime"}}
	snapshot.Ref.Target.CoordinatorTerminal = state.Coordinator.Terminal
	snapshot.Session.Target.CoordinatorTerminal = state.Coordinator.Terminal
	envelope, err := orcaworker.NewExecutionStateEnvelope(orcaworker.ExecutionStateIdentity{ProjectID: snapshot.Session.ProjectID,
		ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID, Generation: snapshot.ResourceGeneration,
		ConfigDigest: snapshot.ConfigurationDigest, Protocol: snapshot.Ref.Protocol, CoordinatorTerminal: state.Coordinator.Terminal}, state)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Implementation = []executionapi.ImplementationDataEnvelope{envelope}
	snapshot.Ref.Target.OrchestrationRun = "run-original"
	snapshot.PendingTurn = &executionapi.TurnInput{CommandID: "00000000000000000000000000000004", Prompt: "original prompt"}
	candidate, err = port.Next(context.Background(), snapshot)
	if err != nil || candidate.Validate() != nil || candidate.Action != executionapi.LifecycleCreateResource || candidate.Input.Resource == nil || candidate.Input.Resource.Purpose != "conversation" || candidate.Input.Resource.TurnID != snapshot.PendingTurn.CommandID {
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
	if err != nil || candidate.Validate() != nil || candidate.Action != executionapi.LifecycleStartTurn || candidate.Input.Turn == nil || candidate.Input.Turn.CommandID != snapshot.PendingTurn.CommandID {
		t.Fatalf("remote planner did not separate Dispatch from saved conversation creation: %+v / %v", candidate, err)
	}
}

func TestLifecyclePreflightRetainsRemotePermissionGate(t *testing.T) {
	setup, _ := remoteLifecycleSetup(t)
	port, err := NewExecutionLifecycle(setup)
	if err != nil {
		t.Fatal(err)
	}
	_, err = port.PreflightLifecycle(context.Background(), setup)
	var backendErr *executionapi.BackendError
	if !errors.As(err, &backendErr) || backendErr.Code != executionapi.BackendUnsupported {
		t.Fatalf("remote lifecycle preflight inferred permission evidence: %v", err)
	}
}

func TestLifecycleReconcilesUnknownQuestionAnswerWithoutSending(t *testing.T) {
	snapshot, state := remoteSnapshotForTest(t)
	state.Turns = []orcaworker.NativeTurnState{{TurnID: "turn", TaskID: "task", DispatchID: "dispatch"}}
	envelope, err := orcaworker.NewExecutionStateEnvelope(stateIdentity(snapshot), state)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Implementation = []executionapi.ImplementationDataEnvelope{envelope}
	routeData, _ := json.Marshal(orcaworker.QuestionRouteData{RunID: "run", TaskID: "task", DispatchID: "dispatch", MessageID: "question", Terminal: "agent-terminal"})
	route, err := executionapi.NewQuestionRoute(orcaworker.QuestionRouteImplementationKey, 1, executionapi.QuestionRouteIdentity{
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
	lifecycle := &ExecutionLifecycle{setup: executionapi.LifecycleSetup{Snapshot: snapshot}, config: config, client: New(config)}
	request := remoteLifecycleRequest(snapshot, executionapi.LifecycleDeliverFeedback, executionapi.LifecycleInput{Feedback: &executionapi.FeedbackInput{
		DecisionID: "decision", QuestionID: "question", Message: "answer", Answer: &executionapi.Answer{Answers: map[string][]string{"question": {"answer"}}},
	}})
	receipt, observer, err := lifecycle.Reconcile(context.Background(), request)
	if observer != nil || err == nil || receipt.Disposition != executionapi.OperationUnknown || callCount != 1 {
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
	lifecycle := &ExecutionLifecycle{setup: executionapi.LifecycleSetup{Snapshot: snapshot}, config: config, client: New(config)}
	request := remoteLifecycleRequest(snapshot, executionapi.LifecycleAcknowledge, executionapi.LifecycleInput{Ack: &executionapi.AcknowledgeInput{DeliveryID: "delivery-original"}})
	receipt, observer, err := lifecycle.Reconcile(context.Background(), request)
	if err != nil || observer != nil || receipt.Disposition != executionapi.OperationConfirmed || receipt.DeliveryID != "delivery-original" || callCount != 2 {
		t.Fatalf("original ack was not confirmed read-only: receipt=%+v calls=%d err=%v", receipt, callCount, err)
	}
}

func remoteSnapshotForTest(t *testing.T) (executionapi.LifecycleSnapshot, orcaworker.NativeExecutionState) {
	t.Helper()
	target := executionapi.ExecutionTarget{ID: "orca-supervised:connection", ConnectionID: "connection", ExternalRuntime: "environment",
		CoordinatorTerminal: "coordinator", OrchestrationRun: "run", ExternalTask: "task", Dispatch: "dispatch",
		ExecutionHost: "environment", WorktreeID: "repo::worktree"}
	process := executionapi.ProcessIdentity{Kind: "orca_supervised", RuntimeID: "remote-epoch", CoordinatorRuntimeID: "center-runtime",
		Terminal: "agent-terminal", Incarnation: "incarnation", OrchestrationRun: "run", Task: "task", Dispatch: "dispatch",
		ExecutionHost: "environment", WorktreeID: "repo::worktree"}
	ref := executionapi.ExecutionRef{Version: 1, Backend: executionapi.BackendOrca, Protocol: executionapi.ProtocolOrcaSupervised,
		Target: target, Agent: executionapi.AgentSpec{Name: "codex"}, RunID: "00000000000000000000000000000001", AttemptID: "00000000000000000000000000000002",
		WorkspacePath: "/remote/worktree", WorkspaceBranch: "branch", BaseCommit: "0123456789012345678901234567890123456789", External: process, Generation: "incarnation"}
	session := executionapi.SessionContext{ProjectID: "project", TaskID: "901", WorkflowRunID: ref.RunID, ExecutionID: "00000000000000000000000000000003",
		AttemptID: ref.AttemptID, Agent: ref.Agent, Target: target, SessionID: SessionID(process), Generation: process.Incarnation,
		CurrentTurnID: "turn", CurrentCommandID: "turn"}
	snapshot := executionapi.LifecycleSnapshot{Ref: ref, Session: session, ConfigurationDigest: "config-digest", ResourceGeneration: "resource-generation",
		Workspace: executionapi.WorkspaceIntent{Path: ref.WorkspacePath, Branch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit}}
	state := orcaworker.NativeExecutionState{RemoteRuntimeID: process.RuntimeID, RepositoryID: "repo", Coordinator: executionapi.ProcessIdentity{Kind: "orca-remote-coordinator", Terminal: "coordinator", RuntimeID: "center-runtime"}}
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
