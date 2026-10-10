package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"strings"
	"sync"
	"testing"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/execution"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	terminal "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskapp "github.com/erranliu/SteerLine/internal/task"
)

// All mutations go through the production app factory. SQL is used only to
// install a transaction failure and to inspect committed evidence.
type appCandidateFixture struct {
	f        integrationFixture
	store    *sqlite.SQLiteStore
	commands execution.CommandService
	query    productquery.Service
	ctx      context.Context
}

func newAppCandidateFixture(t *testing.T) appCandidateFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	f := newLifecycleProject(t, "local")
	createIntegrationTask(t, ctx, f, 901)
	store, err := steerlineapp.OpenNativeTaskCommandStore(ctx, &f.config)
	require(t, err)
	t.Cleanup(func() { require(t, store.Close()) })
	commands, err := steerlineapp.NewCommandService(store, f.config)
	require(t, err)
	return appCandidateFixture{f: f, store: store, commands: commands, query: productquery.NewService(store, f.config.Identity()), ctx: ctx}
}

func (f appCandidateFixture) page(t *testing.T) storageports.NativeTaskPage {
	t.Helper()
	p, err := f.query.ShowNativeTask(f.ctx, "901")
	require(t, err)
	if len(p.Tasks) != 1 {
		t.Fatal("app query lost task")
	}
	return p
}

func (f appCandidateFixture) start(t *testing.T, instance string) (executionmodel.TaskClaimReceipt, execution.TaskClaimOptions) {
	t.Helper()
	p := f.page(t)
	profiles := f.f.config.AvailableExecutionProfiles()
	options := execution.TaskClaimOptions{TaskID: "901", RequestID: newID(), ExpectedTaskVersion: p.Tasks[0].Version, ExpectedAuthorityVersion: p.AuthorityVersion, ExpectedGraphRevision: p.GraphRevision, RequestedBy: "operator", Explicit: true, ServiceInstanceID: instance}
	fixed := profiles[0].Freeze(f.f.config.ExecutionDirectory, f.f.config.Identity())
	options.ExecutionProfile = &fixed
	var receipt executionmodel.TaskClaimReceipt
	var err error
	if p.Tasks[0].Status == "open" {
		receipt, err = f.commands.ClaimNativeTask(f.ctx, f.f.config.ExecutionConfig(), options)
	} else {
		receipt, err = f.commands.StartNativeCandidate(f.ctx, f.f.config.ExecutionConfig(), options)
	}
	require(t, err)
	return receipt, options
}

func (f appCandidateFixture) abandon(t *testing.T, id string) {
	t.Helper()
	snapshot, err := f.store.Query(f.ctx, f.f.config.Identity())
	require(t, err)
	for _, w := range snapshot.WorkflowRuns {
		if w.WorkflowRunID != id {
			continue
		}
		candidateVersion := w.ControlRevision
		if candidateVersion == 0 {
			t.Fatal("candidate control revision unavailable from command snapshot or runtime query")
		}
		graph, err := f.store.TaskGraphSnapshot(f.ctx, f.f.config.Identity())
		require(t, err)
		task := graph.ByID["901"]
		r, err := f.commands.AbandonNativeCandidate(f.ctx, f.f.config.ExecutionConfig(), execution.CandidateAbandonOptions{TaskID: "901", WorkflowRunID: id, RequestID: newID(), ExpectedTaskVersion: task.Version, ExpectedCandidateVersion: candidateVersion, RequestedBy: "operator", Reason: "comparison complete"})
		require(t, err)
		if r.Status != "abandoned" {
			t.Fatalf("exit: %+v", r)
		}
		return
	}
	t.Fatal("exact candidate absent")
}

