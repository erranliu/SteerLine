package integration

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

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testkit/protocol"
)

// This suite starts the production service with its formal resolver injection,
// claims an isolated native task through CommandService, and observes it through
// the public runtime/task query APIs. The Codex and Orca executables are protocol
// fixtures implemented by this test binary; no product CLI or user project is used.
func TestCD64OriginalSessionReconciliation(t *testing.T) {
	t.Run("remote_admission_stays_blocked", TestRemotePublicationClaimRejectsUnreachableOrca)
	t.Run("app_server_two_turns", testIntegratedAppServerTwoTurns)
	t.Run("native_original_session_manual_gate", testIntegratedNativeOriginalSessionGate)
	t.Run("remote_protocol_original_assertions", testCD64RemoteProtocolAssertions)
}

func testIntegratedAppServerTwoTurns(t *testing.T) {
	// Include project setup, first-turn persistence and a second original turn
	// in the shared fixture budget under Full's parallel process load.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	fixture := newLifecycleProject(t, "local")
	createIntegrationTask(t, ctx, fixture, 901)
	resolver := &integrationLifecycleResolver{}
	service := startIntegrationService(t, ctx, fixture.config, resolver)
	defer service.stop(t)
	claimIntegrationTask(t, ctx, fixture, 901, service.instanceID)

	first := waitIntegrationRun(t, ctx, service, fixture.config, func(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord) bool {
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

	second := waitIntegrationRun(t, ctx, service, fixture.config, func(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord) bool {
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
	if resolver.count(executionbackend.ProtocolLocalCodexAppServer) == 0 {
		t.Fatal("public service flow never resolved the app-server lifecycle adapter")
	}
}

func testIntegratedNativeOriginalSessionGate(t *testing.T) { testIntegratedNativeSessionGate(t, false) }

func testIntegratedNativeSessionGate(t *testing.T, changeConfig bool) {
	testIntegratedNativeSessionGateWithMutations(t, changeConfig, true)
}

// The narrow product regression stops after reconnecting the original terminal
// and answering its original question. Existing mutation-loss tests keep their
// full scope and unchanged deadlines.
func TestOriginalTerminalReconnectAndFeedback(t *testing.T) {
	testIntegratedNativeSessionGateWithMutations(t, true, false)
}

func testIntegratedNativeSessionGateWithMutations(t *testing.T, changeConfig, mutations bool) {
	if runtime.GOOS != "windows" {
		t.Skip("native Orca protocol fixture uses the Windows host contract")
	}
	// Five native Dispatches, service restart and mutation-loss reconciliation
	// precede the final incarnation rejection; bound the entire fixture flow.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	fixture := newLifecycleProject(t, "orca")
	initial := testprotocol.NativeOrcaState{FirstBusinessDispatch: 1, LoseQuestionAckOnce: true, LoseReplyOnce: true}
	require(t, testprotocol.WriteNativeOrcaState(fixture.nativeStateDir, initial))
	createIntegrationTask(t, ctx, fixture, 902)
	resolver := &integrationLifecycleResolver{beforeNativeExecute: func(request executionbackend.LifecycleRequest) error {
		if request.Operation.Action == string(executionbackend.LifecycleCreateResource) && request.Input.Resource != nil && request.Input.Resource.Purpose == "conversation" {
			return testprotocol.UpdateNativeOrcaState(fixture.nativeStateDir, func(state *testprotocol.NativeOrcaState) {
				if len(state.Tasks) == 4 {
					state.AgentIncarnation = "replaced-agent"
				}
			})
		}
		return nil
	}}
	firstService := startIntegrationService(t, ctx, fixture.config, resolver)
	claimIntegrationTask(t, ctx, fixture, 902, firstService.instanceID)
	first := waitIntegrationRun(t, ctx, firstService, fixture.config, func(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord) bool {
		return run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute != nil &&
			execution.CurrentTurnID != "" && execution.Target.Dispatch == "ctx-1" && execution.TurnNumber == 1
	})
	if first.run.Process == nil {
		t.Fatal("native receipt omitted its process identity")
	}
	original := *first.run.Process
	sessionID, sessionGeneration := first.execution.SessionID, first.execution.SessionGeneration
	workspace, runID, dispatchID := derefString(first.run.WorktreePath), first.run.Target.OrchestrationRun, first.run.Target.Dispatch
	questionID := first.run.Decision.Request.QuestionRoute.QuestionID
	questionDecisionID := first.run.Decision.ID
	if original.Kind != "orca" || original.Terminal == "" || original.Incarnation == "" || sessionID == "" || sessionGeneration != original.Incarnation || runID != "run-fake" || dispatchID != "ctx-1" {
		t.Fatalf("native receipt omitted the original Run/Task/Dispatch/session identity: process=%+v session=%q/%q target=%+v", original, sessionID, sessionGeneration, first.run.Target)
	}
	acknowledged := waitIntegrationRun(t, ctx, firstService, fixture.config, func(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord) bool {
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
	recovered := waitIntegrationRun(t, ctx, secondService, fixture.config, func(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord) bool {
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
	if state, err := testprotocol.ReadNativeOrcaState(fixture.nativeStateDir); err != nil || state.Dispatches != 1 || state.Replies != 0 || state.Sends != 0 {
		t.Fatalf("restart crossed the manual input gate: state=%+v err=%v", state, err)
	}
	assertQueriedTaskPhase(t, ctx, fixture.config, "902", "waiting_decision")
	submitIntegrationAnswer(t, ctx, fixture.config, recovered.run)

	settled := waitIntegrationRun(t, ctx, secondService, fixture.config, func(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord) bool {
		return run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute == nil &&
			execution.CurrentTurnID != "" && execution.CurrentTurnID == execution.LastStoppedTurnID && execution.Target.Dispatch == dispatchID
	})
	if settled.run.Process == nil || settled.run.Process.Terminal != original.Terminal || settled.run.Process.Incarnation != original.Incarnation ||
		settled.execution.SessionID != sessionID || derefString(settled.run.WorktreePath) != workspace || settled.run.Target.OrchestrationRun != runID || settled.run.Target.Dispatch != dispatchID {
		t.Fatalf("original native question did not settle on its original resources: run=%+v execution=%+v", settled.run, settled.execution)
	}
	state, err := testprotocol.ReadNativeOrcaState(fixture.nativeStateDir)
	require(t, err)
	if state.Dispatches != 1 || state.Replies != 1 || state.Sends != 0 || !state.Acked["delivery-ask-1"] || !state.QuestionAckLossArmed || state.LoseAckOnce || state.LoseReplyOnce {
		t.Fatalf("native dispatch/reply/ack was repeated or changed route: %+v", state)
	}
	if resolver.count(executionbackend.ProtocolOrcaTerminalV2) == 0 {
		t.Fatal("public service flow never resolved the native Orca lifecycle adapter")
	}
	if !decisionWasAccepted(settled.runtime, questionDecisionID) || !hasOperationStatus(lifecycleOperations(settled.runtime, "reply_execution_question"), "reply_execution_question", "confirmed") {
		t.Fatalf("original native question answer lacks its confirmed receipt: decision=%+v operations=%+v", settled.runtime.Decisions, lifecycleOperations(settled.runtime, "reply_execution_question"))
	}
	assertNoTaskDelivery(t, settled.runtime, settled.run)
	assertQueriedTaskPhase(t, ctx, fixture.config, "902", "waiting_decision")
	if !mutations {
		secondService.stop(t)
		return
	}
	// Preserve the original native producer regressions, using public answers
	// rather than the removed private actor methods.
	current := settled
	for _, next := range []struct {
		dispatch int
		loss     string
	}{{2, "dispatch"}, {3, "send"}, {4, "task"}} {
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
		current = waitIntegrationRun(t, ctx, secondService, fixture.config, func(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord) bool {
			if run.EnvironmentBlocker != nil && !answeredHosts[run.EnvironmentBlocker.ID] {
				answeredHosts[run.EnvironmentBlocker.ID] = true
				submitIntegrationAnswerOption(t, ctx, fixture.config, run, "recheck_environment")
			}
			if next.loss == "send" && run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute == nil && run.EnvironmentBlocker == nil && execution.Target.Dispatch == "ctx-3" && execution.CurrentTurnID != execution.LastStoppedTurnID && !answeredHosts[run.Decision.ID] {
				answeredHosts[run.Decision.ID] = true
				submitIntegrationAnswer(t, ctx, fixture.config, run)
			}
			return execution.Target.Dispatch == fmt.Sprintf("ctx-%d", next.dispatch) && execution.CurrentTurnID == execution.LastStoppedTurnID &&
				run.Decision != nil && run.Decision.Status == "pending" && run.EnvironmentBlocker == nil && run.Decision.Request.QuestionRoute == nil
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
	rejected := waitIntegrationRun(t, ctx, secondService, fixture.config, func(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord) bool {
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
	if state.AgentIncarnation != "replaced-agent" || len(state.Tasks) != 4 || state.Dispatches != 4 || state.Replies != 1 || state.Sends != 1 || rejected.execution.Target.Dispatch != "ctx-4" || rejected.run.Process == nil || !sameNativePhysicalProcess(*rejected.run.Process, original) {
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
	var coordinator executionbackend.ProcessIdentity
	for _, envelope := range rejected.execution.ImplementationData {
		if envelope.ImplementationKey == orcaworker.ExecutionStateCodecKey {
			saved, err := orcaworker.DecodeExecutionStateEnvelope(envelope, orcaworker.ExecutionStateIdentity{
				ProjectID: fixture.config.Identity(), ExecutionID: rejected.execution.ExecutionID, AttemptID: rejected.execution.CurrentAttemptID,
				Generation: rejected.execution.Generation, ConfigDigest: rejected.execution.ConfigurationDigest, Protocol: executionbackend.ProtocolOrcaTerminalV2,
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

func sameNativePhysicalProcess(a, b executionbackend.ProcessIdentity) bool {
	// Task/Dispatch identify successive turns; all physical resource fields stay fixed.
	a.Task, a.Dispatch, b.Task, b.Dispatch = "", "", "", ""
	return a == b
}

type integrationFixture struct {
	configPath     string
	config         steerlineapp.Config
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

	publisher := executionbackend.PublisherWorker
	if host == "orca" {
		publisher = executionbackend.PublisherWorker
	}
	codexProfile := codexworker.ProfileConfig{Executable: os.Args[0]}
	if runtime.GOOS == "windows" && (host == "local" || host == "orca") {
	}
	codexPayload, err := codexworker.EncodeProfileConfig(codexProfile)
	require(t, err)
	implementationConfigs := []executionbackend.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 2, Payload: codexPayload}}
	if host == "orca" {
		implementationConfigs = nil
	}
	stateDir := filepath.Join(root, "native-protocol")
	if host == "orca" {
		// This positive fixture checks original binding, not deadline expiry;
		// its real helper/tool processes also run under Full's parallel load.
		orcaPayload, encodeErr := orcaworker.EncodeProfileConfig(orcaworker.ProfileConfig{
			Executable: os.Args[0], Args: []string{"--fake", "native-orca", stateDir}, Shell: "powershell.exe", WaitTimeout: "2m",
		})
		require(t, encodeErr)
		implementationConfigs = append(implementationConfigs, executionbackend.ImplementationConfigInput{ImplementationKey: orcaworker.ProfileCodecKey, CodecVersion: 1, Payload: orcaPayload})
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
		"task_poll": "1s", "pr_poll": "1m", "heartbeat": "1s", "command_timeout": "30s", "shutdown_grace": "5s",
		"claimant": "integration", "git": executionbackend.Command{Executable: os.Args[0], Args: []string{"--fake", "publication-git", "git"}},
		"executable_types":   []string{"task"},
		"publication_policy": executionbackend.PublicationPolicy{Publisher: publisher},
		"pr": map[string]any{"kind": "command", "base_url": "https://github.com", "repository": "owner/repo",
			"query": executionbackend.Command{Executable: "git", Args: []string{"status"}}},
	})
	require(t, err)
	configPath := filepath.Join(root, "project.json")
	require(t, os.WriteFile(configPath, projectData, 0600))
	config, err := steerlineapp.LoadConfig(configPath)
	require(t, err)
	if openAuthority {
		project, err := steerlineapp.OpenNativeTaskProject(context.Background(), &config)
		require(t, err)
		require(t, project.Close())
	}
	return integrationFixture{config: config, configPath: configPath, repository: repository, bareRemote: bare, remoteURL: remoteURL, commit: commit, nativeStateDir: stateDir}
}

func createIntegrationTask(t *testing.T, ctx context.Context, fixture integrationFixture, id int64) {
	t.Helper()
	contract := taskmodel.TaskContract{
		ContractID: fmt.Sprintf("lifecycle-%d", id), Repository: fixture.remoteURL, Remote: "origin", Branch: "main",
		SpecRef:    integrationReference(t, fixture.repository, fixture.commit, "docs/spec/integration.spec.md"),
		PlanRef:    integrationReference(t, fixture.repository, fixture.commit, "docs/spec/integration.plan.md"),
		CommonRefs: []taskmodel.FileReference{integrationReference(t, fixture.repository, fixture.commit, "docs/spec/native-task-constraints.md")},
	}
	contract.AcceptanceRefs = []taskmodel.FileReference{contract.SpecRef}
	digest, err := taskmodel.ContractDigest(contract)
	require(t, err)
	contract.Digest = digest
	store, err := steerlineapp.OpenNativeTaskCommandStore(ctx, &fixture.config)
	require(t, err)
	commands := appExecutionCommands(t, store, fixture.config)
	queries := productquery.NewService(store, fixture.config.Identity())
	authority, err := queries.ReadyNativeTaskSnapshot(ctx)
	require(t, err)
	batch := taskmodel.TaskBatchInput{SchemaVersion: "task-batch/v1", ProjectID: fixture.config.Identity(),
		Tasks:        []taskmodel.TaskBatchTask{{LocalKey: "fixture", TaskID: id, Title: "Lifecycle integration", Type: "task", Contract: contract}},
		Dependencies: []taskmodel.TaskBatchEdge{},
	}
	body, err := json.Marshal(batch)
	require(t, err)
	_, err = createTaskBatchFixture(ctx, sqliteCommandStore(commands.Store), fixture.config, body, newID(), authority.AuthorityVersion, "operator")
	require(t, err)
	paused := false
	page, err := queries.ShowNativeTask(ctx, fmt.Sprint(id))
	require(t, err)
	changed, err := commands.SetNativeDispatchState(ctx, execution.NativeDispatchUpdate{
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
	store, err := steerlineapp.OpenNativeTaskCommandStore(ctx, &fixture.config)
	require(t, err)
	commands := appExecutionCommands(t, store, fixture.config)
	page, err := productquery.NewService(store, fixture.config.Identity()).ShowNativeTask(ctx, fmt.Sprint(taskID))
	require(t, err)
	profiles := fixture.config.AvailableExecutionProfiles()
	if len(profiles) != 1 || len(page.Tasks) != 1 {
		t.Fatalf("fixture query returned %d tasks and %d execution profiles", len(page.Tasks), len(profiles))
	}
	_, err = commands.ClaimNativeTask(ctx, fixture.config.ExecutionConfig(), execution.TaskClaimOptions{
		TaskID: fmt.Sprint(taskID), RequestID: newID(), ExpectedTaskVersion: page.Tasks[0].Version,
		ExpectedAuthorityVersion: page.AuthorityVersion, RequestedBy: "operator", Explicit: true,
		ExecutionProfile: func() *executionmodel.FixedExecutionProfile {
			fixed := profiles[0].Freeze(fixture.config.ExecutionDirectory, fixture.config.Identity())
			return &fixed
		}(), ServiceInstanceID: serviceInstanceID,
	})
	require(t, err)
	require(t, store.Close())
}

func appExecutionCommands(t *testing.T, store *sqlite.SQLiteStore, config steerlineapp.Config) execution.CommandService {
	t.Helper()
	commands, err := steerlineapp.NewCommandService(store, config)
	require(t, err)
	return commands
}

func integrationReference(t *testing.T, repository, commit, name string) taskmodel.FileReference {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(name)))
	require(t, err)
	sum := sha256.Sum256(content)
	return taskmodel.FileReference{Path: name, Commit: commit, SHA256: hex.EncodeToString(sum[:])}
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
	directory           *execution.ExecutionDirectory
	mu                  sync.Mutex
	calls               map[executionbackend.ExecutionProtocol]int
	beforeNativeExecute func(executionbackend.LifecycleRequest) error
}

func (r *integrationLifecycleResolver) ResolveLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	r.mu.Lock()
	if r.calls == nil {
		r.calls = make(map[executionbackend.ExecutionProtocol]int)
	}
	r.calls[setup.Snapshot.Ref.Protocol]++
	r.mu.Unlock()
	port, err := r.directory.ResolveLifecycle(setup)
	if err != nil {
		return nil, err
	}
	switch setup.Snapshot.Ref.Protocol {
	case executionbackend.ProtocolLocalCodexAppServer:
		return port, nil
	case executionbackend.ProtocolOrcaTerminalV2:
		if r.beforeNativeExecute != nil {
			return integrationFaultLifecycle{LifecyclePort: port, before: r.beforeNativeExecute}, nil
		}
		return port, nil
	default:
		return nil, executionbackend.UnsupportedAdapterOperation("ResolveLifecycle", "integration fixture does not authenticate this protocol")
	}
}

type integrationFaultLifecycle struct {
	executionbackend.LifecyclePort
	before func(executionbackend.LifecycleRequest) error
}

func (p integrationFaultLifecycle) Execute(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if err := p.before(request.Clone()); err != nil {
		return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationRejected, Diagnostic: err.Error()}, nil, err
	}
	return p.LifecyclePort.Execute(ctx, request.Clone())
}

func (r *integrationLifecycleResolver) count(protocol executionbackend.ExecutionProtocol) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[protocol]
}

type runningIntegrationService struct {
	service    *execution.Service
	config     steerlineapp.Config
	reader     *sqlite.SQLiteStore
	queries    productquery.Service
	instanceID string
	cancel     context.CancelFunc
	done       chan error
	stopped    bool
	closed     bool
}

func startIntegrationService(t *testing.T, parent context.Context, config steerlineapp.Config, resolver executionbackend.LifecycleResolver) *runningIntegrationService {
	return startIntegrationServiceWithSources(t, parent, config, resolver, nil)
}

func startIntegrationServiceWithSources(t *testing.T, parent context.Context, config steerlineapp.Config, resolver executionbackend.LifecycleResolver, sources conversationtypes.ConversationSourcePort) *runningIntegrationService {
	t.Helper()
	if decorated, ok := resolver.(*integrationLifecycleResolver); ok {
		decorated.directory = config.ExecutionDirectory
	}
	var service *execution.Service
	var err error
	if sources == nil {
		service, err = steerlineapp.NewServiceWithLifecycleResolver(config, resolver)
	} else {
		service, err = steerlineapp.NewServiceWithPorts(config, resolver, sources)
	}
	require(t, err)
	reader, err := steerlineapp.OpenNativeTaskReader(parent, &config)
	require(t, err)
	hostConfig := integrationHostConfig(t, config)
	ctx, cancel := context.WithCancel(parent)
	harness := &runningIntegrationService{service: service, config: config, reader: reader,
		queries: productquery.NewService(reader, config.Identity()), cancel: cancel, done: make(chan error, 1)}
	t.Cleanup(func() { harness.stop(t) })
	go func() { harness.done <- steerline.RunService(ctx, service, hostConfig, "manual") }()
	waitFor(t, 30*time.Second, func() bool {
		select {
		case err := <-harness.done:
			harness.stopped = true
			t.Fatalf("public service exited before readiness: %v", err)
		default:
		}
		runtimeState, err := steerlineapp.LoadDatabaseRuntime(config)
		if err == nil && runtimeState != nil && runtimeState.Service.Ready {
			harness.instanceID = runtimeState.Service.InstanceID
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
	runtime   *reconstructiontypes.Runtime
	run       *reconstructiontypes.RunView
	execution *executionmodel.ExecutionRecord
}

func waitIntegrationRun(t *testing.T, ctx context.Context, service *runningIntegrationService, config steerlineapp.Config,
	predicate func(*reconstructiontypes.RunView, *executionmodel.ExecutionRecord) bool) integrationRunSnapshot {
	t.Helper()
	var result integrationRunSnapshot
	t.Cleanup(func() {
		if t.Failed() && result.runtime != nil {
			t.Logf("last public lifecycle: decision=%s current=%s stopped=%s turns=%d pending=%s command=%s pendingCommand=%s status=%s error=%s", mustJSON(result.run.Decision), result.execution.CurrentTurnID, result.execution.LastStoppedTurnID, result.execution.TurnNumber, result.execution.PendingTurnID, result.execution.CurrentCommandID, result.execution.PendingCommandID, result.execution.Status, derefString(result.execution.Error))
			t.Logf("original preparation evidence: %s; last event: %s", mustJSON(result.execution.Environment), derefString(result.execution.LastEvent))
			t.Logf("service: %+v; operation count: %d", result.runtime.Service, len(result.runtime.Operations))
		}
	})
	waitFor(t, 2*time.Minute, func() bool {
		select {
		case err := <-service.done:
			service.stopped = true
			runtimeState, queryErr := steerlineapp.LoadDatabaseRuntime(config)
			t.Fatalf("public service exited during lifecycle: run=%v query=%v service=%+v runs=%+v executions=%+v operations=%+v",
				err, queryErr, runtimeStateService(runtimeState), runtimeStateRuns(runtimeState), runtimeStateExecutions(runtimeState), runtimeStateOperations(runtimeState))
		default:
		}
		runtimeState, err := steerlineapp.LoadDatabaseRuntime(config)
		if err != nil || runtimeState == nil {
			return false
		}
		for _, run := range runtimeState.Runs {
			if run == nil || run.TaskID != "901" && run.TaskID != "902" {
				continue
			}
			var execution *executionmodel.ExecutionRecord
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

func runtimeStateService(state *reconstructiontypes.Runtime) any {
	if state == nil {
		return nil
	}
	return state.Service
}

func runtimeStateRuns(state *reconstructiontypes.Runtime) any {
	if state == nil {
		return nil
	}
	values := make([]reconstructiontypes.RunView, 0, len(state.Runs))
	for _, run := range state.Runs {
		if run != nil {
			values = append(values, *run)
		}
	}
	return values
}

func runtimeStateExecutions(state *reconstructiontypes.Runtime) any {
	if state == nil {
		return nil
	}
	return state.Executions
}

func runtimeStateOperations(state *reconstructiontypes.Runtime) any {
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

func submitIntegrationAnswer(t *testing.T, ctx context.Context, config steerlineapp.Config, run *reconstructiontypes.RunView) {
	t.Helper()
	if run == nil || run.Decision == nil || run.Decision.Status != "pending" {
		t.Fatal("public answer submission lacks the displayed pending decision")
	}
	option := "continue"
	if run.Decision.Request.QuestionRoute != nil || len(run.Decision.Request.Options) == 0 {
		option = ""
	}
	submitIntegrationAnswerOption(t, ctx, config, run, option)
}

func submitIntegrationAnswerOption(t *testing.T, ctx context.Context, config steerlineapp.Config, run *reconstructiontypes.RunView, option string) {
	t.Helper()
	if option == "open_terminal" || option == "recheck_environment" {
		candidate := productquery.Candidate(&productquery.CandidateView{RunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, EnvironmentBlocker: run.EnvironmentBlocker})
		target, err := productapi.EnvironmentTarget(candidate)
		require(t, err)
		hostConfig := integrationHostConfig(t, config)
		request, err := steerline.SubmitControlTargeted(hostConfig, option, productapi.ControlPayload{Environment: target}, "fixture", newID(), "", nil)
		require(t, err)
		receipt, err := steerline.WaitControlReceipt(ctx, hostConfig, request.RequestID, 10*time.Millisecond)
		require(t, err)
		if receipt.Status != "applied" && receipt.Status != "failed" {
			t.Fatalf("environment command: %+v", receipt)
		}
		return
	}
	answer := steerlineapp.AnswerOptions{
		RunID: run.RunID, DecisionID: run.Decision.ID, Option: option, Note: "Continue the original work.",
		By: "operator", ExpectedVersion: run.Decision.Version,
	}
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for {
		_, _, err := steerlineapp.SubmitDecisionAnswer(ctx, config, answer)
		if err == nil {
			return
		}
		if strings.Contains(err.Error(), "stale_revision:") {
			// Refresh this displayed question after a known rejection. A renewed
			// generic/host choice must retain the exact request and target; a
			// different native question or consumed answer is never retried.
			reader, readErr := steerlineapp.OpenNativeTaskReader(ctx, &config)
			require(t, readErr)
			runtimeState, readErr := steerlineapp.LoadDatabaseRuntime(config)
			closeErr := reader.Close()
			require(t, readErr)
			require(t, closeErr)
			found := false
			for _, current := range runtimeState.Runs {
				if current != nil && current.RunID == run.RunID && current.ExecutionID == run.ExecutionID && current.AttemptID == run.AttemptID && current.Decision != nil && current.Decision.Status == "pending" &&
					(current.Decision.ID == run.Decision.ID || current.Decision.Request.QuestionRoute == nil && run.Decision.Request.QuestionRoute == nil && derefString(current.LogPath) == derefString(run.LogPath) && string(mustJSON(current.Decision.Request)) == string(mustJSON(run.Decision.Request))) {
					answer.DecisionID, answer.ExpectedVersion = current.Decision.ID, current.Decision.Version
					found = true
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

func assertQueriedTaskPhase(t *testing.T, ctx context.Context, config steerlineapp.Config, taskID, phase string) {
	t.Helper()
	reader, err := steerlineapp.OpenNativeTaskReader(ctx, &config)
	require(t, err)
	defer reader.Close()
	page, err := productquery.NewService(reader, config.Identity()).ShowNativeTask(ctx, taskID)
	require(t, err)
	if len(page.Tasks) != 1 || len(page.Tasks[0].WorkflowRuns) != 1 || page.Tasks[0].WorkflowRuns[0].Phase != phase {
		t.Fatalf("public task query phase=%+v, want %s", page.Tasks, phase)
	}
}

func lifecycleOperations(runtimeState *reconstructiontypes.Runtime, kind string) []executionmodel.OperationRecord {
	var result []executionmodel.OperationRecord
	for _, operation := range runtimeState.Operations {
		if operation.Kind == kind {
			result = append(result, operation)
		}
	}
	return result
}

func hasOperationStatus(operations []executionmodel.OperationRecord, kind, status string) bool {
	for _, operation := range operations {
		if operation.Kind == kind && operation.Status == status {
			return true
		}
	}
	return false
}

func hasAcknowledgement(runtimeState *reconstructiontypes.Runtime, deliveryID, status string) bool {
	for _, operation := range lifecycleOperations(runtimeState, "acknowledge_delivery") {
		if operation.Status != status {
			continue
		}
		var input executionbackend.AcknowledgeInput
		if json.Unmarshal(operation.Request, &input) == nil && input.DeliveryID == deliveryID {
			return true
		}
	}
	return false
}

func decisionWasAccepted(runtimeState *reconstructiontypes.Runtime, decisionID string) bool {
	for _, decision := range runtimeState.Decisions {
		if decision.DecisionID == decisionID {
			return decision.Status == "reply_accepted"
		}
	}
	return false
}

func assertNoTaskDelivery(t *testing.T, runtimeState *reconstructiontypes.Runtime, run *reconstructiontypes.RunView) {
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
