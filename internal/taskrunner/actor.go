package taskrunner

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/hostport"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type ActorStore interface {
	Store
	ReadRunnerSnapshot(context.Context, string) (storageports.RunnerSnapshot, error)
	SaveCheckpoint(context.Context, hostcontract.ServiceCheckpoint) (hostcontract.ServiceCheckpoint, error)
}
type ActorOptions struct {
	RunnerBuild           *runnercontract.BuildPin
	NativeSessionAdapters []string
	Tasks                 *Service
	Store                 ActorStore
	Host                  hostport.HostPort
	Repository            hostcontract.Repository
	Mode, ConfigDigest    string
	Heartbeat             time.Duration
	// Control validates, fixes and commits a product request; it cannot invoke
	// native adapters. App supplies its Task/Runner CLI command coordinator.
	Control func(context.Context, api.ControlRequest) (api.ControlReceipt, error)
}

type actorCall struct {
	run    func(context.Context) error
	result chan error
}
type Actor struct {
	// ObserveAuxiliary observes caller-owned non-Task operations on the existing
	// runtime tick; it never manages an agent process or dispatches Task work.
	ObserveAuxiliary func(context.Context) error
	State            reconstruction.Runtime
	options          ActorOptions
	mu               sync.Mutex
	ctx              context.Context
	calls            chan actorCall
	workMu           sync.Mutex
	workContext      context.Context
	cancelWork       context.CancelFunc
	controlPending   bool
	wakeup           chan struct{}
}

func NewActor(options ActorOptions) (*Actor, error) {
	if options.Tasks == nil || options.Store == nil || options.Host == nil || options.Control == nil || options.Repository.Identity != options.Tasks.Project || options.Mode != "manual" && options.Mode != "auto" || options.Heartbeat <= 0 {
		return nil, errors.New("task service requires its fixed caller and control ports")
	}
	identity, err := options.Host.CurrentProcessIdentity()
	if err != nil {
		return nil, err
	}
	if identity == nil {
		return nil, errors.New("service process identity is unavailable")
	}
	at := time.Now().UTC()
	checkpoint := hostcontract.ServiceCheckpoint{SchemaVersion: hostcontract.ServiceCheckpointSchema, Repository: options.Repository, UpdatedAt: at, HeartbeatAt: at,
		Service: hostcontract.ServiceState{RunnerBuild: options.RunnerBuild, Mode: options.Mode, InstanceID: id(), PID: identity.PID, Status: "starting", Claimant: options.Tasks.Claimant, ProcessIdentity: identity, ConfigDigest: options.ConfigDigest, ControlProtocol: 1, Capabilities: []string{"task_dag", "runner_cli", "start_task", "start_candidate", "stop_candidate", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "runner_reply", "runner_resume", "answer_decision"}, HeartbeatIntervalMS: options.Heartbeat.Milliseconds()}}
	viewerAvailable := false
	for _, adapter := range options.NativeSessionAdapters {
		if adapter == "codex-app-server" || adapter == "orca-terminal" {
			viewerAvailable = true
			checkpoint.Service.Capabilities = append(checkpoint.Service.Capabilities, "open_native_session:"+adapter)
		}
	}
	if viewerAvailable {
		checkpoint.Service.Capabilities = append(checkpoint.Service.Capabilities, "open_native_session")
	}
	return &Actor{State: reconstruction.RuntimeFromCheckpoint(checkpoint, options.Mode, true, 0), options: options, calls: make(chan actorCall), wakeup: make(chan struct{}, 1)}, nil
}

func (a *Actor) ControlWakeup() <-chan struct{} { return a.wakeup }

// NotifyControl only interrupts caller-side waiting. The actor still validates
// the immutable inbox before withdrawing any candidate.
func (a *Actor) NotifyControl() {
	a.workMu.Lock()
	a.controlPending = true
	if a.cancelWork != nil {
		a.cancelWork()
	}
	a.workMu.Unlock()
	select {
	case a.wakeup <- struct{}{}:
	default:
	}
}

func (a *Actor) RunWork(ctx context.Context, run func(context.Context) error) error {
	work, cancel := context.WithCancel(ctx)
	a.workMu.Lock()
	a.workContext, a.cancelWork = work, cancel
	if a.controlPending {
		cancel()
	}
	a.workMu.Unlock()
	defer func() {
		cancel()
		a.workMu.Lock()
		a.workContext, a.cancelWork = nil, nil
		a.workMu.Unlock()
	}()
	return run(work)
}