func TestTM07StorageWiring(t *testing.T) {
	f := newAppCandidateFixture(t)
	var _ storageports.ProductReader = f.store
	commands, err := steerlineapp.NewCommandService(f.store, f.f.config)
	require(t, err)
	commandType := reflect.TypeOf(execution.CommandService{})
	for _, field := range []string{"Store", "Work"} {
		value, ok := commandType.FieldByName(field)
		if !ok || value.Type.Kind() != reflect.Interface {
			t.Fatalf("execution command field %s is not a consumer port", field)
		}
	}
	f.commands = commands
	f.start(t, "")
	page := f.page(t)
	if page.Tasks[0].Status != "in_progress" || len(page.Tasks[0].WorkflowRuns) != 1 {
		t.Fatal("task, execution and product readers did not observe the same SQLite database")
	}
	for _, field := range []string{"Workflows", "Events", "Tasks", "Projects", "Activity"} {
		value, ok := reflect.TypeOf(productquery.Service{}).FieldByName(field)
		if !ok || value.Type.Kind() != reflect.Interface {
			t.Fatalf("product query field %s is not an owned read port", field)
		}
	}
	taskReader, ok := reflect.TypeOf(taskapp.Service{}).FieldByName("Reader")
	if !ok || taskReader.Type.Kind() != reflect.Interface {
		t.Fatal("task service reader is not a consumer-owned interface")
	}
	storageType := reflect.TypeOf(execution.ServiceStorage{})
	for _, field := range []string{"State", "Commands", "Worker", "Conversations", "Records"} {
		value, ok := storageType.FieldByName(field)
		if !ok || value.Type.Kind() != reflect.Interface {
			t.Fatalf("execution storage field %s is not a narrow port", field)
		}
	}
	closeField, ok := storageType.FieldByName("Close")
	if !ok || closeField.Type.Kind() != reflect.Func || closeField.Type.String() != "func() error" {
		t.Fatal("execution storage bundle does not receive an application-owned close callback")
	}
	service, err := steerlineapp.NewServiceWithLifecycleResolver(f.f.config, &integrationLifecycleResolver{})
	require(t, err)
	require(t, service.Close())
	reader, err := steerlineapp.OpenNativeTaskReader(f.ctx, &f.f.config)
	require(t, err)
	require(t, reader.Close())
}

func TestMC1(t *testing.T) {
	f := newAppCandidateFixture(t)
	ids, executions, branches, paths := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i := 0; i < 5; i++ {
		r, _ := f.start(t, "")
		if ids[r.WorkflowRunID] || executions[r.ExecutionID] || r.WorkflowRunID == "" || r.ExecutionID == "" {
			t.Fatal("app reused candidate identity")
		}
		ids[r.WorkflowRunID], executions[r.ExecutionID] = true, true
	}
	snapshot, err := f.store.Query(f.ctx, f.f.config.Identity())
	require(t, err)
	for _, e := range snapshot.Executions {
		if e.WorkspaceIntent == nil || branches[e.WorkspaceIntent.Branch] || paths[e.WorkspaceIntent.Path] || e.WorkspaceIntent.BaseCommit != f.f.commit {
			t.Fatal("app lost fixed independent workspace intent")
		}
		branches[e.WorkspaceIntent.Branch], paths[e.WorkspaceIntent.Path] = true, true
	}
	p := f.page(t)
	if p.Tasks[0].Version != 2 || len(p.Tasks[0].WorkflowRuns) != 5 || len(snapshot.Executions) != 5 {
		t.Fatal("additional candidates reclaimed task or vanished from app query")
	}
}

