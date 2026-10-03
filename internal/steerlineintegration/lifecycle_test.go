package steerlineintegration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/testprotocol"
	"github.com/erranliu/SteerLine/internal/steerlineapp"
)

// This suite starts the production service with its formal resolver injection,
// claims an isolated native task through CommandService, and observes it through
// the public runtime/task query APIs. The Codex and Orca executables are protocol
// fixtures implemented by this test binary; no product CLI or user project is used.
func TestCD64OriginalSessionReconciliation(t *testing.T) {
	t.Run("remote_admission_stays_blocked", TestRemotePublicationClaimRequiresPermissionEvidence)
	t.Run("app_server_two_turns", testIntegratedAppServerTwoTurns)
	t.Run("native_original_session_manual_gate", testIntegratedNativeOriginalSessionGate)
	t.Run("remote_protocol_original_assertions", testCD64RemoteProtocolAssertions)
}

func testIntegratedAppServerTwoTurns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fixture := newLifecycleProject(t, "local")
	createIntegrationTask(t, ctx, fixture, 901)
	resolver := &integrationLifecycleResolver{}
	service := startIntegrationService(t, ctx, fixture.config, resolver)
	defer service.stop(t)
	claimIntegrationTask(t, ctx, fixture, 901, service.instanceID)

	first := waitIntegrationRun(t, ctx, service, fixture.config, func(run *steerline.Run, execution *steerline.ExecutionRecord) bool {
		return run.Decision != nil && run.Decision.Status == "pending" && execution.TurnNumber == 1 && execution.CurrentTurnID == execution.LastStoppedTurnID
	})
	if first.run.Process == nil {
		t.Fatal("first app-server receipt omitted its process identity")
	}
	firstSession, firstProcess, firstTurn := first.execution.SessionID, *first.run.Process, first.execution.CurrentTurnID
	if firstSession != integrationAppServerSession || firstProcess.Kind != "os" || firstTurn == "" {
		t.Fatalf("first app-server receipt lost its original session/process/turn: session=%q process=%+v turn=%q", firstSession, firstProcess, firstTurn)
	}
	assertQueriedTaskPhase(t, ctx, fixture.config, "901", "waiting_decision")
	submitIntegrationAnswer(t, ctx, fixture.config, first.run)

	second := waitIntegrationRun(t, ctx, service, fixture.config, func(run *steerline.Run, execution *steerline.ExecutionRecord) bool {
		return execution.TurnNumber == 2 && execution.CurrentTurnID != firstTurn && execution.CurrentTurnID == execution.LastStoppedTurnID &&
			run.Decision != nil && run.Decision.Status == "pending"
	})
	if second.execution.SessionID != firstSession || second.run.Process == nil || *second.run.Process != firstProcess {
		t.Fatalf("second app-server turn replaced its original session or process: first=%+v/%s second=%+v/%s", firstProcess, firstSession, second.run.Process, second.execution.SessionID)
	}
	starts := lifecycleOperations(second.runtime, "start_worker_turn")
	prepares := lifecycleOperations(second.runtime, "prepare_worker_session")
	if len(starts) != 2 || len(prepares) != 1 {
		t.Fatalf("app-server operation receipts = prepare:%d start:%d, want one original session and two turns", len(prepares), len(starts))
	}
	assertNoTaskDelivery(t, second.runtime, second.run)
	assertQueriedTaskPhase(t, ctx, fixture.config, "901", "waiting_decision")
	if resolver.count(executionapi.ProtocolLocalCodexAppServer) == 0 {
		t.Fatal("public service flow never resolved the app-server lifecycle adapter")
	}
}