func (a *Actor) InstanceID() string { return a.State.Service.InstanceID }

// VerifyRunnerAdmission reuses the application's pinned Runner/config gate
// without claiming a Task, starting a daemon or changing actor state.
func (a *Actor) VerifyRunnerAdmission(ctx context.Context) error {
	if a.options.Tasks.Admit == nil {
		return errors.New("original Runner instance admission is unavailable")
	}
	return a.options.Tasks.Admit(ctx)
}
func (a *Actor) BeginActor(ctx context.Context) error {
	a.ctx = ctx
	return a.options.Tasks.RecoverInputs(ctx)
}
func (a *Actor) RecoverAdmission() (bool, error) { return true, nil }
func (a *Actor) Initialize(ctx context.Context, _ context.CancelFunc) error {
	if a.options.Tasks.Admit != nil {
		if err := a.options.Tasks.Admit(ctx); err != nil {
			return err
		}
	}
	a.mu.Lock()
	a.State.Service.Status = "running"
	a.State.Service.Ready = true
	a.mu.Unlock()
	if err := a.publish(ctx); err != nil {
		return err
	}
	return nil
}
func (a *Actor) Signals() <-chan actorCall   { return a.calls }
func (a *Actor) HistoryErrors() <-chan error { return nil }
func (a *Actor) WaitSources()                {}
func (a *Actor) HandleSignal(call actorCall) error {
	ctx := a.workContext
	if ctx == nil {
		ctx = a.ctx
	}
	call.result <- call.run(ctx)
	return nil
}
func (a *Actor) HandleHistoryError(_ context.Context, err error) error { return err }
func (a *Actor) PollHistory(context.Context)                           {}

