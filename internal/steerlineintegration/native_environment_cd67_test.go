package steerlineintegration

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerline/testprotocol"
	"github.com/erranliu/SteerLine/internal/steerlineapp"
	"os"
	"strings"
	"testing"
	"time"
)

func integratedEnvironmentState(t *testing.T, e *steerline.ExecutionRecord) orcaworker.NativeExecutionState {
	t.Helper()
	if e.Process == nil {
		t.Fatal("native environment has no fixed process")
	}
	for _, envelope := range e.ImplementationData {
		if envelope.ImplementationKey == orcaworker.ExecutionStateCodecKey {
			state, err := orcaworker.DecodeExecutionStateEnvelope(envelope, orcaworker.ExecutionStateIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest, Protocol: executionapi.ExecutionProtocol(e.Protocol), Process: *e.Process, CoordinatorTerminal: e.Target.CoordinatorTerminal})
			require(t, err)
			return state
		}
	}
	t.Fatal("native execution state envelope missing")
	return orcaworker.NativeExecutionState{}
}

func TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal(t *testing.T) {
	testIntegratedManagedLaunch(t, false)
}
func TestNativeOrcaFixedLunaLaunchReconcilesRenamedTerminalAfterRestart(t *testing.T) {
	testIntegratedManagedLaunch(t, true)
}
func testIntegratedManagedLaunch(t *testing.T, renamed bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	f := newLifecycleProject(t, "orca")
	raw, err := os.ReadFile(steerlineapp.GlobalExecutionConfigPath())
	require(t, err)
	var global map[string]json.RawMessage
	require(t, json.Unmarshal(raw, &global))
	var profiles []map[string]json.RawMessage
	require(t, json.Unmarshal(global["execution_profiles"], &profiles))
	var inputs []executionapi.ImplementationConfigInput
	require(t, json.Unmarshal(profiles[0]["implementation_configs"], &inputs))
	for i := range inputs {
		if inputs[i].ImplementationKey == codexworker.ProfileCodecKey {
			settings, err := codexworker.DecodeProfileConfig(inputs[i].Payload)
			require(t, err)
			settings.Model, settings.ReasoningEffort = "gpt-6-luna", "high"
			inputs[i].Payload, err = codexworker.EncodeProfileConfig(settings)
			require(t, err)
		}
	}
	profiles[0]["implementation_configs"] = mustJSON(inputs)
	global["execution_profiles"] = mustJSON(profiles)
	require(t, os.WriteFile(steerlineapp.GlobalExecutionConfigPath(), mustJSON(global), 0600))
	f.config, err = steerlineapp.LoadConfig(f.configPath)
	require(t, err)
	initial := testprotocol.NativeOrcaState{EnvironmentMode: true, BusinessActive: true, ReadinessMode: "success", LoseFixedTerminalOnce: !renamed, RenameFixedAgentAfterCreate: renamed}
	require(t, testprotocol.WriteNativeOrcaState(f.nativeStateDir, initial))
	createIntegrationTask(t, ctx, f, 901)
	first := startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
	claimIntegrationTask(t, ctx, f, 901, first.instanceID)
	if !renamed {
		uncertain := waitIntegrationRun(t, ctx, first, f.config, func(run *steerline.Run, e *steerline.ExecutionRecord) bool {
			return run.TaskID == "901" && e.Environment != nil && e.Environment.Blocker != nil && run.Decision != nil && run.Decision.Status == "pending"
		})
		if uncertain.run.Process != nil {
			t.Fatal("unknown terminal receipt guessed")
		}
		submitIntegrationAnswerOption(t, ctx, f.config, uncertain.run, "recheck_environment")
	} else {
		waitIntegrationRun(t, ctx, first, f.config, func(run *steerline.Run, e *steerline.ExecutionRecord) bool {
			return run.TaskID == "901" && e.Process != nil && e.Environment != nil && e.Environment.Ready
		})
		first.stop(t)
		first = startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
	}
	defer first.stop(t)
	recovered := waitIntegrationRun(t, ctx, first, f.config, func(run *steerline.Run, e *steerline.ExecutionRecord) bool {
		state, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
		require(t, err)
		return run.TaskID == "901" && e.Process != nil && e.Environment != nil && state.Dispatches == 2
	})
	state, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
	require(t, err)
	if !state.FixedAgentTerminalCreated || state.Dispatches != 2 || !strings.Contains(state.FixedAgentCommand, "'gpt-6-luna'") || state.CreatedWithAgent != "" || len(recovered.execution.Environment.WriteRoots) != 5 {
		t.Fatalf("original native launch/binding/policy was not reconciled: state=%+v environment=%+v", state, recovered.execution.Environment)
	}
	saved := integratedEnvironmentState(t, recovered.execution)
	if !saved.AgentBound {
		t.Fatal("reconciled launch replaced its original saved terminal incarnation")
	}
}

func TestNativeSourceReplacementRetainsOutputEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	f := newLifecycleProject(t, "orca")
	require(t, testprotocol.WriteNativeOrcaState(f.nativeStateDir, testprotocol.NativeOrcaState{EnvironmentMode: true, BusinessActive: true, ReadinessMode: "success"}))
	createIntegrationTask(t, ctx, f, 901)
	service := startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
	defer service.stop(t)
	claimIntegrationTask(t, ctx, f, 901, service.instanceID)
	original := waitIntegrationRun(t, ctx, service, f.config, func(run *steerline.Run, e *steerline.ExecutionRecord) bool {
		return run.TaskID == "901" && e.TurnNumber == 1 && e.Process != nil
	})
	state := integratedEnvironmentState(t, original.execution)
	if len(state.Turns) == 0 || state.Turns[len(state.Turns)-1].Purpose != "business" || state.Turns[len(state.Turns)-1].DispatchID == "" {
		t.Fatal("source fixture lost its original business Dispatch")
	}
	originalTurn := state.Turns[len(state.Turns)-1]
	profileRaw := integrationFixedProfileJSON(t, original.runtime, original.run.RunID)
	var profile steerline.FixedExecutionProfile
	require(t, json.Unmarshal(profileRaw, &profile))
	config := f.config.ApplyFixedExecutionProfile(profile)
	e := original.execution
	snapshot := executionapi.LifecycleSnapshot{Ref: executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendKind(e.Backend), Protocol: executionapi.ExecutionProtocol(e.Protocol), Target: e.Target, Agent: executionapi.AgentSpec{Name: e.Agent}, RunID: original.run.RunID, AttemptID: original.run.AttemptID, WorkspacePath: e.Workspace.Worktree, WorkspaceBranch: e.Workspace.Branch, BaseCommit: e.Workspace.Base, Generation: e.Generation, External: *e.Process}, Session: executionapi.SessionContext{ProjectID: e.ProjectIdentity, TaskID: original.run.TaskID, WorkflowRunID: original.run.RunID, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Target: e.Target, Agent: executionapi.AgentSpec{Name: e.Agent}, SessionID: e.SessionID, Generation: e.SessionGeneration, CurrentTurnID: e.CurrentTurnID, CurrentCommandID: e.CurrentCommandID, LastStoppedTurnID: e.LastStoppedTurnID}, ConfigurationDigest: e.ConfigurationDigest, ResourceGeneration: e.Generation, Implementation: e.ImplementationData, Workspace: executionapi.WorkspaceIntent{Path: e.Workspace.Worktree, Branch: e.Workspace.Branch, BaseCommit: e.Workspace.Base}}
	port, err := config.ExecutionDirectory.ResolveLifecycle(executionapi.LifecycleSetup{Snapshot: snapshot, ProfileData: profile.ImplementationData, ProfileID: profile.ID, AcceptedConfigVersion: profile.AcceptedConfigVersion, Policy: profile.ExecutionPolicy, ProcessHost: config.ProcessHost, RepositoryPath: config.Repository, RunDirectory: config.RunDir, CommandTimeout: 15 * time.Second, GitExecutable: config.Git.Executable, GitArgs: config.Git.Args})
	require(t, err)
	host, ok := port.(contracts.HostEnvironmentPort)
	if !ok {
		t.Fatal("actual native adapter lost its output port")
	}
	require(t, testprotocol.UpdateNativeOrcaState(f.nativeStateDir, func(state *testprotocol.NativeOrcaState) { state.EnvironmentMode = false; state.Replies = 1 }))
	first, envelope, err := host.ReadWorkerOutput(ctx, snapshot, contracts.HostReadInput{})
	require(t, err)
	if first.Source == "" || first.Cursor == "" {
		t.Fatalf("original native output omitted source/cursor: %+v", first)
	}
	for i := range snapshot.Implementation {
		if snapshot.Implementation[i].ImplementationKey == envelope.ImplementationKey {
			snapshot.Implementation[i] = envelope.Clone()
		}
	}
	require(t, testprotocol.UpdateNativeOrcaState(f.nativeStateDir, func(state *testprotocol.NativeOrcaState) { state.ForceSourceChange = true }))
	changed, envelope, err := host.ReadWorkerOutput(ctx, snapshot, contracts.HostReadInput{})
	require(t, err)
	saved, err := orcaworker.DecodeExecutionStateEnvelope(envelope, orcaworker.ExecutionStateIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest, Protocol: executionapi.ExecutionProtocol(e.Protocol), Process: *e.Process, CoordinatorTerminal: e.Target.CoordinatorTerminal})
	require(t, err)
	turn := saved.Turns[len(saved.Turns)-1]
	nativeState, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
	require(t, err)
	if !changed.SourceChanged || !changed.UpdateAttemptCursor || changed.Source != "transcript" || changed.Cursor == "" || !changed.ContentComplete || turn.DispatchID != originalTurn.DispatchID || turn.OutputSource != "transcript" || !turn.OutputComplete || turn.OutputCursor == "" || !nativeState.TranscriptAfterSwitch {
		t.Fatal("source replacement lost original native content, completeness, Dispatch or cursor")
	}
}