func TestMC9(t *testing.T) {
	f := newAppCandidateFixture(t)
	first, _ := f.start(t, "")
	var last executionmodel.TaskClaimReceipt
	var options execution.TaskClaimOptions
	for i := 1; i < 5; i++ {
		last, options = f.start(t, "")
	}
	replayed, err := f.commands.StartNativeCandidate(f.ctx, f.f.config.ExecutionConfig(), options)
	require(t, err)
	if replayed.WorkflowRunID != last.WorkflowRunID {
		t.Fatal("at-limit replay replaced original candidate")
	}
	options.RequestID = newID()
	if _, err = f.commands.StartNativeCandidate(f.ctx, f.f.config.ExecutionConfig(), options); err == nil || !strings.Contains(err.Error(), "candidate_limit_reached") {
		t.Fatalf("sixth candidate: %v", err)
	}
	f.abandon(t, first.WorkflowRunID)
	f.start(t, "")
	p := f.page(t)
	if p.Tasks[0].Version != 2 || len(p.Tasks[0].WorkflowRuns) != 6 {
		t.Fatal("replacement changed task claim or pruned history")
	}
	var participating int
	for _, w := range p.Tasks[0].WorkflowRuns {
		if w.Abandonment == nil {
			participating++
		}
	}
	if participating != 5 {
		t.Fatal("exit did not immediately release participation")
	}
}

func TestMC7(t *testing.T) {
	f := newAppCandidateFixture(t)
	a, _ := f.start(t, "")
	b, _ := f.start(t, "")
	f.abandon(t, a.WorkflowRunID)
	f.abandon(t, b.WorkflowRunID)
	p := f.page(t)
	if p.Tasks[0].Status != "in_progress" || p.Tasks[0].Version != 2 {
		t.Fatal("empty experiment reopened task")
	}
	// A new app construction reads the same authoritative candidate history.
	commands, err := steerlineapp.NewCommandService(f.store, f.f.config)
	require(t, err)
	f.commands = commands
	next, _ := f.start(t, "")
	if next.WorkflowRunID == a.WorkflowRunID || next.WorkflowRunID == b.WorkflowRunID {
		t.Fatal("restart revived original candidate")
	}
	r, err := f.commands.DiscardNativeTask(f.ctx, f.f.config.ExecutionConfig(), "901", newID(), 2, "discard experiment", "operator")
	require(t, err)
	if r.Status != "discarded" || f.page(t).Tasks[0].Status != "discarded" {
		t.Fatal("app discard failed")
	}
}

func TestMC3(t *testing.T) {
	f := newAppCandidateFixture(t)
	service := startIntegrationService(t, f.ctx, f.f.config, &integrationLifecycleResolver{})
	a, _ := f.start(t, service.instanceID)
	b, _ := f.start(t, service.instanceID)
	ready := func(id string) integrationRunSnapshot {
		return waitIntegrationRun(t, f.ctx, service, f.f.config, func(r *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return r.RunID == id && r.Decision != nil && r.Decision.Status == "pending" && e.TurnNumber == 2 && e.CurrentTurnID == e.LastStoppedTurnID
		})
	}
	first := ready(a.WorkflowRunID)
	second := ready(b.WorkflowRunID)
	submitIntegrationAnswer(t, f.ctx, f.f.config, second.run)
	after := waitIntegrationRun(t, f.ctx, service, f.f.config, func(r *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return r.RunID == b.WorkflowRunID && e.TurnNumber == 3 && e.CurrentTurnID == e.LastStoppedTurnID
	})
	if after.execution.SessionID != second.execution.SessionID || after.run.Process == nil || *after.run.Process != *second.run.Process {
		t.Fatal("selected feedback replaced original session/process")
	}
	snapshot, err := f.store.Query(f.ctx, f.f.config.Identity())
	require(t, err)
	for _, e := range snapshot.Executions {
		if e.ExecutionID == a.ExecutionID && (e.TurnNumber != 2 || e.CurrentTurnID != first.execution.CurrentTurnID) {
			t.Fatal("candidate B feedback crossed to candidate A")
		}
	}
}

func TestMC6(t *testing.T) { testAdditionalImplementationWithCandidateGuard(t, true) }

type delayedAppResolver struct {
	directory *execution.ExecutionDirectory
	entered   chan executionbackend.LifecycleRequest
	release   chan struct{}
	once      sync.Once
}

func (r *delayedAppResolver) ResolveLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	p, err := r.directory.ResolveLifecycle(setup)
	if err != nil {
		return nil, err
	}
	return delayedAppPort{LifecyclePort: p, resolver: r}, nil
}