func testIntegratedNativeOriginalSessionGate(t *testing.T) { testIntegratedNativeSessionGate(t, false) }
func testIntegratedNativeSessionGate(t *testing.T, changeConfig bool) {
	if runtime.GOOS != "windows" {
		t.Skip("native Orca protocol fixture uses the Windows host contract")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	fixture := newLifecycleProject(t, "orca")
	initial := testprotocol.NativeOrcaState{EnvironmentMode: true, BusinessActive: true, FirstBusinessDispatch: 2, EndReadinessOnBusiness: true, LoseQuestionAckOnce: true, LoseReplyOnce: true}
	require(t, testprotocol.WriteNativeOrcaState(fixture.nativeStateDir, initial))
	createIntegrationTask(t, ctx, fixture, 902)
	resolver := &integrationLifecycleResolver{beforeNativeExecute: func(request executionapi.LifecycleRequest) error {
		if request.Operation.Action == string(executionapi.LifecycleCreateResource) && request.Input.Resource != nil && request.Input.Resource.Purpose == "conversation" {
			return testprotocol.UpdateNativeOrcaState(fixture.nativeStateDir, func(state *testprotocol.NativeOrcaState) {
				if len(state.Tasks) == 5 {
					state.AgentIncarnation = "replaced-agent"
				}
			})
		}
		return nil
	}}
	firstService := startIntegrationService(t, ctx, fixture.config, resolver)
	claimIntegrationTask(t, ctx, fixture, 902, firstService.instanceID)
	first := waitIntegrationRun(t, ctx, firstService, fixture.config, func(run *steerline.Run, execution *steerline.ExecutionRecord) bool {
		return run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute != nil &&
			execution.CurrentTurnID != "" && execution.Target.Dispatch == "ctx-2" && execution.TurnNumber == 1
	})
	if first.run.Process == nil {
		t.Fatal("native receipt omitted its process identity")
	}
	original := *first.run.Process
	sessionID, sessionGeneration := first.execution.SessionID, first.execution.SessionGeneration
	workspace, runID, dispatchID := derefString(first.run.WorktreePath), first.run.Target.OrchestrationRun, first.run.Target.Dispatch
	questionID := first.run.Decision.Request.QuestionRoute.QuestionID
	questionDecisionID := first.run.Decision.ID
	if original.Kind != "orca" || original.Terminal == "" || original.Incarnation == "" || sessionID == "" || sessionGeneration != original.Incarnation || runID != "run-fake" || dispatchID != "ctx-2" {
		t.Fatalf("native receipt omitted the original Run/Task/Dispatch/session identity: process=%+v session=%q/%q target=%+v", original, sessionID, sessionGeneration, first.run.Target)
	}
	acknowledged := waitIntegrationRun(t, ctx, firstService, fixture.config, func(run *steerline.Run, execution *steerline.ExecutionRecord) bool {
		return execution.PendingDeliveryID == ""
	})
	if !hasAcknowledgement(acknowledged.runtime, "delivery-ask-1", "confirmed") {
		t.Fatalf("original inbox acknowledgement was not reconciled: %+v", lifecycleOperations(acknowledged.runtime, "acknowledge_delivery"))
	}
	if acknowledged.run.Target.Dispatch != dispatchID || acknowledged.run.Decision.Request.QuestionRoute.QuestionID != questionID {
		t.Fatal("question consumption changed the original Dispatch or question")
	}
	firstService.stop(t)
	if changeConfig {
		fixture.config = changedIntegrationConfig(t, fixture)
	}
	secondService := startIntegrationService(t, ctx, fixture.config, resolver)
	defer secondService.stop(t)
	recovered := waitIntegrationRun(t, ctx, secondService, fixture.config, func(run *steerline.Run, execution *steerline.ExecutionRecord) bool {
		return execution.RecoveryRequired && run.Decision != nil && run.Decision.Status == "pending" &&
			run.Decision.Request.QuestionRoute != nil && run.Decision.Request.QuestionRoute.QuestionID == questionID &&
			(execution.RecoveryState == "live" || execution.RecoveryState == "question")
	})
	if recovered.run.Process == nil || recovered.run.Process.Terminal != original.Terminal || recovered.run.Process.Incarnation != original.Incarnation ||
		recovered.execution.SessionID != sessionID || recovered.execution.SessionGeneration != sessionGeneration ||
		derefString(recovered.run.WorktreePath) != workspace || recovered.run.Target.OrchestrationRun != runID || recovered.run.Target.Dispatch != dispatchID {
		t.Fatalf("service restart replaced the original native resources: run=%+v execution=%+v", recovered.run, recovered.execution)
	}
	if changeConfig && string(integrationFixedProfileJSON(t, first.runtime, first.run.RunID)) != string(integrationFixedProfileJSON(t, recovered.runtime, recovered.run.RunID)) {
		t.Fatal("changed current config rewrote the original frozen key, codec, target or policy")
	}
	if state, err := testprotocol.ReadNativeOrcaState(fixture.nativeStateDir); err != nil || state.Dispatches != 2 || state.Replies != 0 || state.Sends != 0 {
		t.Fatalf("restart crossed the manual input gate: state=%+v err=%v", state, err)
	}
	assertQueriedTaskPhase(t, ctx, fixture.config, "902", "waiting_decision")
	submitIntegrationAnswer(t, ctx, fixture.config, recovered.run)

	settled := waitIntegrationRun(t, ctx, secondService, fixture.config, func(run *steerline.Run, execution *steerline.ExecutionRecord) bool {
		return run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute == nil &&
			execution.CurrentTurnID != "" && execution.CurrentTurnID == execution.LastStoppedTurnID && execution.Target.Dispatch == dispatchID
	})
	if settled.run.Process == nil || settled.run.Process.Terminal != original.Terminal || settled.run.Process.Incarnation != original.Incarnation ||
		settled.execution.SessionID != sessionID || derefString(settled.run.WorktreePath) != workspace || settled.run.Target.OrchestrationRun != runID || settled.run.Target.Dispatch != dispatchID {
		t.Fatalf("original native question did not settle on its original resources: run=%+v execution=%+v", settled.run, settled.execution)
	}
	state, err := testprotocol.ReadNativeOrcaState(fixture.nativeStateDir)
	require(t, err)
	if state.Dispatches != 2 || state.Replies != 1 || state.Sends != 0 || !state.Acked["delivery-ask-1"] || !state.QuestionAckLossArmed || state.LoseAckOnce || state.LoseReplyOnce {
		t.Fatalf("native dispatch/reply/ack was repeated or changed route: %+v", state)
	}
	if resolver.count(executionapi.ProtocolOrcaTerminalV2) == 0 {
		t.Fatal("public service flow never resolved the native Orca lifecycle adapter")
	}
	if !decisionWasAccepted(settled.runtime, questionDecisionID) || !hasOperationStatus(lifecycleOperations(settled.runtime, "reply_execution_question"), "reply_execution_question", "confirmed") {
		t.Fatalf("original native question answer lacks its confirmed receipt: decision=%+v operations=%+v", settled.runtime.Decisions, lifecycleOperations(settled.runtime, "reply_execution_question"))
	}
	assertNoTaskDelivery(t, settled.runtime, settled.run)
	assertQueriedTaskPhase(t, ctx, fixture.config, "902", "waiting_decision")
	// Preserve the original native producer regressions, using public answers
	// rather than the removed private actor methods.
	current := settled
	for _, next := range []struct {
		dispatch int
		loss     string
	}{{3, "dispatch"}, {4, "send"}, {5, "task"}} {
		t.Logf("native original mutation case: %s", next.loss)
		require(t, testprotocol.UpdateNativeOrcaState(fixture.nativeStateDir, func(state *testprotocol.NativeOrcaState) {
			switch next.loss {
			case "dispatch":
				state.LoseDispatchAt = next.dispatch
			case "send":
				state.EscalationAt, state.LoseSendOnce = next.dispatch, true
			case "task":
				state.LoseTaskAt = next.dispatch
			}
		}))
		submitIntegrationAnswer(t, ctx, fixture.config, current.run)
		answeredHosts := map[string]bool{}
		current = waitIntegrationRun(t, ctx, secondService, fixture.config, func(run *steerline.Run, execution *steerline.ExecutionRecord) bool {
			if run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.HostTarget != nil && !answeredHosts[run.Decision.ID] {
				answeredHosts[run.Decision.ID] = true
				submitIntegrationAnswerOption(t, ctx, fixture.config, run, "recheck_environment")
			}
			if next.loss == "send" && run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute == nil && run.Decision.Request.HostTarget == nil && execution.Target.Dispatch == "ctx-4" && execution.CurrentTurnID != execution.LastStoppedTurnID && !answeredHosts[run.Decision.ID] {
				answeredHosts[run.Decision.ID] = true
				submitIntegrationAnswer(t, ctx, fixture.config, run)
			}
			return execution.Target.Dispatch == fmt.Sprintf("ctx-%d", next.dispatch) && execution.CurrentTurnID == execution.LastStoppedTurnID &&
				run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.HostTarget == nil && run.Decision.Request.QuestionRoute == nil
		})
		state, err := testprotocol.ReadNativeOrcaState(fixture.nativeStateDir)
		require(t, err)
		if state.Dispatches != next.dispatch || len(state.Tasks) != next.dispatch || state.Replies != 1 || current.run.Process == nil || !sameNativePhysicalProcess(*current.run.Process, original) || current.execution.SessionID != sessionID || derefString(current.run.WorktreePath) != workspace {
			t.Fatalf("native %s recovery repeated a mutation or replaced the original binding: state=%+v execution=%+v", next.loss, state, current.execution)
		}
		if next.loss == "send" && (state.Sends != 1 || state.LoseSendOnce) {
			t.Fatalf("native status feedback lost its original one-write reconciliation: %+v", state)
		}
		assertNoTaskDelivery(t, current.runtime, current.run)
	}
	state, err = testprotocol.ReadNativeOrcaState(fixture.nativeStateDir)
	require(t, err)
	submitIntegrationAnswer(t, ctx, fixture.config, current.run)
	rejected := waitIntegrationRun(t, ctx, secondService, fixture.config, func(run *steerline.Run, execution *steerline.ExecutionRecord) bool {
		state, err := testprotocol.ReadNativeOrcaState(fixture.nativeStateDir)
		require(t, err)
		return state.AgentIncarnation == "replaced-agent" && (execution.Error != nil || execution.Environment != nil && execution.Environment.Blocker != nil)
	})
	preserved := false
	for _, decision := range rejected.runtime.Decisions {
		if decision.DecisionID == current.run.Decision.ID && decision.Status == "answer_saved" {
			preserved = true
		}
	}
	if !preserved {
		t.Fatal("replaced incarnation discarded its original accepted answer")
	}
	state, err = testprotocol.ReadNativeOrcaState(fixture.nativeStateDir)
	require(t, err)
	if state.AgentIncarnation != "replaced-agent" || len(state.Tasks) != 5 || state.Dispatches != 5 || state.Replies != 1 || state.Sends != 1 || rejected.execution.Target.Dispatch != "ctx-5" || rejected.run.Process == nil || !sameNativePhysicalProcess(*rejected.run.Process, original) {
		t.Fatal("replaced native incarnation consumed saved input or changed the original resources")
	}
	// Protocol cleanup after stopping the fixture verifies the exact saved
	// terminals. It does not fabricate a merged PR or a TaskDelivery.
	secondService.stop(t)
	require(t, testprotocol.UpdateNativeOrcaState(fixture.nativeStateDir, func(state *testprotocol.NativeOrcaState) { state.AgentIncarnation = ""; state.LoseCloseOnce = true }))
	client := orcaworker.New(orcaworker.Config{Repository: fixture.repository, CommandTimeout: "10s", Target: rejected.execution.Target,
		Output: func(callContext context.Context, timeout time.Duration, args ...string) ([]byte, error) {
			callContext, cancel := context.WithTimeout(callContext, timeout)
			defer cancel()
			command := exec.CommandContext(callContext, os.Args[0], append([]string{"--fake", "native-orca", fixture.nativeStateDir}, args...)...)
			command.Dir = fixture.repository
			return command.CombinedOutput()
		}})
	if _, err := client.CloseTerminalChecked(ctx, original.Terminal, original.RuntimeID); err == nil {
		t.Fatal("lost native close receipt was guessed as confirmed")
	}
	absent, err := orcaworker.InspectCleanupTerminal(ctx, client.MetaCall, original, fixture.repository, "agent", true)
	require(t, err)
	if !absent {
		t.Fatal("original agent close could not be reconciled read-only")
	}
	var coordinator executionapi.ProcessIdentity
	for _, envelope := range rejected.execution.ImplementationData {
		if envelope.ImplementationKey == orcaworker.ExecutionStateCodecKey {
			saved, err := orcaworker.DecodeExecutionStateEnvelope(envelope, orcaworker.ExecutionStateIdentity{
				ProjectID: fixture.config.Identity(), ExecutionID: rejected.execution.ExecutionID, AttemptID: rejected.execution.CurrentAttemptID,
				Generation: rejected.execution.Generation, ConfigDigest: rejected.execution.ConfigurationDigest, Protocol: executionapi.ProtocolOrcaTerminalV2,
				Process: *rejected.execution.Process, CoordinatorTerminal: rejected.execution.Target.CoordinatorTerminal})
			require(t, err)
			coordinator = saved.Coordinator
		}
	}
	if coordinator.Terminal == "" {
		t.Fatal("cleanup lost its original coordinator binding")
	}
	closed, err := client.CloseTerminalChecked(ctx, coordinator.Terminal, coordinator.RuntimeID)
	require(t, err)
	if closed.Disposition != "confirmed" {
		t.Fatal("original coordinator close was not confirmed")
	}
	state, err = testprotocol.ReadNativeOrcaState(fixture.nativeStateDir)
	require(t, err)
	if !state.ClosedAgent || !state.ClosedCoordinator || len(state.CloseRequests) != 2 || state.CloseRequests[0] != original.Terminal || state.CloseRequests[1] != coordinator.Terminal {
		t.Fatalf("native cleanup repeated close or chose other resources: %+v", state)
	}
	assertNoTaskDelivery(t, rejected.runtime, rejected.run)

}

func sameNativePhysicalProcess(a, b executionapi.ProcessIdentity) bool {
	// Task/Dispatch identify successive turns; all physical resource fields stay fixed.
	a.Task, a.Dispatch, b.Task, b.Dispatch = "", "", "", ""
	return a == b
}

type integrationFixture struct {
	configPath     string
	config         steerline.Config
	repository     string
	bareRemote     string
	remoteURL      string
	commit         string
	nativeStateDir string
}

func newLifecycleProject(t *testing.T, host string) integrationFixture {
	return newLifecycleProjectBase(t, host, true)
}

// Some catalog fixtures bind their final provider before opening authority.
func newLifecycleProjectBase(t *testing.T, host string, openAuthority bool) integrationFixture {
	t.Helper()
	root := t.TempDir()
	t.Setenv("STEERLINE_INTEGRATION_HISTORY_FILE", filepath.Join(root, "app-server-history.json"))
	home := filepath.Join(root, "home")
	t.Setenv("STEERLINE_HOME", home)
	repository, bare := filepath.Join(root, "repo"), filepath.Join(root, "origin.git")
	remoteURL := "git@github.com:owner/repo.git"
	fixtureGitCommand(t, "init", "--quiet", "--bare", bare)
	fixtureGitCommand(t, "init", "--quiet", "-b", "main", repository)
	fixtureGitCommand(t, "-C", repository, "config", "user.name", "Lifecycle Fixture")
	fixtureGitCommand(t, "-C", repository, "config", "user.email", "fixture@example.invalid")
	files := map[string]string{
		"docs/spec/integration.spec.md":        "# Integration Spec\n\n## Acceptance\n\nRun the isolated lifecycle fixture.\n",
		"docs/spec/integration.plan.md":        "# Integration Plan\n\nUse the public service and query ports.\n",
		"docs/spec/native-task-constraints.md": "# Native task constraints\n\nUse isolated temporary state.\n",
	}
	for name, content := range files {
		path := filepath.Join(repository, filepath.FromSlash(name))
		require(t, os.MkdirAll(filepath.Dir(path), 0700))
		require(t, os.WriteFile(path, []byte(content), 0600))
	}
	fixtureGitCommand(t, "-C", repository, "add", ".")
	fixtureGitCommand(t, "-C", repository, "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "integration fixture")
	fixtureGitCommand(t, "-C", repository, "remote", "add", "origin", remoteURL)
	fixtureGitCommand(t, "-C", repository, "push", "--quiet", bare, "HEAD:refs/heads/main")
	commit := fixtureGitOutput(t, "-C", repository, "rev-parse", "HEAD")
	t.Setenv("STEERLINE_TEST_REMOTE_URL", remoteURL)
	t.Setenv("STEERLINE_FAKE_GIT_FETCH_REMOTE", bare)

	policy := executionapi.ExecutionPolicy{Filesystem: executionapi.FilesystemPolicy{ReadScope: "host", WriteScope: "none"}, Network: "deny", Escalation: "deny"}
	publisher := executionapi.PublisherHostService
	if host == "orca" {
		policy.Filesystem.WriteScope, policy.Filesystem.GitMetadataWrite, policy.Network = "workspace", true, "allow"
		publisher = executionapi.PublisherWorker
	}
	codexProfile := codexworker.ProfileConfig{Executable: os.Args[0]}
	if runtime.GOOS == "windows" && (host == "local" || host == "orca") {
		codexProfile.WindowsSandbox = "unelevated"
	}
	codexPayload, err := codexworker.EncodeProfileConfig(codexProfile)
	require(t, err)
	implementationConfigs := []executionapi.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 1, Payload: codexPayload}}
	stateDir := filepath.Join(root, "native-protocol")
	if host == "orca" {
		orcaPayload, encodeErr := orcaworker.EncodeProfileConfig(orcaworker.ProfileConfig{
			Executable: os.Args[0], Args: []string{"--fake", "native-orca", stateDir}, Shell: "powershell.exe", WaitTimeout: "1m",
		})
		require(t, encodeErr)
		implementationConfigs = append(implementationConfigs, executionapi.ImplementationConfigInput{ImplementationKey: orcaworker.ProfileCodecKey, CodecVersion: 1, Payload: orcaPayload})
	}
	globalData, err := json.Marshal(map[string]any{
		"version": 2, "default_execution_profile": "fixture",
		"execution_profiles": []any{map[string]any{"id": "fixture", "name": "Integration fixture", "host": host, "agent": "codex", "implementation_configs": implementationConfigs}},
	})
	require(t, err)
	require(t, os.MkdirAll(filepath.Dir(steerlineapp.GlobalExecutionConfigPath()), 0700))
	require(t, os.WriteFile(steerlineapp.GlobalExecutionConfigPath(), globalData, 0600))
	projectData, err := json.Marshal(map[string]any{
		"repository": repository, "remote": "origin", "branch": "main", "task_authority": "native", "workflow_protocol": "worker-flow-v1",
		"task_poll": "1s", "pr_poll": "1m", "heartbeat": "1s", "command_timeout": "15s", "shutdown_grace": "5s",
		"claimant": "integration", "git": steerline.Command{Executable: os.Args[0], Args: []string{"--fake", "publication-git", "git"}},
		"executable_types": []string{"task"}, "execution_policy": policy,
		"publication_policy": executionapi.PublicationPolicy{Publisher: publisher},
		"pr": map[string]any{"kind": "command", "base_url": "https://github.com", "repository": "owner/repo",
			"query": steerline.Command{Executable: "git", Args: []string{"status"}}, "close": steerline.Command{Executable: "git", Args: []string{"status"}}},
	})
	require(t, err)
	configPath := filepath.Join(root, "project.json")
	require(t, os.WriteFile(configPath, projectData, 0600))
	config, err := steerlineapp.LoadConfig(configPath)
	require(t, err)
	config.Claimant = "integration"
	if openAuthority {
		project, err := steerline.OpenNativeTaskProject(context.Background(), &config)
		require(t, err)
		require(t, project.Close())
	}
	return integrationFixture{config: config, configPath: configPath, repository: repository, bareRemote: bare, remoteURL: remoteURL, commit: commit, nativeStateDir: stateDir}
}