// Call serializes service-owned HTTP commands with actor ticks. Cancellation
// after dispatch never guesses whether its durable command was committed.
func (a *Actor) Call(ctx context.Context, run func(context.Context) error) error {
	if a.ctx == nil {
		return errors.New("task service actor has not started")
	}
	call := actorCall{run: run, result: make(chan error, 1)}
	select {
	case a.calls <- call:
	case <-ctx.Done():
		return ctx.Err()
	case <-a.ctx.Done():
		return errors.New("task service stopped before command dispatch")
	}
	select {
	case err := <-call.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-a.ctx.Done():
		return errors.New("task service command outcome is unverified; inspect its original ID")
	}
}
func (a *Actor) TickDecision(ctx context.Context) error {
	// Publish live candidates before spending time on auxiliary records. A failed
	// auxiliary query must never suppress this tick's Runner observation.
	reconcileErr := a.options.Tasks.Reconcile(ctx)
	if err := a.observeError(ctx, reconcileErr); err != nil {
		return err
	}
	if a.ObserveAuxiliary != nil {
		auxiliary, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := a.ObserveAuxiliary(auxiliary)
		cancel()
		return a.observeError(ctx, errors.Join(reconcileErr, err))
	}
	return nil
}
func (a *Actor) TickTask(ctx context.Context, _ context.CancelFunc) error {
	snapshot, err := a.options.Store.ReadRunnerSnapshot(ctx, a.options.Tasks.Project)
	if err != nil {
		return err
	}
	if snapshot.Graph.Authority.DispatchMode != "auto" || snapshot.Graph.Authority.DispatchPaused {
		return nil
	}
	return a.observeError(ctx, a.options.Tasks.Dispatch(ctx))
}
func (a *Actor) TickControl(ctx context.Context, _ context.CancelFunc) error {
	a.workMu.Lock()
	a.controlPending = false
	a.workMu.Unlock()
	ids, err := a.options.Host.ControlRequestIDs()
	if err != nil {
		return err
	}
	var requests []api.ControlRequest
	for _, id := range ids {
		if receipt, err := a.options.Host.ReadControlReceipt(id); err == nil && receipt != nil {
			continue
		}
		request, err := a.options.Host.ReadControlRequest(id)
		if err != nil {
			continue
		}
		if request.Action == "stop_service" {
			continue
		}
		requests = append(requests, request)
	}
	sort.SliceStable(requests, func(i, j int) bool {
		return requests[i].Action == "stop_candidate" && requests[j].Action != "stop_candidate"
	})
	var failures []error
	for _, request := range requests {
		a.workMu.Lock()
		urgent := a.controlPending
		a.workMu.Unlock()
		if urgent && request.Action != "stop_candidate" {
			break // let the scheduler reload the newly published stop first
		}
		id := request.RequestID
		requestFileDigest := a.options.Host.ControlRequestDigest(id)
		if !validControlDigest(requestFileDigest) {
			failures = append(failures, errors.New("original host control request digest is unavailable"))
			continue
		}
		var receipt api.ControlReceipt
		var err error
		if request.Action == "stop_candidate" {
			budget, cancel := context.WithTimeout(ctx, 5*time.Second)
			receipt, err = a.options.Control(budget, request)
			cancel()
		} else {
			err = a.RunWork(ctx, func(work context.Context) error {
				var e error
				receipt, e = a.options.Control(work, request)
				return e
			})
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if a.options.Host.ControlRequestDigest(id) != requestFileDigest {
			failures = append(failures, errors.New("host control request changed while applying its original command"))
			continue
		}
		receipt.RequestDigest = requestFileDigest
		if err := a.options.Host.WriteControlReceipt(receipt); err != nil {
			failures = append(failures, err)
		}
	}
	return a.observeError(ctx, errors.Join(failures...))
}
func (a *Actor) TickPR(ctx context.Context) error {
	snapshot, err := a.options.Store.ReadRunnerSnapshot(ctx, a.options.Tasks.Project)
	if err != nil {
		return err
	}
	delivered := map[string]bool{}
	var failures []error
	for _, c := range snapshot.Candidates {
		if ctx.Err() != nil {
			break
		}
		task := snapshot.Graph.ByID[c.TaskID]
		if delivered[c.TaskID] || task.Status != "in_progress" || c.StopRequested || c.Participation != "active" || c.Projection == nil || c.Projection.PR == nil {
			continue
		}
		_, err := a.options.Tasks.Deliver(ctx, c.TaskID, c.ID, task.Version)
		if err == nil {
			delivered[c.TaskID] = true
			continue
		}
		var domain *taskmodel.Error
		if errors.As(err, &domain) && domain.Code == "merge_state_unknown" {
			continue
		}
		failures = append(failures, err)
	}
	return a.observeError(ctx, errors.Join(failures...))
}
func (a *Actor) observeError(ctx context.Context, err error) error {
	a.mu.Lock()
	a.State.Service.Error = nil
	if err != nil {
		message := err.Error()
		a.State.Service.Error = &message
	}
	a.mu.Unlock()
	if a.ctx != nil {
		ctx = a.ctx
	}
	return a.publish(ctx)
}
func (a *Actor) publish(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	snapshot, err := a.options.Store.ReadRunnerSnapshot(ctx, a.options.Tasks.Project)
	if err != nil {
		return err
	}
	a.State.UpdatedAt = time.Now().UTC()
	checkpoint, err := a.options.Store.SaveCheckpoint(ctx, reconstruction.Checkpoint(a.State))
	if err != nil {
		return err
	}
	a.State = Project(snapshot, checkpoint)
	return nil
}
func (a *Actor) Heartbeat() error {
	a.mu.Lock()
	a.State.HeartbeatAt = time.Now().UTC()
	a.mu.Unlock()
	return a.publish(a.ctx)
}

// ExecuteTaskMutation uses the same caller actor as candidate and DAG updates.
// Once dispatched, cancellation leaves the original durable receipt queryable.
func (a *Actor) ExecuteTaskMutation(ctx context.Context, run func(context.Context) error) error {
	return a.Call(ctx, func(work context.Context) error {
		a.mu.Lock()
		ready := a.State.Service.Ready
		a.mu.Unlock()
		if !ready || a.ctx.Err() != nil {
			return errors.New("project actor is not accepting task commands")
		}
		err := run(work)
		return errors.Join(err, a.publish(a.ctx))
	})
}

func (a *Actor) Discard(ctx context.Context, requestID string, intent taskmodel.DiscardIntent) (taskmodel.TaskDiscardReceipt, error) {
	var receipt taskmodel.TaskDiscardReceipt
	err := a.Call(ctx, func(work context.Context) error {
		store, ok := a.options.Store.(DiscardStore)
		if !ok {
			return errors.New("task discard store is unavailable")
		}
		var err error
		receipt, err = a.options.Tasks.Discard(work, store, requestID, intent)
		return err
	})
	return receipt, err
}

// EndActor commits local caller state. ProjectRuntime owns Runner and storage.
func (a *Actor) EndActor(runErr error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a.mu.Lock()
	a.State.Service.Ready = false
	a.State.Service.Status = "stopped"
	a.mu.Unlock()
	return errors.Join(runErr, a.publish(ctx))
}
