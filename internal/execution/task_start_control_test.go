package execution

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type startGate struct {
	runID            string
	entered, release chan struct{}
	once             sync.Once
	calls            atomic.Int32
	failure          error
}

func (g *startGate) wait(ctx context.Context) error {
	g.calls.Add(1)
	g.once.Do(func() { close(g.entered) })
	select {
	case <-g.release:
		return g.failure
	case <-ctx.Done():
		return ctx.Err()
	}
}

type slowStartWork struct {
	TaskWorkPort
	gate *startGate
}

func (w slowStartWork) Fixed(ctx context.Context, project, taskID string, version uint64, candidate bool) (taskmodel.FixedWork, error) {
	if taskID == "101" {
		if err := w.gate.wait(ctx); err != nil {
			return taskmodel.FixedWork{}, err
		}
	}
	return w.TaskWorkPort.Fixed(ctx, project, taskID, version, candidate)
}

type slowStartResolver struct {
	gate   *startGate
	action executionbackend.LifecycleAction
}

func (r slowStartResolver) ResolveLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	return slowStartPort{mcPort: mcPort{&neutralLifecycle{Setup: setup}}, gate: r.gate, action: r.action}, nil
}

type slowStartPort struct {
	mcPort
	gate   *startGate
	action executionbackend.LifecycleAction
}

func (p slowStartPort) PreflightLifecycle(ctx context.Context, setup executionbackend.LifecycleSetup) (executionbackend.CapabilityReport, error) {
	if p.action == "" && setup.Snapshot.Session.WorkflowRunID == p.gate.runID {
		if err := p.gate.wait(ctx); err != nil {
			return executionbackend.CapabilityReport{}, err
		}
	}
	return p.mcPort.PreflightLifecycle(ctx, setup)
}

func (p slowStartPort) Execute(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if request.Snapshot.Session.WorkflowRunID == p.gate.runID && request.Operation.Action == string(p.action) {
		if err := p.gate.wait(ctx); err != nil {
			return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown}, nil, err
		}
	}
	return p.mcPort.Execute(ctx, request)
}

func newTaskStartFixture(t *testing.T) (*Service, Config) {
	t.Helper()
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	profiles := []ExecutionProfile{
		neutralProfile(t, "first", "First", neutralSettings{Executable: os.Args[0], Model: "first", ReasoningEffort: "medium"}),
		neutralProfile(t, "second", "Second", neutralSettings{Executable: os.Args[0], Model: "second", ReasoningEffort: "high"}),
	}
	c = withNeutralProfiles(c, profiles, "first")
	commands := newTestCommandService(store, c)
	input := taskBatchFixture(t, c, commit, batchTasks(101, 105, 106), []taskmodel.TaskBatchEdge{{Predecessor: taskmodel.TaskBatchReference{LocalKey: "k101"}, Successor: taskmodel.TaskBatchReference{LocalKey: "k106"}}})
	_, err := commands.CreateTaskBatch(ctx, c, input, newID(), 1, "operator")
	require(t, err)
	authority, err := store.NativeAuthority(ctx, c.Identity())
	require(t, err)
	authority.DispatchPaused, authority.AuthorityVersion = false, authority.AuthorityVersion+1
	require(t, store.transactRecords(ctx, func(tx *fixtureTransaction) error {
		return tx.UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}))
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("fixture push: %v: %s", err, out)
	}
	s, err := newNeutralService(c)
	require(t, err)
	s.lifecycleResolver = mcResolver{}
	s.State.Service.Ready = true
	require(t, s.publishCheckpoint())
	t.Cleanup(func() { _ = s.Close() })
	return s, c
}

