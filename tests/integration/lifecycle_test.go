package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"

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

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

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
		return run.Decision != nil && run.Decision.Status == "pending" && execution.TurnNumber == 2 && execution.CurrentTurnID == execution.LastStoppedTurnID
	})
	if first.run.Process == nil {
		t.Fatal("first app-server receipt omitted its process identity")
	}
	firstSession, firstProcess, firstTurn := first.execution.SessionID, *first.run.Process, first.execution.CurrentTurnID
	if firstSession != integrationAppServerSession || firstProcess.Kind != "os" || firstTurn == "" {
		t.Fatalf("first app-server receipt lost its original session/process/turn: session=%q process=%+v turn=%q", firstSession, firstProcess, firstTurn)
	}
	assertQueriedTaskStatus(t, ctx, fixture.config, "901", "waiting_input")
	submitIntegrationAnswer(t, ctx, fixture.config, first.run)

	second := waitIntegrationRun(t, ctx, service, fixture.config, func(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord) bool {
		return execution.TurnNumber == 3 && execution.CurrentTurnID != firstTurn && execution.CurrentTurnID == execution.LastStoppedTurnID &&
			run.Decision != nil && run.Decision.Status == "pending"
	})
	if second.execution.SessionID != firstSession || second.run.Process == nil || *second.run.Process != firstProcess {
		t.Fatalf("second app-server turn replaced its original session or process: first=%+v/%s second=%+v/%s", firstProcess, firstSession, second.run.Process, second.execution.SessionID)
	}
	starts := lifecycleOperations(second.runtime, "start_worker_turn")
	prepares := lifecycleOperations(second.runtime, "prepare_worker_session")
	if len(starts) != 3 || len(prepares) != 1 {
		t.Fatalf("app-server operation receipts = prepare:%d start:%d, want one original session and execute/report/feedback turns", len(prepares), len(starts))
	}
	assertNoTaskDelivery(t, second.runtime, second.run)
	assertQueriedTaskStatus(t, ctx, fixture.config, "901", "waiting_input")
	if resolver.count(executionbackend.ProtocolLocalCodexAppServer) == 0 {
		t.Fatal("public service flow never resolved the app-server lifecycle adapter")
	}
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
	if host != "local" {
		t.Fatal("shared fixture only models local app-server; terminal transport has its own fixture")
	}
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

	codexProfile := codexworker.ProfileConfig{Executable: os.Args[0]}
	if runtime.GOOS == "windows" && (host == "local" || host == "orca") {
	}
	codexPayload, err := codexworker.EncodeProfileConfig(codexProfile)
	require(t, err)
	implementationConfigs := []executionbackend.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 2, Payload: codexPayload}}

	stateDir := filepath.Join(root, "native-protocol")

	globalData, err := json.Marshal(map[string]any{
		"version": 2, "default_execution_profile": "fixture",
		"execution_profiles": []any{map[string]any{"id": "fixture", "name": "Integration fixture", "host": host, "agent": "codex", "implementation_configs": implementationConfigs}},
	})
	require(t, err)
	require(t, os.MkdirAll(filepath.Dir(steerlineapp.GlobalExecutionConfigPath()), 0700))
	require(t, os.WriteFile(steerlineapp.GlobalExecutionConfigPath(), globalData, 0600))
	projectData, err := json.Marshal(map[string]any{
		"repository": repository, "remote": "origin", "branch": "main", "task_authority": "native", "workflow_protocol": "worker-flow-v2",
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

func createIntegrationTask(t *testing.T, ctx context.Context, fixture integrationFixture, id int64, predecessors ...int64) {
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
	for _, predecessor := range predecessors {
		batch.Dependencies = append(batch.Dependencies, taskmodel.TaskBatchEdge{Predecessor: taskmodel.TaskBatchReference{TaskID: &predecessor}, Successor: taskmodel.TaskBatchReference{LocalKey: "fixture"}})
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
	case executionbackend.ProtocolOrcaTerminalV3:
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
	predicate func(*reconstructiontypes.RunView, *executionmodel.ExecutionRecord) bool, budgets ...time.Duration) integrationRunSnapshot {
	t.Helper()
	var result integrationRunSnapshot
	t.Cleanup(func() {
		if t.Failed() && result.runtime != nil {
			t.Logf("last public lifecycle: decision=%s current=%s stopped=%s turns=%d pending=%s command=%s pendingCommand=%s status=%s error=%s", mustJSON(result.run.Decision), result.execution.CurrentTurnID, result.execution.LastStoppedTurnID, result.execution.TurnNumber, result.execution.PendingTurnID, result.execution.CurrentCommandID, result.execution.PendingCommandID, result.execution.Status, derefString(result.execution.Error))
			t.Logf("original preparation evidence: %s; last event: %s", mustJSON(result.execution.Environment), derefString(result.execution.LastEvent))
			t.Logf("service: %+v; operation count: %d", result.runtime.Service, len(result.runtime.Operations))
		}
	})
	budget := 2 * time.Minute
	if len(budgets) > 0 {
		budget = budgets[0]
	}
	waitFor(t, budget, func() bool {
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
		command, created, err := steerlineapp.SubmitDecisionAnswer(ctx, config, answer)
		t.Logf("original answer submission: decision=%s version=%d command=%s created=%t status=%s error=%v", answer.DecisionID, answer.ExpectedVersion, command.CommandID, created, command.Status, err)
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

func assertQueriedTaskStatus(t *testing.T, ctx context.Context, config steerlineapp.Config, taskID, phase string) {
	t.Helper()
	reader, err := steerlineapp.OpenNativeTaskReader(ctx, &config)
	require(t, err)
	defer reader.Close()
	page, err := productquery.NewService(reader, config.Identity()).ShowNativeTask(ctx, taskID)
	require(t, err)
	if len(page.Tasks) != 1 || len(page.Tasks[0].WorkflowRuns) != 1 || page.Tasks[0].WorkflowRuns[0].Status != phase {
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

// Keep native multi-turn mutation-loss coverage explicit when broad aggregate
// fixture cases are simplified on main.