func createIntegrationTask(t *testing.T, ctx context.Context, fixture integrationFixture, id int64) {
	t.Helper()
	contract := steerline.TaskContract{
		ContractID: fmt.Sprintf("lifecycle-%d", id), Repository: fixture.remoteURL, Remote: "origin", Branch: "main",
		SpecRef:    integrationReference(t, fixture.repository, fixture.commit, "docs/spec/integration.spec.md"),
		PlanRef:    integrationReference(t, fixture.repository, fixture.commit, "docs/spec/integration.plan.md"),
		CommonRefs: []steerline.FileReference{integrationReference(t, fixture.repository, fixture.commit, "docs/spec/native-task-constraints.md")},
	}
	contract.AcceptanceRefs = []steerline.FileReference{contract.SpecRef}
	digest, err := steerline.TaskContractDigest(contract)
	require(t, err)
	contract.Digest = digest
	store, err := steerline.OpenNativeTaskCommandStore(ctx, &fixture.config)
	require(t, err)
	commands := steerline.CommandService{Store: store, Project: fixture.config.Identity()}
	queries := steerline.NewQueryService(store, fixture.config.Identity())
	authority, err := queries.ReadyNativeTaskSnapshot(ctx)
	require(t, err)
	batch := steerline.TaskBatchInput{SchemaVersion: "task-batch/v1", ProjectID: fixture.config.Identity(),
		Tasks:        []steerline.TaskBatchTask{{LocalKey: "fixture", TaskID: id, Title: "Lifecycle integration", Type: "task", Contract: contract}},
		Dependencies: []steerline.TaskBatchEdge{},
	}
	body, err := json.Marshal(batch)
	require(t, err)
	_, err = commands.CreateTaskBatch(ctx, fixture.config, body, newID(), authority.AuthorityVersion, "operator")
	require(t, err)
	paused := false
	page, err := queries.ShowNativeTask(ctx, fmt.Sprint(id))
	require(t, err)
	changed, err := commands.SetNativeDispatchState(ctx, steerline.NativeDispatchUpdate{
		RequestID: newID(), ExpectedAuthorityVersion: page.AuthorityVersion, Paused: &paused, RequestedBy: "operator",
	})
	require(t, err)
	page, err = queries.ShowNativeTask(ctx, fmt.Sprint(id))
	require(t, err)
	if len(page.Tasks) != 1 || page.Tasks[0].Status != "open" {
		t.Fatalf("public task query did not show the created ready task: %+v", page.Tasks)
	}
	if changed.AuthorityVersion != page.AuthorityVersion {
		t.Fatalf("task query returned stale authority after public dispatch update: changed=%d page=%d", changed.AuthorityVersion, page.AuthorityVersion)
	}
	require(t, store.Close())
}