type delayedAppPort struct {
	executionbackend.LifecyclePort
	resolver *delayedAppResolver
}

func (p delayedAppPort) PolicyCapabilities() executionbackend.LifecyclePolicyCapabilities {
	return p.LifecyclePort.(executionbackend.LifecyclePolicyPort).PolicyCapabilities()
}

func (p delayedAppPort) Execute(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if request.Operation.Action == string(executionbackend.LifecyclePrepareSession) {
		p.resolver.once.Do(func() { p.resolver.entered <- request.Clone() })
		select {
		case <-p.resolver.release:
		case <-ctx.Done():
			return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown}, nil, ctx.Err()
		}
	}
	return p.LifecyclePort.Execute(ctx, request)
}

func TestMC8(t *testing.T) {
	f := newAppCandidateFixture(t)
	resolver := &delayedAppResolver{directory: f.f.config.ExecutionDirectory, entered: make(chan executionbackend.LifecycleRequest, 1), release: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(resolver.release) })
	service := startIntegrationService(t, f.ctx, f.f.config, resolver)
	claimed, _ := f.start(t, service.instanceID)
	var request executionbackend.LifecycleRequest
	select {
	case request = <-resolver.entered:
	case <-time.After(30 * time.Second):
		t.Fatal("real app never reached original asynchronous action")
	}
	snapshot, err := f.store.Query(f.ctx, f.f.config.Identity())
	require(t, err)
	var persisted bool
	for _, op := range snapshot.Operations {
		if op.OperationID == request.Operation.ID && op.Status == "in_flight" {
			persisted = true
		}
	}
	if !persisted {
		t.Fatal("external action ran before committed original intent")
	}
	f.abandon(t, claimed.WorkflowRunID)
	// The actor processed exit while the external action was still blocked.
	release.Do(func() { close(resolver.release) })
	late := waitIntegrationRun(t, f.ctx, service, f.f.config, func(r *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return r.RunID == claimed.WorkflowRunID && e.Process != nil
	})
	if late.execution.ExecutionID != request.Snapshot.Session.ExecutionID || late.execution.CurrentAttemptID != request.Snapshot.Session.AttemptID || late.execution.SessionID == "" {
		t.Fatal("late receipt replaced the original execution binding")
	}
	p := f.page(t)
	if p.Tasks[0].WorkflowRuns[0].Abandonment == nil || p.Tasks[0].Status != "in_progress" || p.Tasks[0].Delivery != nil {
		t.Fatal("late action revived abandoned business or fabricated delivery")
	}
}

func TestTM04ExecutionWiring(t *testing.T) {
	t.Run("claim_transaction_failure_and_original_receipt", func(t *testing.T) {
		f := newAppCandidateFixture(t)
		db, err := sql.Open("sqlite", filepath.Join(f.f.config.RunDir, "steerline.db"))
		require(t, err)
		defer db.Close()
		p := f.page(t)
		profile := f.f.config.AvailableExecutionProfiles()[0]
		fixed := profile.Freeze(f.f.config.ExecutionDirectory, f.f.config.Identity())
		options := execution.TaskClaimOptions{TaskID: "901", RequestID: newID(), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: p.AuthorityVersion, RequestedBy: "operator", Explicit: true, ExecutionProfile: &fixed}
		_, err = db.ExecContext(f.ctx, `CREATE TRIGGER fail_execution_audit BEFORE INSERT ON operations BEGIN SELECT RAISE(ABORT,'execution audit failure'); END`)
		require(t, err)
		if r, err := f.commands.ClaimNativeTask(f.ctx, f.f.config.ExecutionConfig(), options); err == nil || r.WorkflowRunID != "" {
			t.Fatalf("failed commit exposed claim: %+v/%v", r, err)
		}
		before := f.page(t)
		if before.Tasks[0].Status != "open" || before.Tasks[0].Version != 1 || len(before.Tasks[0].WorkflowRuns) != 0 || before.GraphRevision != p.GraphRevision || before.AuthorityVersion != p.AuthorityVersion {
			t.Fatal("failed transaction left task/execution mutations")
		}
		_, err = db.ExecContext(f.ctx, `DROP TRIGGER fail_execution_audit`)
		require(t, err)
		claimed, err := f.commands.ClaimNativeTask(f.ctx, f.f.config.ExecutionConfig(), options)
		require(t, err)
		original, err := f.store.Query(f.ctx, f.f.config.Identity())
		require(t, err)
		replayed, err := f.commands.ClaimNativeTask(f.ctx, f.f.config.ExecutionConfig(), options)
		require(t, err)
		after, err := f.store.Query(f.ctx, f.f.config.Identity())
		require(t, err)
		x, _ := json.Marshal(original)
		y, _ := json.Marshal(after)
		if claimed != replayed || string(x) != string(y) {
			t.Fatal("original request replay changed receipt or database revisions")
		}
	})
	t.Run("intent_binding_and_two_original_turns", testIntegratedAppServerTwoTurns)
}