func startControlRequest(t *testing.T, s *Service, taskID, profileID string, expectedVersion ...uint64) productapi.ControlRequest {
	t.Helper()
	_, profile, err := s.runtime.Profiles.ResolveExecutionProfile(profileID, "")
	require(t, err)
	at := now()
	payload := productapi.ControlPayload{TaskID: taskID, ExecutionProfileID: profile.ID, ExecutionProfileDigest: profile.Digest}
	if len(expectedVersion) > 0 {
		payload.ExpectedTaskVersion = expectedVersion[0]
	}
	req := productapi.ControlRequest{ProtocolVersion: ControlProtocolVersion, RequestID: newID(), RepositoryIdentity: s.Config.Identity(), InstanceID: s.State.Service.InstanceID, ExpectedControlRevision: s.State.Control.Revision, Action: "start_task", Payload: mustJSON(payload), RequestedBy: "operator", CreatedAt: at, ExpiresAt: at.Add(2 * time.Minute)}
	require(t, publishControlRequest(s.Config.RunDir, req))
	return req
}

func driveTaskStarts(t *testing.T, ctx context.Context, s *Service, done func() bool) {
	t.Helper()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for {
		require(t, s.resumeWorkerFlow(ctx))
		require(t, s.pollTaskControlReceipts(ctx))
		if done() {
			return
		}
		select {
		case message := <-s.signalQueue:
			require(t, s.HandleSignal(message))
		case <-time.After(10 * time.Millisecond):
		case <-deadline.C:
			t.Fatalf("independent start did not progress: runs=%s", mustJSON(s.runs))
		}
	}
}

func TestTaskStartControlDoesNotSerializeIndependentTasks(t *testing.T) {
	for _, stage := range []string{"preflight", "worker-preflight", "worker-preflight-failure", "workspace", "session"} {
		t.Run(stage, func(t *testing.T) {
			s, c := newTaskStartFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			s.runContext, s.signalQueue = ctx, make(chan Signal, 128)
			t.Cleanup(func() { cancel(); s.shutdown() })
			gate := &startGate{entered: make(chan struct{}), release: make(chan struct{})}
			if stage == "worker-preflight-failure" {
				gate.failure = errors.New("task 101's worker endpoint is unavailable")
			}
			if stage == "preflight" {
				s.taskWork = slowStartWork{TaskWorkPort: s.taskWork, gate: gate}
			} else {
				action := executionbackend.LifecyclePrepareWorkspace
				if stage == "worker-preflight" || stage == "worker-preflight-failure" {
					action = ""
				} else if stage == "session" {
					action = executionbackend.LifecyclePrepareSession
				}
				s.lifecycleResolver = slowStartResolver{gate: gate, action: action}
			}
			first := startControlRequest(t, s, "101", "first")
			gate.runID = first.RequestID
			require(t, s.pollControl(ctx, cancel))
			driveTaskStarts(t, ctx, s, func() bool {
				select {
				case <-gate.entered:
					return true
				default:
					return false
				}
			})
			second := startControlRequest(t, s, "105", "second")
			require(t, s.pollControl(ctx, cancel))
			driveTaskStarts(t, ctx, s, func() bool {
				r, err := ReadControlReceipt(c, second.RequestID)
				return err == nil && r.Status == "applied"
			})
			if s.State.Control.Operation != nil || s.State.Control.Revision != first.ExpectedControlRevision {
				t.Fatal("task startup held or changed the global control slot")
			}
			if r, err := ReadControlReceipt(c, first.RequestID); err == nil && r.Status != "applying" || err != nil && !os.IsNotExist(err) {
				t.Fatalf("slow original request was prematurely settled: %+v / %v", r, err)
			}
			duplicate := startControlRequest(t, s, "101", "first")
			require(t, s.pollControl(ctx, cancel))
			r, err := ReadControlReceipt(c, duplicate.RequestID)
			require(t, err)
			if r.Status != "rejected" || r.Code != "already_managed" {
				t.Fatalf("duplicate start admitted: %+v", r)
			}
			dependent := startControlRequest(t, s, "106", "second")
			require(t, s.pollControl(ctx, cancel))
			r, err = ReadControlReceipt(c, dependent.RequestID)
			require(t, err)
			if r.Status != "rejected" {
				t.Fatal("parallel startup bypassed a DAG dependency")
			}
			close(gate.release)
			finalStatus := "applied"
			if gate.failure != nil {
				finalStatus = "failed"
			}
			driveTaskStarts(t, ctx, s, func() bool {
				r, err := ReadControlReceipt(c, first.RequestID)
				return err == nil && r.Status == finalStatus
			})
			a, b := s.find(first.RequestID), s.find(second.RequestID)
			if a == nil || b == nil || a.ExecutionID == b.ExecutionID || a.SessionID == b.SessionID || s.workflowRecord(a).ExecutionProfile.ID != "first" || s.workflowRecord(b).ExecutionProfile.ID != "second" || gate.calls.Load() != 1 {
				t.Fatal("startup mixed identities/profiles or repeated the original effect")
			}
		})
	}
}