func TestWorkerEnvironmentWaitsForOriginalAgentDetection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	f := newLifecycleProject(t, "orca")
	require(t, testprotocol.WriteNativeOrcaState(f.nativeStateDir, testprotocol.NativeOrcaState{EnvironmentMode: true, BusinessActive: true, HoldFixedAgentIdentity: true, ReadinessMode: "hold"}))
	createIntegrationTask(t, ctx, f, 901)
	service := startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
	defer service.stop(t)
	claimIntegrationTask(t, ctx, f, 901, service.instanceID)
	waiting := waitIntegrationRun(t, ctx, service, f.config, func(run *steerline.Run, e *steerline.ExecutionRecord) bool {
		return run.TaskID == "901" && e.Environment != nil && e.Environment.OriginalTerminal != nil
	})
	env := waiting.execution.Environment
	if env.Blocker != nil || waiting.execution.Process != nil || env.Ready {
		t.Fatalf("unidentified startup became blocker or ready agent: process=%+v environment=%+v", waiting.execution.Process, env)
	}
	deadline, nonce, terminal := env.Preparation.Deadline, env.Preparation.Nonce, *env.OriginalTerminal
	var launchID, launchKey, launchDigest string
	for _, op := range waiting.runtime.Operations {
		var resource executionapi.ResourceIntent
		if op.Kind == string(executionapi.LifecycleCreateResource) && json.Unmarshal(op.Request, &resource) == nil && resource.Purpose == "agent-terminal" && op.Status == "accepted" {
			launchID, launchKey, launchDigest = op.OperationID, op.IdempotencyKey, op.PayloadDigest
		}
	}
	accepted := launchID != ""
	state, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
	require(t, err)
	if !accepted || state.Dispatches != 0 {
		t.Fatal("startup receipt lost or input sent before original identification")
	}
	// Additional public actor passes observe only the original launch.
	time.Sleep(1200 * time.Millisecond)
	view, err := service.queries.ReadProjectView(ctx, "901", 100)
	require(t, err)
	for _, e := range view.Runtime.Executions {
		if e.ExecutionID == waiting.execution.ExecutionID && (e.Environment.Blocker != nil || !e.Environment.Preparation.Deadline.Equal(deadline) || e.Environment.Preparation.Nonce != nonce || *e.Environment.OriginalTerminal != terminal) {
			t.Fatal("waiting reset original preparation or replaced terminal")
		}
	}
	require(t, testprotocol.UpdateNativeOrcaState(f.nativeStateDir, func(state *testprotocol.NativeOrcaState) { state.HoldFixedAgentIdentity = false }))
	resumed := waitIntegrationRun(t, ctx, service, f.config, func(run *steerline.Run, e *steerline.ExecutionRecord) bool {
		state, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
		require(t, err)
		return run.TaskID == "901" && e.Process != nil && state.Dispatches == 1
	})
	confirmed, originalRetained := false, false
	for _, op := range resumed.runtime.Operations {
		var resource executionapi.ResourceIntent
		if op.OperationID == launchID && op.IdempotencyKey == launchKey && op.PayloadDigest == launchDigest && op.Status == "accepted" {
			originalRetained = true
		}
		if op.Kind == string(executionapi.LifecycleCreateResource) && json.Unmarshal(op.Request, &resource) == nil && resource.Purpose == "agent-bind" && op.Status == "confirmed" {
			confirmed = true
		}
	}
	if !confirmed || !originalRetained || resumed.execution.Process.Terminal != terminal.Terminal || resumed.execution.Process.Incarnation != terminal.Incarnation || resumed.execution.Environment.Preparation.Nonce != nonce || !resumed.execution.Environment.Preparation.Deadline.Equal(deadline) {
		t.Fatal("identification did not resume same launch/intent/budget exactly once")
	}
}