func claimIntegrationTask(t *testing.T, ctx context.Context, fixture integrationFixture, taskID int64, serviceInstanceID string) {
	t.Helper()
	store, err := steerline.OpenNativeTaskCommandStore(ctx, &fixture.config)
	require(t, err)
	commands := steerline.CommandService{Store: store, Project: fixture.config.Identity()}
	page, err := steerline.NewQueryService(store, fixture.config.Identity()).ShowNativeTask(ctx, fmt.Sprint(taskID))
	require(t, err)
	profiles := fixture.config.AvailableExecutionProfiles()
	if len(profiles) != 1 || len(page.Tasks) != 1 {
		t.Fatalf("fixture query returned %d tasks and %d execution profiles", len(page.Tasks), len(profiles))
	}
	_, err = commands.ClaimNativeTask(ctx, fixture.config, steerline.TaskClaimOptions{
		TaskID: fmt.Sprint(taskID), RequestID: newID(), ExpectedTaskVersion: page.Tasks[0].Version,
		ExpectedAuthorityVersion: page.AuthorityVersion, RequestedBy: "operator", Explicit: true,
		ExecutionProfile: &profiles[0], ServiceInstanceID: serviceInstanceID,
	})
	require(t, err)
	require(t, store.Close())
}

func integrationReference(t *testing.T, repository, commit, name string) steerline.FileReference {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(name)))
	require(t, err)
	sum := sha256.Sum256(content)
	return steerline.FileReference{Path: name, Commit: commit, SHA256: hex.EncodeToString(sum[:])}
}