func TestTaskStartControlRejectsStaleDisplayedTaskVersion(t *testing.T) {
	s, c := newTaskStartFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := startControlRequest(t, s, "101", "first", 999)
	require(t, s.pollControl(ctx, cancel))
	receipt, err := ReadControlReceipt(c, request.RequestID)
	require(t, err)
	if receipt.Status != "rejected" || receipt.Code != "stale_version" || len(s.domain.Executions) != 0 {
		t.Fatalf("stale displayed task version was silently retargeted: receipt=%+v executions=%d", receipt, len(s.domain.Executions))
	}
}

func TestTaskStartControlReportsUnsupportedCapabilityInReceipt(t *testing.T) {
	s, c := newTaskStartFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	s.runContext, s.signalQueue = ctx, make(chan Signal, 8)
	t.Cleanup(func() { cancel(); s.shutdown() })
	gate := &startGate{entered: make(chan struct{}), release: make(chan struct{}), failure: &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: "fixed development flow has no verified structured final source"}}
	s.taskWork = slowStartWork{TaskWorkPort: s.taskWork, gate: gate}
	request := startControlRequest(t, s, "101", "first")
	require(t, s.pollControl(ctx, cancel))
	select {
	case <-gate.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("preparation did not start")
	}
	close(gate.release)
	select {
	case message := <-s.signalQueue:
		require(t, s.HandleSignal(message))
	case <-time.After(10 * time.Second):
		t.Fatal("unsupported preparation did not return")
	}
	receipt, err := ReadControlReceipt(c, request.RequestID)
	require(t, err)
	if receipt.Status != "failed" || receipt.Code != "unsupported" || len(s.domain.Executions) != 1 || s.find(request.RequestID).WorkflowStage != "start.failed" {
		t.Fatalf("unsupported start lost its candidate or failure receipt: receipt=%+v executions=%d", receipt, len(s.domain.Executions))
	}
}

func TestTaskStartControlRecoversReservationsWithoutReplay(t *testing.T) {
	s, c := newTaskStartFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := startControlRequest(t, s, "101", "first")
	require(t, s.pollControl(ctx, cancel))
	second := startControlRequest(t, s, "105", "second")
	s.controlFault = func(point string) {
		if point == "after_start_reservation" {
			panic("crash after atomic claim, before applying receipt")
		}
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("reservation crash was not injected")
			}
		}()
		_ = s.pollControl(ctx, cancel)
	}()
	require(t, s.refreshNativeSnapshot(ctx))
	a, b := s.find(first.RequestID), s.find(second.RequestID)
	if a == nil || b == nil {
		t.Fatal("both independent claims must survive the crash")
	}
	firstExecution, secondExecution := a.ExecutionID, b.ExecutionID
	require(t, s.Close())
	restarted, err := newNeutralService(c)
	require(t, err)
	t.Cleanup(func() { _ = restarted.Close() })
	restarted.lifecycleResolver = mcResolver{}
	require(t, restarted.adoptOrGateServiceExecutions())
	restarted.State.Service.Ready = true
	require(t, restarted.pollControl(ctx, cancel))
	recovered, err := ReadControlReceipt(c, second.RequestID)
	require(t, err)
	if recovered.Status != "applying" || recovered.RunID != second.RequestID || recovered.InstanceID != second.InstanceID || restarted.State.Service.InstanceID == second.InstanceID {
		t.Fatalf("committed start did not recover under its original request: %+v", recovered)
	}
	// Receipt-only reconciliation never creates worker resources.
	require(t, restarted.pollControl(ctx, cancel))
	if len(restarted.domain.Executions) != 2 || restarted.find(first.RequestID).ExecutionID != firstExecution || restarted.find(second.RequestID).ExecutionID != secondExecution || len(restarted.workers) != 0 {
		t.Fatal("request replay replaced an original execution")
	}
	for _, req := range []productapi.ControlRequest{first, second} {
		run := restarted.find(req.RequestID)
		require(t, restarted.prepareReservedStart(ctx, run))
		run = restarted.find(req.RequestID)
		mcStartTurn(t, restarted, run)
		mcFinishTurn(t, restarted, run)
		require(t, restarted.pollTaskControlReceipts(ctx))
		r, err := ReadControlReceipt(c, req.RequestID)
		require(t, err)
		if r.Status != "applied" || r.RunID != req.RequestID {
			t.Fatalf("recovered worker did not settle its original request: %+v", r)
		}
	}
	if len(restarted.domain.Executions) != 2 {
		t.Fatal("recovery allocated replacement executions")
	}
}

