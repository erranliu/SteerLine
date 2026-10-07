package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	testprotocol "github.com/erranliu/SteerLine/internal/testkit/protocol"
)

func integratedEnvironmentState(t *testing.T, e *executionmodel.ExecutionRecord) orcaworker.NativeExecutionState {
	t.Helper()
	if e.Process == nil {
		t.Fatal("native environment has no fixed process")
	}
	for _, envelope := range e.ImplementationData {
		if envelope.ImplementationKey == orcaworker.ExecutionStateCodecKey {
			state, err := orcaworker.DecodeExecutionStateEnvelope(envelope, orcaworker.ExecutionStateIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest, Protocol: executionbackend.ExecutionProtocol(e.Protocol), Process: *e.Process, CoordinatorTerminal: e.Target.CoordinatorTerminal})
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
	var inputs []executionbackend.ImplementationConfigInput
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
	var originalProcess executionbackend.ProcessIdentity
	var originalSession, originalTurn string
	if !renamed {
		uncertain := waitIntegrationRun(t, ctx, first, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return run.TaskID == "901" && e.Environment != nil && e.Environment.Blocker != nil
		})
		if uncertain.run.Process != nil {
			// The read-only reconciliation may already have recovered the original
			// terminal before this public snapshot. It still needs the explicit
			// environment recheck and must never bind a replacement resource.
			state, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
			require(t, err)
			env := uncertain.execution.Environment
			process := uncertain.run.Process
			if !state.FixedAgentTerminalCreated || env.Ready || env.OriginalTerminal == nil || process.Terminal != "term-agent" || process.RuntimeID != "fake-runtime" || process.Incarnation != "inc-agent" || process.WorktreeID != "fake-repo::"+strings.ReplaceAll(state.Path, "\\", "/") || *env.OriginalTerminal != *process {
				t.Fatal("lost receipt bound an unverified or replacement terminal")
			}
		}
		submitIntegrationAnswerOption(t, ctx, f.config, uncertain.run, "recheck_environment")
	} else {
		started := waitIntegrationRun(t, ctx, first, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			state, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
			require(t, err)
			return run.TaskID == "901" && e.Process != nil && e.Environment != nil && e.Environment.Ready &&
				e.TurnNumber == 1 && e.CurrentTurnID != "" && state.Dispatches == 2
		})
		// Renamed-terminal recovery starts from an acknowledged business turn.
		// Readiness alone does not authorize automatic input after a restart.
		originalProcess = *started.execution.Process
		originalSession, originalTurn = started.execution.SessionID, started.execution.CurrentTurnID
		first.stop(t)
		first = startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
	}
	defer first.stop(t)
	recovered := waitIntegrationRun(t, ctx, first, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		state, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
		require(t, err)
		return run.TaskID == "901" && e.Process != nil && e.Environment != nil && state.Dispatches == 2
	})
	state, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
	require(t, err)
	if !state.FixedAgentTerminalCreated || state.Dispatches != 2 || state.CreatedWithAgent != "" || len(recovered.execution.Environment.WriteRoots) != 5 {
		t.Fatalf("original native launch/binding/policy was not reconciled: state=%+v environment=%+v", state, recovered.execution.Environment)
	}
	e := recovered.execution
	launchPath := filepath.Join(f.config.RunDir, "runs", recovered.run.RunID, "turns", e.ExecutionID, "session-launch.json")
	launchRaw, err := os.ReadFile(launchPath)
	require(t, err)
	var launch codexworker.SessionLaunch
	require(t, json.Unmarshal(launchRaw, &launch))
	if !strings.Contains(state.FixedAgentCommand, "'codex-session-launch'") || !strings.Contains(state.FixedAgentCommand, "'"+launchPath+"'") {
		t.Fatal("original terminal did not launch its bound session configuration")
	}
	binding := launch.Binding
	if binding.Version != 1 || binding.ProjectID != e.ProjectIdentity || binding.TaskID != "901" || binding.WorkflowID != recovered.run.RunID || binding.ExecutionID != e.ExecutionID || binding.AttemptID != e.CurrentAttemptID ||
		binding.Backend != executionbackend.BackendOrca || binding.Protocol != executionbackend.ProtocolOrcaTerminalV2 || binding.TargetID != e.Target.ID || binding.ExternalRef != e.Process.WorktreeID ||
		filepath.Clean(filepath.FromSlash(binding.Workspace)) != filepath.Clean(filepath.FromSlash(e.Workspace.Worktree)) || binding.SessionID != "" || launch.Prompt != "" || launch.BindingPath != filepath.Join(filepath.Dir(launchPath), "codex-history.v1.json") {
		t.Fatal("session launcher changed the original execution, worktree or bootstrap intent")
	}
	resources, err := codexworker.DecodePolicyResourceEnvelope(e.PolicyResources.ImplementationData, codexworker.PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest})
	require(t, err)
	modelIndex := slices.Index(launch.Args, "--model")
	if binding.Command.Executable != resources.Settings.Executable || !reflect.DeepEqual(launch.Args, resources.Settings.Args) || launch.Sandbox != "workspace-write" ||
		modelIndex < 0 || modelIndex+1 >= len(launch.Args) || launch.Args[modelIndex+1] != "gpt-6-luna" || !slices.Contains(launch.Args, "model_reasoning_effort=high") {
		t.Fatal("session launcher lost the frozen model, effort or permission arguments")
	}
	saved := integratedEnvironmentState(t, recovered.execution)
	if !saved.AgentBound {
		t.Fatal("reconciled launch replaced its original saved terminal incarnation")
	}
	if renamed && (*recovered.execution.Process != originalProcess || recovered.execution.SessionID != originalSession || recovered.execution.CurrentTurnID != originalTurn || recovered.execution.TurnNumber != 1) {
		t.Fatal("renamed terminal restart changed the original process, session or business turn")
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
	original := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.TaskID == "901" && e.TurnNumber == 1 && e.Process != nil
	})
	state := integratedEnvironmentState(t, original.execution)
	if len(state.Turns) == 0 || state.Turns[len(state.Turns)-1].Purpose != "business" || state.Turns[len(state.Turns)-1].DispatchID == "" {
		t.Fatal("source fixture lost its original business Dispatch")
	}
	originalTurn := state.Turns[len(state.Turns)-1]
	profileRaw := integrationFixedProfileJSON(t, original.runtime, original.run.RunID)
	var profile executionmodel.FixedExecutionProfile
	require(t, json.Unmarshal(profileRaw, &profile))
	config := f.config.ApplyFixedExecutionProfile(profile)
	e := original.execution
	snapshot := executionbackend.LifecycleSnapshot{Ref: executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendKind(e.Backend), Protocol: executionbackend.ExecutionProtocol(e.Protocol), Target: e.Target, Agent: executionbackend.AgentSpec{Name: e.Agent}, RunID: original.run.RunID, AttemptID: original.run.AttemptID, WorkspacePath: e.Workspace.Worktree, WorkspaceBranch: e.Workspace.Branch, BaseCommit: e.Workspace.Base, Generation: e.Generation, External: *e.Process}, Session: executionbackend.SessionContext{ProjectID: e.ProjectIdentity, TaskID: original.run.TaskID, WorkflowRunID: original.run.RunID, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Target: e.Target, Agent: executionbackend.AgentSpec{Name: e.Agent}, SessionID: e.SessionID, Generation: e.SessionGeneration, CurrentTurnID: e.CurrentTurnID, CurrentCommandID: e.CurrentCommandID, LastStoppedTurnID: e.LastStoppedTurnID}, ConfigurationDigest: e.ConfigurationDigest, ResourceGeneration: e.Generation, Implementation: e.ImplementationData, Workspace: executionbackend.WorkspaceIntent{Path: e.Workspace.Worktree, Branch: e.Workspace.Branch, BaseCommit: e.Workspace.Base}}
	port, err := config.ExecutionDirectory.ResolveLifecycle(executionbackend.LifecycleSetup{Snapshot: snapshot, ProfileData: profile.ImplementationData, ProfileID: profile.ID, AcceptedConfigVersion: profile.AcceptedConfigVersion, ProcessHost: config.ProcessHost, RepositoryPath: config.Repository, RunDirectory: config.RunDir, CommandTimeout: 15 * time.Second, GitExecutable: config.Git.Executable, GitArgs: config.Git.Args})
	require(t, err)
	host, ok := port.(executionbackend.HostEnvironmentPort)
	if !ok {
		t.Fatal("actual native adapter lost its output port")
	}
	require(t, testprotocol.UpdateNativeOrcaState(f.nativeStateDir, func(state *testprotocol.NativeOrcaState) { state.EnvironmentMode = false; state.Replies = 1 }))
	first, envelope, err := host.ReadWorkerOutput(ctx, snapshot, executionbackend.HostReadInput{})
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
	changed, envelope, err := host.ReadWorkerOutput(ctx, snapshot, executionbackend.HostReadInput{})
	require(t, err)
	saved, err := orcaworker.DecodeExecutionStateEnvelope(envelope, orcaworker.ExecutionStateIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest, Protocol: executionbackend.ExecutionProtocol(e.Protocol), Process: *e.Process, CoordinatorTerminal: e.Target.CoordinatorTerminal})
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
	waiting := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.TaskID == "901" && e.Environment != nil && e.Environment.OriginalTerminal != nil
	})
	env := waiting.execution.Environment
	if env.Blocker != nil || waiting.execution.Process != nil || env.Ready {
		t.Fatalf("unidentified startup became blocker or ready agent: process=%+v environment=%+v", waiting.execution.Process, env)
	}
	deadline, nonce, terminal := env.Preparation.Deadline, env.Preparation.Nonce, *env.OriginalTerminal
	var launchID, launchKey, launchDigest string
	for _, op := range waiting.runtime.Operations {
		var resource executionbackend.ResourceIntent
		if op.Kind == string(executionbackend.LifecycleCreateResource) && json.Unmarshal(op.Request, &resource) == nil && resource.Purpose == "agent-terminal" && op.Status == "accepted" {
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
	view, err := steerlineapp.LoadDatabaseRuntime(f.config)
	require(t, err)
	for _, e := range view.Executions {
		if e.ExecutionID == waiting.execution.ExecutionID && (e.Environment.Blocker != nil || !e.Environment.Preparation.Deadline.Equal(deadline) || e.Environment.Preparation.Nonce != nonce || *e.Environment.OriginalTerminal != terminal) {
			t.Fatal("waiting reset original preparation or replaced terminal")
		}
	}
	require(t, testprotocol.UpdateNativeOrcaState(f.nativeStateDir, func(state *testprotocol.NativeOrcaState) { state.HoldFixedAgentIdentity = false }))
	resumed := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		state, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
		require(t, err)
		return run.TaskID == "901" && e.Process != nil && state.Dispatches == 1
	})
	confirmed, originalRetained := false, false
	for _, op := range resumed.runtime.Operations {
		var resource executionbackend.ResourceIntent
		if op.OperationID == launchID && op.IdempotencyKey == launchKey && op.PayloadDigest == launchDigest && op.Status == "accepted" {
			originalRetained = true
		}
		if op.Kind == string(executionbackend.LifecycleCreateResource) && json.Unmarshal(op.Request, &resource) == nil && resource.Purpose == "agent-bind" && op.Status == "confirmed" {
			confirmed = true
		}
	}
	if !confirmed || !originalRetained || resumed.execution.Process.Terminal != terminal.Terminal || resumed.execution.Process.Incarnation != terminal.Incarnation || resumed.execution.Environment.Preparation.Nonce != nonce || !resumed.execution.Environment.Preparation.Deadline.Equal(deadline) {
		t.Fatal("identification did not resume same launch/intent/budget exactly once")
	}
}