func fixtureGitCommand(t *testing.T, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func fixtureGitOutput(t *testing.T, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(output))
}

type integrationLifecycleResolver struct {
	directory           *steerline.ExecutionDirectory
	mu                  sync.Mutex
	calls               map[executionapi.ExecutionProtocol]int
	beforeNativeExecute func(executionapi.LifecycleRequest) error
}

func (r *integrationLifecycleResolver) ResolveLifecycle(setup executionapi.LifecycleSetup) (executionapi.LifecyclePort, error) {
	r.mu.Lock()
	if r.calls == nil {
		r.calls = make(map[executionapi.ExecutionProtocol]int)
	}
	r.calls[setup.Snapshot.Ref.Protocol]++
	r.mu.Unlock()
	port, err := r.directory.ResolveLifecycle(setup)
	if err != nil {
		return nil, err
	}
	switch setup.Snapshot.Ref.Protocol {
	case executionapi.ProtocolLocalCodexAppServer:
		return port, nil
	case executionapi.ProtocolOrcaTerminalV2:
		if r.beforeNativeExecute != nil {
			host, ok := port.(contracts.HostEnvironmentPort)
			if !ok {
				return nil, fmt.Errorf("native fixture adapter lacks its host port")
			}
			mapper, ok := port.(contracts.ConversationInputMapper)
			if !ok {
				return nil, fmt.Errorf("native fixture adapter lacks its conversation mapper")
			}
			return integrationFaultLifecycle{LifecyclePort: port, HostEnvironmentPort: host, ConversationInputMapper: mapper, before: r.beforeNativeExecute}, nil
		}
		return port, nil
	default:
		return nil, executionapi.UnsupportedAdapterOperation("ResolveLifecycle", "integration fixture does not authenticate this protocol")
	}
}