func TestTaskStartControlRejectsAuthorityChangedDuringPreflight(t *testing.T) {
	s, c := newTaskStartFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	s.runContext, s.signalQueue = ctx, make(chan Signal, 8)
	t.Cleanup(func() { cancel(); s.shutdown() })
	gate := &startGate{entered: make(chan struct{}), release: make(chan struct{})}
	s.taskWork = slowStartWork{TaskWorkPort: s.taskWork, gate: gate}
	req := startControlRequest(t, s, "101", "first")
	require(t, s.pollControl(ctx, cancel))
	select {
	case <-gate.entered:
	case <-time.After(30 * time.Second):
		t.Fatal("preflight never entered")
	}
	authority, err := s.commandStore.NativeAuthority(ctx, c.Identity())
	require(t, err)
	paused := true
	_, err = s.commandService(c.Identity()).SetNativeDispatchState(ctx, NativeDispatchUpdate{RequestID: newID(), ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Paused: &paused})
	require(t, err)
	require(t, s.refreshNativeSnapshot(ctx))
	close(gate.release)
	select {
	case message := <-s.signalQueue:
		require(t, s.HandleSignal(message))
	case <-time.After(30 * time.Second):
		t.Fatal("preflight did not return")
	}
	r, err := ReadControlReceipt(c, req.RequestID)
	require(t, err)
	if r.Status != "failed" || len(s.domain.Executions) != 1 || s.find(req.RequestID).WorkflowStage != "start.failed" || ctx.Err() != nil {
		t.Fatalf("changed authority lost its failed candidate: %+v", r)
	}
}

func TestTaskStartControlKeepsUncertainEffectsScopedToOriginalTask(t *testing.T) {
	s, c := newTaskStartFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := startControlRequest(t, s, "101", "first")
	require(t, s.pollControl(ctx, cancel))
	run := s.find(first.RequestID)
	input, err := workerWorkspaceRequest(run)
	require(t, err)
	_, err = s.startWorkerOperation(run, "prepare_workspace", "worker-workspace:"+run.ExecutionID, input)
	require(t, err)
	s.markOneServiceRecoveryGate(run)
	if !s.executionRecord(run).RecoveryRequired || s.pristineTaskStartReservation(run) {
		t.Fatal("an uncertain original worker effect was treated as a pristine claim")
	}
	require(t, s.commitWorker(run))
	require(t, s.pollTaskControlReceipts(ctx))
	r, err := ReadControlReceipt(c, first.RequestID)
	require(t, err)
	if r.Status != "indeterminate" {
		t.Fatalf("unknown original effect was reported as a started worker: %+v", r)
	}
	second := startControlRequest(t, s, "105", "second")
	require(t, s.pollControl(ctx, cancel))
	r, err = ReadControlReceipt(c, second.RequestID)
	require(t, err)
	if r.Status != "applying" || len(s.domain.Executions) != 2 || s.State.Control.Operation != nil || len(s.State.Control.Blockers) != 0 {
		t.Fatal("unknown task 101 blocked independent task 105 or replayed its effect")
	}
}
