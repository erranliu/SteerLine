package orcaworker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func TestNativeLifecyclePlansCommittedResourcesAndSeparateInput(t *testing.T) {
	project, workflow, execution, attempt := stableIDValue(1), stableIDValue(2), stableIDValue(3), stableIDValue(4)
	config := ProfileConfig{Executable: "orca", Args: []string{"--native"}, Shell: "powershell.exe", WaitTimeout: "1m"}
	target := executionapi.ExecutionTarget{ID: nativeTargetID(config)}
	snapshot := executionapi.LifecycleSnapshot{
		Ref: executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendOrca,
			Protocol: executionapi.ProtocolOrcaTerminalV2, Target: target, Agent: executionapi.AgentSpec{Name: "codex"},
			RunID: workflow, AttemptID: attempt},
		Session: executionapi.SessionContext{ProjectID: project, TaskID: "808", WorkflowRunID: workflow, ExecutionID: execution,
			AttemptID: attempt, Agent: executionapi.AgentSpec{Name: "codex"}, Target: target},
		ConfigurationDigest: "fixed-profile", ResourceGeneration: stableIDValue(5),
		WorkspacePlan: executionapi.WorkspaceIntent{Path: "C:/repo/worktrees/worker", Branch: "codex/808", BaseCommit: "0123456789012345678901234567890123456789"},
	}
	lifecycle := &ExecutionLifecycle{setup: executionapi.LifecycleSetup{Snapshot: snapshot},
		config: Config{Target: target}, client: New(Config{}), codex: codexProfileForTest(), orca: config}
	step, err := lifecycle.Next(context.Background(), snapshot)
	if err != nil || step.Action != executionapi.LifecyclePrepareWorkspace {
		t.Fatalf("first plan step=%+v error=%v", step, err)
	}

	snapshot.Workspace = snapshot.WorkspacePlan
	snapshot.Ref.WorkspacePath, snapshot.Ref.WorkspaceBranch, snapshot.Ref.BaseCommit = snapshot.Workspace.Path, snapshot.Workspace.Branch, snapshot.Workspace.BaseCommit
	snapshot.Ref.Target.WorktreeID = "repo::C:/repo/worktrees/worker"
	snapshot.Session.Target = snapshot.Ref.Target
	state := NativeExecutionState{RepositoryID: "repo"}
	putState := func() {
		t.Helper()
		envelope, err := NewExecutionStateEnvelope(ExecutionStateIdentity{ProjectID: project, ExecutionID: execution, AttemptID: attempt,
			Generation: snapshot.ResourceGeneration, ConfigDigest: snapshot.ConfigurationDigest, Protocol: snapshot.Ref.Protocol,
			Process: snapshot.Ref.External, CoordinatorTerminal: snapshot.Ref.Target.CoordinatorTerminal}, state)
		if err != nil {
			t.Fatal(err)
		}
		snapshot.Implementation = []executionapi.ImplementationDataEnvelope{envelope}
	}
	putState()
	step, err = lifecycle.Next(context.Background(), snapshot)
	if err != nil || step.Action != executionapi.LifecycleCreateResource || step.Input.Resource.Purpose != "agent-terminal" {
		t.Fatalf("agent plan step=%+v error=%v", step, err)
	}

	snapshot.Ref.External = executionapi.ProcessIdentity{Kind: "orca", Terminal: "agent", RuntimeID: "runtime", Incarnation: "agent-inc",
		WorktreeID: snapshot.Ref.Target.WorktreeID}
	state.AgentBound = true
	putState()
	step, err = lifecycle.Next(context.Background(), snapshot)
	if err != nil || step.Input.Resource.Purpose != "coordinator" {
		t.Fatalf("coordinator plan step=%+v error=%v", step, err)
	}

	state.Coordinator = executionapi.ProcessIdentity{Kind: "orca-native-coordinator", Terminal: "coordinator", RuntimeID: "runtime", Incarnation: "coord-inc"}
	snapshot.Ref.Target.CoordinatorTerminal = "coordinator"
	snapshot.Session.Target = snapshot.Ref.Target
	putState()
	step, err = lifecycle.Next(context.Background(), snapshot)
	if err != nil || step.Input.Resource.Purpose != "workflow" {
		t.Fatalf("Run plan step=%+v error=%v", step, err)
	}
	snapshot.Ref.Target.OrchestrationRun = "run-fixed"
	snapshot.Session.Target = snapshot.Ref.Target
	state.RelayIgnored = true
	putState()
	if step, err = lifecycle.Next(context.Background(), snapshot); err != nil || !step.Done {
		t.Fatalf("idle plan=%+v error=%v", step, err)
	}

	pending := &executionapi.TurnInput{CommandID: stableIDValue(6), Prompt: "run the worker task", Purpose: "readiness"}
	snapshot.PendingTurn = pending
	putState()
	step, err = lifecycle.Next(context.Background(), snapshot)
	if err != nil || step.Action != executionapi.LifecycleCreateResource || step.Input.Resource.Purpose != "conversation" || step.Input.Resource.TurnPurpose != "readiness" {
		t.Fatalf("Task plan step=%+v error=%v", step, err)
	}
	state.Turns = append(state.Turns, NativeTurnState{TurnID: pending.CommandID, TaskID: "task-fixed", Purpose: "readiness"})
	putState()
	step, err = lifecycle.Next(context.Background(), snapshot)
	if err != nil || step.Action != executionapi.LifecycleStartTurn {
		t.Fatalf("Dispatch plan step=%+v error=%v", step, err)
	}
	state.Turns[0].DispatchID, state.Turns[0].InputSubmissionPending = "dispatch-fixed", true
	putState()
	step, err = lifecycle.Next(context.Background(), snapshot)
	if err != nil || step.Action != executionapi.LifecycleSubmitInput {
		t.Fatalf("input submission plan step=%+v error=%v", step, err)
	}
}