type integrationFaultLifecycle struct {
	executionapi.LifecyclePort
	contracts.HostEnvironmentPort
	contracts.ConversationInputMapper
	before func(executionapi.LifecycleRequest) error
}

func (p integrationFaultLifecycle) Execute(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	if err := p.before(request.Clone()); err != nil {
		return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationRejected, Diagnostic: err.Error()}, nil, err
	}
	return p.LifecyclePort.Execute(ctx, request.Clone())
}

func (r *integrationLifecycleResolver) count(protocol executionapi.ExecutionProtocol) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[protocol]
}

type runningIntegrationService struct {
	service    *steerline.Service
	config     steerline.Config
	reader     persistence.Connection
	queries    steerline.QueryService
	instanceID string
	cancel     context.CancelFunc
	done       chan error
	stopped    bool
	closed     bool
}

func startIntegrationService(t *testing.T, parent context.Context, config steerline.Config, resolver executionapi.LifecycleResolver) *runningIntegrationService {
	return startIntegrationServiceWithSources(t, parent, config, resolver, nil)
}

func startIntegrationServiceWithSources(t *testing.T, parent context.Context, config steerline.Config, resolver executionapi.LifecycleResolver, sources contracts.ConversationSourcePort) *runningIntegrationService {
	t.Helper()
	if decorated, ok := resolver.(*integrationLifecycleResolver); ok {
		decorated.directory = config.ExecutionDirectory
	}
	var service *steerline.Service
	var err error
	if sources == nil {
		service, err = steerlineapp.NewServiceWithLifecycleResolver(config, resolver)
	} else {
		service, err = steerlineapp.NewServiceWithPorts(config, resolver, sources)
	}
	require(t, err)
	reader, err := steerline.OpenNativeTaskReader(parent, &config)
	require(t, err)
	ctx, cancel := context.WithCancel(parent)
	harness := &runningIntegrationService{service: service, config: config, reader: reader,
		queries: steerline.NewQueryService(reader, config.Identity()), cancel: cancel, done: make(chan error, 1)}
	t.Cleanup(func() { harness.stop(t) })
	go func() { harness.done <- service.RunWithDispatch(ctx, "manual") }()
	waitFor(t, 30*time.Second, func() bool {
		select {
		case err := <-harness.done:
			harness.stopped = true
			t.Fatalf("public service exited before readiness: %v", err)
		default:
		}
		view, err := harness.queries.ReadProjectView(parent, "901", 100)
		if err != nil {
			view, err = harness.queries.ReadProjectView(parent, "902", 100)
		}
		if err == nil && view.Runtime != nil && view.Runtime.Service.Ready {
			harness.instanceID = view.Runtime.Service.InstanceID
			return true
		}
		return false
	})
	return harness
}