func TestTM08AppHostWiring(t *testing.T) {
	t.Run("default_app_host_composition", func(t *testing.T) {
		f := newAppCandidateFixture(t)
		service, err := steerlineapp.NewServiceWithLifecycleResolver(f.f.config, &integrationLifecycleResolver{})
		require(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		defer service.Close()
		if err := service.BeginActor(ctx); err != nil {
			t.Fatal(err)
		}
		if err := service.Initialize(ctx, cancel); err != nil {
			t.Fatal(err)
		}
		if service.State.Service.InstanceID == "" || !service.State.Service.Ready {
			t.Fatalf("app constructor did not initialize and publish the host identity: %+v", service.State.Service)
		}
		if err := service.Heartbeat(); err != nil {
			t.Fatalf("host heartbeat failed through the injected execution port: %v", err)
		}
		if err := service.EndActor(nil, nil, nil); err != nil {
			t.Fatalf("host shutdown failed: %v", err)
		}
		if service.State.Service.Status != "stopped" || service.State.Service.Ready {
			t.Fatalf("host shutdown left the service ready: %+v", service.State.Service)
		}
	})
}

func TestTM06ResourceWiringHelper(t *testing.T) {
	if os.Getenv("STEERLINE_TM06_RESOURCE_HELPER") != "1" {
		return
	}
	_, _ = fmt.Fprint(os.Stdout, "tm06-managed-process-output")
}

func TestTM06ResourceWiring(t *testing.T) {
	host := steerlineapp.NewProcessHost(10 * time.Second)
	file := filepath.Join(t.TempDir(), "execution", "receipt.json")
	require(t, host.WriteExecutionFile(file, []byte("{\"generation\":\"original\"}")))
	data, err := os.ReadFile(file)
	require(t, err)
	if string(data) != "{\"generation\":\"original\"}" {
		t.Fatal("process resource host changed execution-file bytes")
	}
	firstGeneration, secondGeneration := host.NewProcessGeneration(), host.NewProcessGeneration()
	if firstGeneration == "" || secondGeneration == "" || firstGeneration == secondGeneration {
		t.Fatal("process resource host did not provide distinct generations")
	}
	t.Setenv("STEERLINE_TM06_RESOURCE_HELPER", "1")
	executable, err := os.Executable()
	require(t, err)
	output, err := host.OutputCommand(context.Background(), executionbackend.LaunchCommand{
		Executable: executable, Args: []string{"-test.run=^TestTM06ResourceWiringHelper$"}, WorkingDirectory: t.TempDir(),
	})
	require(t, err)
	if !strings.Contains(string(output), "tm06-managed-process-output") {
		t.Fatalf("managed local child output missing: %s", output)
	}

	directory, err := steerlineapp.NewBuiltinDirectory(host)
	require(t, err)
	workspacePath := t.TempDir()
	codexPayload, err := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: "codex"})
	require(t, err)
	codexEnvelope, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{
		CodecVersion: 2, Purpose: executionbackend.ImplementationConfigPurpose, ImplementationKey: codexworker.ProfileCodecKey,
		ProjectID: "project", ProfileID: "profile", AcceptedConfigVersion: "accepted",
	}, codexPayload)
	require(t, err)
	for _, test := range []struct {
		host, wantProtocol, factory, backendPackage string
	}{
		{"local", string(executionbackend.ProtocolLocalCodexAppServer), "github.com/erranliu/SteerLine/internal/execution/adapters/codex.NewExecutionLifecycle", ""},
		{"orca", string(executionbackend.ProtocolOrcaTerminalV3), "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport.NewLifecycle", "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport"},
	} {
		config := execution.Config{Host: test.host, Agent: "codex"}
		entry, err := directory.Resolve(config)
		require(t, err)
		if string(entry.Key.Protocol) != test.wantProtocol {
			t.Fatalf("%s selected protocol %q, want %q", test.host, entry.Key.Protocol, test.wantProtocol)
		}
		if test.backendPackage != "" && reflect.TypeOf(entry.Backend(config)).PkgPath() != test.backendPackage {
			t.Fatalf("%s backend resolves outside its adapter package: %T", test.host, entry.Backend(config))
		}
		if test.host == "local" {
			setup := executionbackend.LifecycleSetup{
				Snapshot: executionbackend.LifecycleSnapshot{
					Ref: executionbackend.ExecutionRef{Backend: executionbackend.BackendLocal, Protocol: executionbackend.ProtocolLocalCodexAppServer,
						Agent: executionbackend.AgentSpec{Name: "codex"}, WorkspacePath: workspacePath},
					Session: executionbackend.SessionContext{ProjectID: "project", WorkflowRunID: "workflow", ExecutionID: "execution", AttemptID: "attempt", Agent: executionbackend.AgentSpec{Name: "codex"}},
				},
				ProfileData: []executionbackend.ImplementationDataEnvelope{codexEnvelope}, ProfileID: "profile", AcceptedConfigVersion: "accepted",
				ProcessHost: host, RepositoryPath: workspacePath, RunDirectory: filepath.Join(t.TempDir(), "run"),
			}
			lifecycle, err := entry.Lifecycle(setup)
			require(t, err)
			lifecycleType := reflect.TypeOf(lifecycle)
			if lifecycleType.Kind() == reflect.Pointer {
				lifecycleType = lifecycleType.Elem()
			}
			if lifecycleType.PkgPath() != strings.TrimSuffix(test.factory, ".NewExecutionLifecycle") {
				t.Fatalf("local lifecycle resolves outside its adapter package: %T", lifecycle)
			}
			continue
		}
		p := terminal.Profile{Executable: "orca", RepositorySelector: "id:repo", WaitTimeout: "30s"}
		envelope, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{CodecVersion: terminal.ProfileVersion, Purpose: executionbackend.ImplementationConfigPurpose, ImplementationKey: terminal.ProfileKey, ProjectID: "project", ProfileID: "profile", AcceptedConfigVersion: "accepted"}, mustJSON(p))
		require(t, err)
		lifecycle, err := entry.Lifecycle(executionbackend.LifecycleSetup{Snapshot: executionbackend.LifecycleSnapshot{Ref: executionbackend.ExecutionRef{Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaTerminalV3, Target: p.Target(), Agent: executionbackend.AgentSpec{Name: "opaque"}}, Session: executionbackend.SessionContext{ProjectID: "project"}}, ProfileData: []executionbackend.ImplementationDataEnvelope{envelope}, ProfileID: "profile", AcceptedConfigVersion: "accepted"})
		require(t, err)
		typ := reflect.TypeOf(lifecycle)
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if typ.PkgPath() != test.backendPackage {
			t.Fatalf("Orca lifecycle outside adapter: %T", lifecycle)
		}

	}
}