func TestProbeMutationRunsAsOneDurableLifecycleAction(t *testing.T) {
	gitDir, worktree := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(gitDir, "info"), 0700); err != nil {
		t.Fatal(err)
	}
	identity := executionapi.ProbeIdentity{Execution: stableIDValue(7), Turn: stableIDValue(8), Nonce: stableIDValue(9), ProfileDigest: "profile",
		Runtime: "runtime", Terminal: "terminal", Incarnation: "incarnation", Worktree: worktree}
	snapshot := executionapi.LifecycleSnapshot{Ref: executionapi.ExecutionRef{Backend: executionapi.BackendOrca, Protocol: executionapi.ProtocolOrcaTerminalV2,
		WorkspacePath: worktree, External: executionapi.ProcessIdentity{Kind: "orca", Terminal: "terminal", RuntimeID: "runtime", Incarnation: "incarnation"}},
		Session:             executionapi.SessionContext{ProjectID: gitDir, ExecutionID: identity.Execution, AttemptID: stableIDValue(10), WorkflowRunID: stableIDValue(11)},
		ConfigurationDigest: "config", ResourceGeneration: stableIDValue(12)}
	state := NativeExecutionState{Coordinator: executionapi.ProcessIdentity{Kind: "orca", Terminal: "coordinator", RuntimeID: "runtime", Incarnation: "incarnation"},
		Turns: []NativeTurnState{{TurnID: identity.Turn, Purpose: "readiness"}}}
	snapshot.PreparationProbe = &identity
	envelope, err := NewExecutionStateEnvelope(ExecutionStateIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID,
		AttemptID: snapshot.Session.AttemptID, Generation: snapshot.ResourceGeneration, ConfigDigest: snapshot.ConfigurationDigest,
		Protocol: snapshot.Ref.Protocol, Process: snapshot.Ref.External}, state)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Implementation = []executionapi.ImplementationDataEnvelope{envelope}
	lifecycle := &ExecutionLifecycle{setup: executionapi.LifecycleSetup{Snapshot: snapshot}, client: New(Config{}), orca: ProfileConfig{WaitTimeout: "1m"}}
	request := executionapi.LifecycleRequest{Snapshot: snapshot, Operation: executionapi.Operation{ID: "operation", Action: string(executionapi.LifecycleHostAction), PayloadDigest: "digest"},
		Input: executionapi.LifecycleInput{Host: &executionapi.HostActionInput{Kind: executionapi.HostPrepareProbeFiles, Probe: &identity}}}
	receipt, err := lifecycle.ExecuteHostAction(context.Background(), request)
	if err != nil || receipt.Disposition != executionapi.OperationConfirmed {
		t.Fatalf("probe file action receipt=%+v error=%v", receipt, err)
	}
	plan, err := BuildProbe(identity)
	if err != nil {
		t.Fatal(err)
	}
	if present, err := CheckProbeFiles(identity, plan); err != nil || !present {
		t.Fatalf("probe files were not prepared by the action: present=%v err=%v", present, err)
	}
	ignored, err := ReadinessIgnored(gitDir)
	if err != nil || !ignored {
		t.Fatalf("probe ignore rule was not prepared by the action: ignored=%v err=%v", ignored, err)
	}
	reconciled, err := lifecycle.ReconcileHostAction(context.Background(), request)
	if err != nil || reconciled.Disposition != executionapi.OperationConfirmed {
		t.Fatalf("probe action receipt could not be reconciled read-only: %+v err=%v", reconciled, err)
	}
	cleanup := request
	cleanup.Operation.ID = "cleanup-operation"
	cleanup.Input.Host = &executionapi.HostActionInput{Kind: executionapi.HostCleanupProbeFiles, Probe: &identity}
	if _, err = lifecycle.ExecuteHostAction(context.Background(), cleanup); err != nil {
		t.Fatal(err)
	}
	if absent, err := ProbeFilesAbsent(identity); err != nil || !absent {
		t.Fatalf("cleanup action left probe files: absent=%v err=%v", absent, err)
	}
	exclude, err := os.ReadFile(filepath.Join(gitDir, "info", "exclude"))
	if err != nil || !strings.Contains(string(exclude), "/.steerline-readiness/") {
		t.Fatalf("persisted ignore rule disappeared during probe cleanup: %q err=%v", exclude, err)
	}
}

func stableIDValue(last byte) string {
	const hex = "0123456789abcdef"
	value := make([]byte, 32)
	for i := range value {
		value[i] = hex[0]
	}
	value[len(value)-1] = hex[int(last)]
	return string(value)
}

func codexProfileForTest() codexworker.ProfileConfig {
	return codexworker.ProfileConfig{Executable: "codex", WindowsSandbox: "unelevated"}
}