func (h *runningIntegrationService) stop(t *testing.T) {
	t.Helper()
	if h == nil || h.closed {
		return
	}
	h.closed = true
	h.cancel()
	if !h.stopped {
		select {
		case err := <-h.done:
			h.stopped = true
			if err != nil {
				t.Errorf("public service shutdown: %v", err)
			}
		case <-time.After(20 * time.Second):
			t.Error("public service did not stop")
		}
	}
	if err := h.service.Close(); err != nil {
		t.Errorf("close service database: %v", err)
	}
	if h.reader != nil {
		if err := h.reader.Close(); err != nil {
			t.Errorf("close query reader: %v", err)
		}
	}
}

type integrationRunSnapshot struct {
	runtime   *steerline.Runtime
	run       *steerline.Run
	execution *steerline.ExecutionRecord
}

func waitIntegrationRun(t *testing.T, ctx context.Context, service *runningIntegrationService, config steerline.Config,
	predicate func(*steerline.Run, *steerline.ExecutionRecord) bool) integrationRunSnapshot {
	t.Helper()
	var result integrationRunSnapshot
	t.Cleanup(func() {
		if t.Failed() && result.runtime != nil {
			t.Logf("last public lifecycle: decision=%s current=%s stopped=%s turns=%d pending=%s command=%s pendingCommand=%s status=%s error=%s", mustJSON(result.run.Decision), result.execution.CurrentTurnID, result.execution.LastStoppedTurnID, result.execution.TurnNumber, result.execution.PendingTurnID, result.execution.CurrentCommandID, result.execution.PendingCommandID, result.execution.Status, derefString(result.execution.Error))
			t.Logf("original preparation evidence: %s; last event: %s", mustJSON(result.execution.Environment), derefString(result.execution.LastEvent))
		}
	})
	waitFor(t, 2*time.Minute, func() bool {
		select {
		case err := <-service.done:
			service.stopped = true
			diagnosticContext, diagnosticCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer diagnosticCancel()
			view, queryErr := service.queries.ReadProjectView(diagnosticContext, "901", 100)
			if queryErr != nil {
				view, queryErr = service.queries.ReadProjectView(diagnosticContext, "902", 100)
			}
			var runtimeState *steerline.Runtime
			if view.Runtime != nil {
				runtimeState = view.Runtime
			}
			t.Fatalf("public service exited during lifecycle: run=%v query=%v service=%+v runs=%+v executions=%+v operations=%+v",
				err, queryErr, runtimeStateService(runtimeState), runtimeStateRuns(runtimeState), runtimeStateExecutions(runtimeState), runtimeStateOperations(runtimeState))
		default:
		}
		view, err := service.queries.ReadProjectView(ctx, "901", 100)
		if err != nil {
			view, err = service.queries.ReadProjectView(ctx, "902", 100)
		}
		if err != nil || view.Runtime == nil {
			return false
		}
		runtimeState := view.Runtime
		for _, run := range runtimeState.Runs {
			if run == nil || run.TaskID != "901" && run.TaskID != "902" {
				continue
			}
			var execution *steerline.ExecutionRecord
			for i := range runtimeState.Executions {
				if runtimeState.Executions[i].ExecutionID == run.ExecutionID {
					execution = &runtimeState.Executions[i]
					break
				}
			}
			if execution != nil {
				result = integrationRunSnapshot{runtime: runtimeState, run: run, execution: execution}
			}
			if execution != nil && predicate(run, execution) {
				result = integrationRunSnapshot{runtime: runtimeState, run: run, execution: execution}
				return true
			}
		}
		return false
	})
	return result
}

func runtimeStateService(state *steerline.Runtime) any {
	if state == nil {
		return nil
	}
	return state.Service
}
func runtimeStateRuns(state *steerline.Runtime) any {
	if state == nil {
		return nil
	}
	values := make([]steerline.Run, 0, len(state.Runs))
	for _, run := range state.Runs {
		if run != nil {
			values = append(values, *run)
		}
	}
	return values
}
func runtimeStateExecutions(state *steerline.Runtime) any {
	if state == nil {
		return nil
	}
	return state.Executions
}
func runtimeStateOperations(state *steerline.Runtime) any {
	if state == nil {
		return nil
	}
	return state.Operations
}

func waitFor(t *testing.T, timeout time.Duration, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the public lifecycle/query state")
}

func submitIntegrationAnswer(t *testing.T, ctx context.Context, config steerline.Config, run *steerline.Run) {
	t.Helper()
	if run == nil || run.Decision == nil || run.Decision.Status != "pending" {
		t.Fatal("public answer submission lacks the displayed pending decision")
	}
	option := "continue"
	if run.Decision.Request.QuestionRoute != nil {
		option = ""
	}
	submitIntegrationAnswerOption(t, ctx, config, run, option)
}

func submitIntegrationAnswerOption(t *testing.T, ctx context.Context, config steerline.Config, run *steerline.Run, option string) {
	t.Helper()
	answer := steerline.AnswerOptions{
		RunID: run.RunID, DecisionID: run.Decision.ID, Option: option, Note: "Continue the original work.",
		By: "operator", ExpectedVersion: run.Decision.Version,
	}
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for {
		_, _, err := steerline.SubmitDecisionAnswer(ctx, config, answer)
		if err == nil {
			return
		}
		if strings.Contains(err.Error(), "stale_revision:") {
			// Refresh this displayed question after a known rejection. A renewed
			// generic/host choice must retain the exact request and target; a
			// different native question or consumed answer is never retried.
			reader, readErr := steerline.OpenNativeTaskReader(ctx, &config)
			require(t, readErr)
			view, readErr := steerline.NewQueryService(reader, config.Identity()).ReadProjectView(ctx, run.TaskID, 100)
			closeErr := reader.Close()
			require(t, readErr)
			require(t, closeErr)
			found := false
			if view.Runtime != nil {
				for _, current := range view.Runtime.Runs {
					if current != nil && current.RunID == run.RunID && current.ExecutionID == run.ExecutionID && current.AttemptID == run.AttemptID && current.Decision != nil && current.Decision.Status == "pending" &&
						(current.Decision.ID == run.Decision.ID || current.Decision.Request.QuestionRoute == nil && run.Decision.Request.QuestionRoute == nil && derefString(current.LogPath) == derefString(run.LogPath) && string(mustJSON(current.Decision.Request)) == string(mustJSON(run.Decision.Request))) {
						answer.DecisionID, answer.ExpectedVersion = current.Decision.ID, current.Decision.Version
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("stale displayed decision changed its original question/target: %v", err)
			}
		} else if !strings.Contains(err.Error(), "stale_instance: SQLite projection does not match the ready service instance") {
			require(t, err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("public answer command kept seeing a stale service projection: %v", ctx.Err())
		case <-deadline.C:
			t.Fatalf("public answer command did not observe a ready service projection: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func assertQueriedTaskPhase(t *testing.T, ctx context.Context, config steerline.Config, taskID, phase string) {
	t.Helper()
	reader, err := steerline.OpenNativeTaskReader(ctx, &config)
	require(t, err)
	defer reader.Close()
	page, err := steerline.NewQueryService(reader, config.Identity()).ShowNativeTask(ctx, taskID)
	require(t, err)
	if len(page.Tasks) != 1 || len(page.Tasks[0].WorkflowRuns) != 1 || page.Tasks[0].WorkflowRuns[0].Phase != phase {
		t.Fatalf("public task query phase=%+v, want %s", page.Tasks, phase)
	}
}

func lifecycleOperations(runtimeState *steerline.Runtime, kind string) []steerline.OperationRecord {
	var result []steerline.OperationRecord
	for _, operation := range runtimeState.Operations {
		if operation.Kind == kind {
			result = append(result, operation)
		}
	}
	return result
}

func hasOperationStatus(operations []steerline.OperationRecord, kind, status string) bool {
	for _, operation := range operations {
		if operation.Kind == kind && operation.Status == status {
			return true
		}
	}
	return false
}

func hasAcknowledgement(runtimeState *steerline.Runtime, deliveryID, status string) bool {
	for _, operation := range lifecycleOperations(runtimeState, "acknowledge_delivery") {
		if operation.Status != status {
			continue
		}
		var input executionapi.AcknowledgeInput
		if json.Unmarshal(operation.Request, &input) == nil && input.DeliveryID == deliveryID {
			return true
		}
	}
	return false
}

func decisionWasAccepted(runtimeState *steerline.Runtime, decisionID string) bool {
	for _, decision := range runtimeState.Decisions {
		if decision.DecisionID == decisionID {
			return decision.Status == "reply_accepted"
		}
	}
	return false
}

func assertNoTaskDelivery(t *testing.T, runtimeState *steerline.Runtime, run *steerline.Run) {
	t.Helper()
	if runtimeState == nil || run == nil || run.TaskStatus != "in_progress" {
		t.Fatalf("unfinished worker flow changed task delivery state: runtime=%+v run=%+v", runtimeState, run)
	}
	for _, delivery := range runtimeState.Deliveries {
		if delivery.TaskID == run.DomainTaskID {
			t.Fatalf("lifecycle-only integration unexpectedly wrote TaskDelivery: %+v", delivery)
		}
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
