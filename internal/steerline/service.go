package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
)

type Service struct {
	Config               Config
	baseConfig           Config
	lifecycleResolver    executionapi.LifecycleResolver
	conversationSources  contracts.ConversationSourcePort
	dispatchConfig       *Config
	dispatchProfile      *ExecutionProfile
	State                Runtime
	domain               DomainSnapshot
	committed            DomainSnapshot
	runs                 []*Run
	pendingEvents        []DomainEvent
	pendingOperations    map[string]string
	workerDecisionWrites map[string]bool
	store                *Store
	domainStore          centerReader
	commandStore         persistence.CommandStore
	conversationStore    persistence.Conversations
	closeStore           func() error
	// workers owns live observers by authoritative workflow run ID.
	workers                    map[string]*Worker
	ownedProcesses             map[string]executionapi.StopHandle
	lifecycleEffects           map[string]pendingLifecycleEffect
	lifecycleMessages          map[string][]workerMessage
	appServerRecovering        map[string]bool
	workerMessages             chan workerMessage
	runContext                 context.Context
	forwarders                 sync.WaitGroup
	forwardMu                  sync.Mutex
	forwardedDone              []workerMessage
	conversationReplayRequests chan []conversationJournalSource
	conversationReplayErrors   chan error
	// Console selects service-side rendering: "", "off", "pretty", "summary" or "json".
	Console           string
	consoleOut        io.Writer
	consoleSequence   uint64
	controlSeen       map[string]string
	stopControl       *ControlRequest
	stopAcceptedAt    *time.Time
	targetTask        string
	lastDispatched    string
	reservedRunID     string
	candidateSnapshot *TaskSnapshot
	explicitDispatch  bool
	controlFault      func(string)
	startupID         string
}

// NewService constructs a service from one explicit, immutable implementation
// directory and its conversation source port. Lifecycle operations receive
// only execution values and already-persisted operation intents. The application
// calls PrepareServiceConfig before opening and transferring the connection.
func NewService(c Config, directory *ExecutionDirectory, resolver executionapi.LifecycleResolver, conversationSources contracts.ConversationSourcePort, processHost executionapi.ManagedProcessHost, db persistence.Connection) (_ *Service, retErr error) {
	if directory == nil || resolver == nil || conversationSources == nil || processHost == nil || c.Persistence == nil || db == nil {
		return nil, errors.New("execution directory, conversation source, process host and persistence ports are required")
	}
	c.ExecutionDirectory = directory
	c.ProcessHost = processHost
	var admissionErr error
	c, admissionErr = admitServiceConfig(c)
	if admissionErr != nil {
		return nil, admissionErr
	}
	defer func() {
		if retErr != nil {
			_ = db.Close()
		}
	}()
	s := &Service{Config: c, baseConfig: c, lifecycleResolver: resolver, conversationSources: conversationSources, domainStore: db, commandStore: db, conversationStore: db, closeStore: db.Close, store: &Store{Domain: db, Project: c.identity}, workers: map[string]*Worker{}, ownedProcesses: map[string]executionapi.StopHandle{}}
	old, e := db.LoadRuntime(c.identity, false)
	if errors.Is(e, persistence.ErrNotFound) {
		old, e = nil, nil
	}
	if e != nil {
		return nil, e
	}
	s.State = Runtime{SchemaVersion: WorkerFlowRuntimeSchema, Repository: Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.identity}, Tasks: []TaskRecord{}, WorkflowRuns: []WorkflowRunRecord{}, Executions: []ExecutionRecord{}, Attempts: []StepAttemptRecord{}, Decisions: []DecisionRecord{}, Runs: []*Run{}, Control: ControlState{DispatchMode: "auto", Revision: 1, Blockers: []ControlBlocker{}}}
	authority, authErr := db.NativeAuthority(context.Background(), c.identity)
	if authErr != nil {
		return nil, authErr
	}
	if authority.DispatchMode == "auto" && c.DefaultProfileID() == "" {
		return nil, errors.New("automatic dispatch requires default_execution_profile")
	}
	s.State.Control = ControlState{DispatchMode: authority.DispatchMode, DispatchPaused: authority.DispatchPaused, Revision: authority.AuthorityVersion, Blockers: []ControlBlocker{}}
	if old != nil {
		if old.Repository.Identity != c.identity || old.Repository.Remote != c.Remote || old.Repository.Branch != c.Branch {
			return nil, nativeErr("authority_mismatch", "runtime repository or branch differs from the native authority; preserve state")
		}
		s.State = *old
		s.runs = old.Runs
		s.State.Control.DispatchMode, s.State.Control.DispatchPaused, s.State.Control.Revision = authority.DispatchMode, authority.DispatchPaused, authority.AuthorityVersion
		if op := s.State.Control.Operation; op != nil {
			if !controlIDPattern.MatchString(op.RequestID) || !validControlAction(op.Action) || (op.Phase != "applying" && op.Phase != "completed") || len(op.RequestDigest) != 64 || op.Action == "start_task" && (!controlIDPattern.MatchString(op.RunID) || op.TaskID == "") || op.Phase == "completed" && op.Result == nil {
				return nil, errors.New("invalid runtime control operation; preserve the snapshot for recovery")
			}
			if op.Phase == "completed" && (op.Result.Version != ControlProtocolVersion || op.Result.RequestID != op.RequestID || op.Result.RequestDigest != op.RequestDigest || op.Result.Action != op.Action || !terminalControlStatus(op.Result.Status)) {
				return nil, errors.New("completed control receipt does not match its operation")
			}
		}
		if s.State.Control.Operation == nil && s.State.CurrentTask != nil && s.find(*s.State.CurrentTask) == nil {
			// Automatic dispatch persists the reservation before fetch. With no run
			// record there has been no claim; clear only this pre-claim reservation.
			s.State.CurrentTask = nil
			s.State.SlotOccupied = false
		}
	}
	if err := s.refreshNativeSnapshot(context.Background()); err != nil {
		return nil, err
	}
	for _, run := range s.runs {
		if run.WorkflowVersion != WorkerFlowVersion {
			return nil, nativeErr("unsupported_schema", "unsupported workflow version")
		}
		if _, err := s.executionConfigForRun(run); err != nil {
			return nil, err
		}
	}
	s.State.SchemaVersion = WorkerFlowRuntimeSchema
	capabilities := []string{"answer_decision", "open_terminal", "recheck_environment", "start_task", "start_candidate", "abandon_candidate", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service"}
	s.State.Service = ServiceState{ExecutionPolicy: c.ExecutionPolicy, PolicySource: c.policySource, InstanceID: newID(), PID: os.Getpid(), Status: "running", Claimant: c.Claimant, Mode: "maintenance", Ready: false, ControlProtocol: ControlProtocolVersion, Capabilities: capabilities, ConfigDigest: c.configDigest, HeartbeatIntervalMS: duration(c.Heartbeat).Milliseconds()}
	identity, err := currentProcessIdentity()
	if err != nil {
		return nil, fmt.Errorf("identify service process: %w", err)
	}
	s.State.Service.ProcessIdentity = identity
	if err := db.ValidateConversationCheckpointFormat(context.Background(), c.identity, conversationSources.JournalSources()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Close() error {
	if s == nil || s.closeStore == nil {
		return nil
	}
	return s.closeStore()
}

func (s *Service) publishCheckpoint() error {
	if err := s.projectState(); err != nil {
		return err
	}
	checkpoint := runtimeCheckpoint(s.State)
	if err := s.store.Publish(&checkpoint); err != nil {
		return err
	}
	s.State.Revision, s.State.UpdatedAt, s.State.HeartbeatAt = checkpoint.Revision, checkpoint.UpdatedAt, checkpoint.HeartbeatAt
	return s.refreshNativeSnapshot(context.Background())
}

// note queues a canonical event for the next domain transaction.
func (s *Service) note(r *Run, source, kind, status, title, detail string, meta map[string]any) {
	ev := ActivityEvent{Source: source, Kind: kind, Status: status, Title: title, Detail: detail, Meta: meta}
	s.recordActivity(r, ev)
	s.emitConsole(ev)
}

func (s *Service) recordActivity(r *Run, ev ActivityEvent) {
	if r == nil {
		return
	}
	ev.V, ev.RunID, ev.TaskID = ActivityVersion, r.RunID, r.TaskID
	if ev.TS.IsZero() {
		ev.TS = now()
	}
	metadata := map[string]any{}
	for key, value := range ev.Meta {
		metadata[key] = value
	}
	metadata["attempt_id"], metadata["execution_id"] = r.AttemptID, r.ExecutionID
	ev.Meta = metadata
	payload, err := json.Marshal(ev)
	if err != nil {
		s.setError(err)
		return
	}
	domainTask, executionID := r.DomainTaskID, r.ExecutionID
	if domainTask == "" || executionID == "" || !executionapi.StableExecutionID(r.RunID) || !executionapi.StableExecutionID(r.AttemptID) {
		s.setError(nativeErr("unsupported_schema", "native activity lacks its fixed task, run, execution or attempt identity"))
		return
	}
	externalID := newID()
	if ev.ItemID != "" {
		externalID = "activity-" + r.AttemptID + "-" + operationDigest([]byte(ev.Source + "\x00" + ev.ItemID))[:24]
	}
	s.pendingEvents = append(s.pendingEvents, DomainEvent{ProjectIdentity: s.Config.identity, TaskID: domainTask, WorkflowRunID: r.RunID, ExecutionID: executionID, AttemptID: r.AttemptID, ExternalSource: "center:" + r.RunID, ExternalID: externalID, Type: "activity." + ev.Kind, ReceivedAt: ev.TS, Payload: payload})
}

func (s *Service) emitConsole(ev ActivityEvent) {
	if s.Console == "" || s.Console == "off" {
		return
	}
	out := s.consoleOut
	if out == nil {
		out = os.Stdout
	}
	switch s.Console {
	case "json":
		// The console combines service events with several per-run journals.
		// Give that output stream its own sequence without changing the journals.
		s.consoleSequence++
		ev.V, ev.Seq = ActivityVersion, s.consoleSequence
		if ev.TS.IsZero() {
			ev.TS = now()
		}
		b, e := json.Marshal(ev)
		if e == nil {
			fmt.Fprintln(out, string(b))
		}
	case "summary":
		if ev.Source == "steer" {
			fmt.Fprintln(out, RenderActivity(ev, RenderOptions{Color: false}))
		}
	default:
		fmt.Fprintln(out, RenderActivity(ev, RenderOptions{Color: stdoutColor()}))
	}
}
func (s *Service) find(id string) *Run {
	var selected *Run
	for _, r := range s.runs {
		if r != nil && r.RunID == id {
			return r
		}
		if r != nil && r.TaskID == id {
			if selected != nil {
				return nil
			}
			selected = r
		}
	}
	return selected
}

func (s *Service) findExecution(id string) *Run {
	for _, run := range s.runs {
		if run != nil && run.ExecutionID == id {
			return run
		}
	}
	return nil
}
func (s *Service) reconcile(ctx context.Context) error {
	if err := s.refreshNativeSnapshot(ctx); err != nil {
		return err
	}
	if err := s.reconcileNativeCleanup(ctx); err != nil {
		return err
	}
	return nil
}
func (s *Service) setError(e error) {
	s.State.Service.Status = "error"
	s.State.Service.Error = ptr(clip(e.Error(), 2000))
}
func (s *Service) dispatch(ctx context.Context) error {
	return s.dispatchNative(ctx)
}

func (s *Service) injectControlFault(point string) {
	if s.controlFault != nil {
		s.controlFault(point)
	}
}

func (s *Service) activeControlRequestID() string {
	if s.explicitDispatch && s.State.Control.Operation != nil {
		return s.State.Control.Operation.RequestID
	}
	return ""
}

func (s *Service) slotReason(ignoreRequestID string) string {
	return s.executionSlotReason(ignoreRequestID, false)
}

func (s *Service) executionSlotReason(ignoreRequestID string, resumingDecision bool) string {
	if len(s.workers) != 0 {
		return "worker is running"
	}
	if operation := s.State.Control.Operation; operation != nil && operation.RequestID != ignoreRequestID {
		if operation.Phase == "completed" {
			return "terminal control receipt is pending publication"
		}
		return "control operation is applying"
	}
	if len(s.State.Control.Blockers) > 0 {
		return s.State.Control.Blockers[0].Reason
	}
	for _, run := range s.runs {
		if run == nil {
			continue
		}
		if run.ExecutionStatus == "unknown" {
			return "unresolved unknown execution; manual recovery required"
		}
		if run.ExecutionStatus == "pending" || run.ExecutionStatus == "running" {
			return "execution preparation or supervision is still active"
		}
	}
	for _, run := range s.runs {
		if run != nil && !resumingDecision && run.FinishTaskStatus == "waiting_decision" {
			return "waiting for a human decision"
		}
	}
	if s.State.SlotOccupied && s.State.CurrentTask != nil && s.find(*s.State.CurrentTask) == nil {
		return "execution preparation is still active"
	}
	return ""
}

func (s *Service) refreshSlotOccupancy() {
	projectSlot(&s.State)
	s.State.SlotOccupied = s.State.SlotOccupied || len(s.workers) != 0
}
func (s *Service) refreshCandidates(ctx context.Context) error {
	queried := now()
	tasks, err := (NewQueryService(s.domainStore, s.Config.identity)).ReadyNativeTasks(ctx)
	if err != nil {
		return err
	}
	candidates := make([]TaskCandidate, 0, len(tasks))
	for _, task := range tasks {
		if !contains(s.Config.ExecutableTypes, task.Type) || s.find(task.TaskID) != nil {
			continue
		}
		candidates = append(candidates, TaskCandidate{TaskID: task.TaskID, Title: task.Title, Type: task.Type})
	}
	blockReason := ""
	if s.State.Control.DispatchMode == "auto" {
		blockReason = s.slotReason(s.activeControlRequestID())
	}
	s.candidateSnapshot = &TaskSnapshot{Version: 1, InstanceID: s.State.Service.InstanceID, RepositoryIdentity: s.State.Repository.Identity, QueriedAt: queried, LastSuccessAt: &queried, BlockReason: blockReason, Tasks: candidates}
	return nil
}
func decisionMeta(r *Run) map[string]any {
	if r.Decision == nil {
		return nil
	}
	opts := make([]map[string]string, 0, len(r.Decision.Request.Options))
	for _, o := range r.Decision.Request.Options {
		opts = append(opts, map[string]string{"id": o.ID, "label": o.Label})
	}
	m := map[string]any{"decision_id": r.Decision.ID, "options": opts}
	if r.Decision.Request.Recommended != "" {
		m["recommended"] = r.Decision.Request.Recommended
	}
	return m
}

func (s *Service) finishedForRun(run *Run, worker *Worker, err error) error {
	if worker != nil {
		for ev := range worker.Events {
			if eventErr := s.eventForRun(run, ev); eventErr != nil {
				s.setError(eventErr)
			}
		}
	}
	if run == nil || run.WorkflowVersion != WorkerFlowVersion {
		return nativeErr("unsupported_schema", "native service observed a run without the current worker workflow; preserve its execution identity")
	}
	if replayErr := s.reconcileConversationRun(run); replayErr != nil {
		s.setError(fmt.Errorf("completed conversation source replay failed: %w", replayErr))
	}
	return s.finishWorkerFlowFor(run, err)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func (s *Service) pollPRs(ctx context.Context) error {
	return s.pollPRsFor(ctx, "")
}
func (s *Service) pollPRsFor(ctx context.Context, runID string) error {
	return s.pollNativePRs(ctx, runID)
}
func (s *Service) eventForRun(run *Run, ev WorkerEvent) error {
	ack := func(err error) error {
		if ev.Processed != nil {
			ev.Processed <- err
			close(ev.Processed)
		}
		return err
	}
	if ev.RunID != "" && ev.AttemptID != "" && (run == nil || run.RunID != ev.RunID || run.AttemptID != ev.AttemptID) {
		err := s.recordHistoricalWorkerEvent(ev)
		ackWorkerEventPersistence(ev, err)
		return ack(err)
	}
	if run == nil {
		if ev.Persisted != nil {
			err := errors.New("conversation source event has no active run binding")
			ackWorkerEventPersistence(ev, err)
			return ack(err)
		}
		return ack(nil)
	}
	if run.WorkflowVersion != WorkerFlowVersion {
		return ack(nativeErr("unsupported_schema", "worker event belongs to an unsupported workflow"))
	}
	return ack(s.eventWorkerFlowFor(run, ev))
}

func ackWorkerEventPersistence(ev WorkerEvent, err error) {
	if ev.Persisted == nil {
		return
	}
	select {
	case ev.Persisted <- err:
	default:
	}
}

func (s *Service) recordHistoricalWorkerEvent(ev WorkerEvent) error {
	if !executionapi.StableExecutionID(ev.RunID) || !executionapi.StableExecutionID(ev.AttemptID) || ev.Activity.Kind == "" {
		if ev.Persisted != nil {
			return errors.New("conversation source identity is incomplete")
		}
		return nil
	}
	var run *Run
	for _, candidate := range s.runs {
		if candidate != nil && candidate.RunID == ev.RunID {
			run = candidate
			break
		}
	}
	if run == nil {
		if ev.Persisted != nil {
			return errors.New("conversation source run binding is unavailable")
		}
		return nil
	}
	// Keep late facts attached to their original attempt in the domain journal.
	attempt := (*StepAttemptRecord)(nil)
	for i := range s.domain.Attempts {
		if s.domain.Attempts[i].AttemptID == ev.AttemptID && s.domain.Attempts[i].ExecutionID == run.ExecutionID {
			attempt = &s.domain.Attempts[i]
			break
		}
	}
	if attempt == nil {
		if ev.Persisted != nil {
			return errors.New("conversation source attempt binding is unavailable")
		}
		return nil
	}
	if err := s.recordConversationEvent(run, ev); err != nil {
		return err
	}
	ev.Activity.V, ev.Activity.RunID, ev.Activity.TaskID = ActivityVersion, run.RunID, run.TaskID
	if ev.Activity.TS.IsZero() {
		ev.Activity.TS = now()
	}
	payload, err := json.Marshal(ev.Activity)
	if err != nil {
		return err
	}
	s.pendingEvents = append(s.pendingEvents, DomainEvent{ProjectIdentity: s.Config.identity, TaskID: run.DomainTaskID, WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: ev.AttemptID, ExternalSource: "center:" + run.RunID, ExternalID: newID(), Type: "activity." + ev.Activity.Kind, ReceivedAt: ev.Activity.TS, Payload: payload})
	return s.commitEvents()
}
func (s *Service) Run(ctx context.Context) (retErr error) {
	return s.RunWithDispatch(ctx, "auto")
}
func (s *Service) RunWithDispatch(ctx context.Context, mode string) error {
	if mode != "manual" && mode != "auto" {
		return errors.New("dispatch mode must be manual or auto")
	}
	return s.run(ctx, mode)
}

func (s *Service) run(ctx context.Context, dispatchMode string) (retErr error) {
	ctx, cancel := context.WithCancel(ctx)
	s.runContext = ctx
	s.workerMessages = make(chan workerMessage, 128)
	for runID, worker := range s.workers {
		s.forwardWorker(ctx, runID, worker)
	}
	var conversationReplayDone <-chan struct{}
	var historyCollectorDone <-chan struct{}
	defer func() {
		cancel()
		if conversationReplayDone != nil {
			<-conversationReplayDone
		}
		if historyCollectorDone != nil {
			<-historyCollectorDone
		}
		s.conversationReplayRequests = nil
		s.conversationReplayErrors = nil
	}()
	s.State.Service.Mode = "scheduler"
	if err := s.refreshNativeSnapshot(ctx); err != nil {
		return err
	}
	if err := s.adoptOrGateServiceExecutions(); err != nil {
		return err
	}
	s.State.Service.Ready = false
	s.State.Service.Status = "running"
	s.State.Service.HeartbeatIntervalMS = duration(s.Config.Heartbeat).Milliseconds()
	if e := s.publishCheckpoint(); e != nil {
		return e
	}
	s.conversationReplayRequests = make(chan []conversationJournalSource, 16)
	s.conversationReplayErrors = make(chan error, 16)
	replayDone := make(chan struct{})
	conversationReplayDone = replayDone
	replaySources := s.conversationJournalSources()
	go func() {
		defer close(replayDone)
		s.runConversationReplayQueue(ctx, s.conversationReplayRequests, s.conversationReplayErrors)
	}()
	if len(replaySources) > 0 {
		s.conversationReplayRequests <- replaySources
	}
	historyRequests := make(chan []contracts.ConversationHistorySource, 1)
	historyDone := make(chan struct{})
	historyCollectorDone = historyDone
	go func() {
		defer close(historyDone)
		s.runConversationHistoryCollector(ctx, historyRequests, s.conversationReplayErrors)
	}()
	historySources, err := s.discoverConversationHistory(ctx)
	if err != nil {
		return err
	}
	historyRequests <- historySources
	historyTick := time.NewTicker(2 * time.Second)
	defer historyTick.Stop()
	heartDone := make(chan struct{})
	acceptedStops := make(chan stopAcceptance, 1)
	instanceID := s.State.Service.InstanceID
	go func() {
		defer close(heartDone)
		t := time.NewTicker(duration(s.Config.Heartbeat))
		defer t.Stop()
		stopTick := time.NewTicker(500 * time.Millisecond)
		defer stopTick.Stop()
		stopSeen := map[string]bool{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopTick.C:
				stopReq, _, checkedAt := findStopRequestCached(s.Config, instanceID, stopSeen)
				if stopReq != nil {
					if stopReq != nil {
						acceptedStops <- stopAcceptance{Request: *stopReq, At: checkedAt}
					}
					cancel()
					return
				}
			case <-t.C:
				if e := s.store.Heartbeat(); e != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		cancel()
		<-heartDone
		if s.stopControl == nil {
			select {
			case accepted := <-acceptedStops:
				s.stopControl = &accepted.Request
				s.stopAcceptedAt = &accepted.At
			default:
			}
		}
		s.State.Service.Status = "stopping"
		s.State.Service.Ready = false
		if err := s.markServiceRecoveryGate(); err != nil {
			retErr = errors.Join(retErr, err)
		}
		s.publishCheckpoint()
		if e := s.ensureStopApplyingReceipt(); e != nil {
			retErr = errors.Join(retErr, e)
		}
		s.shutdown()
		if s.State.Control.Operation != nil {
			if e := s.recoverControlOperation(); e != nil {
				retErr = errors.Join(retErr, e)
			}
		}
		s.State.Service.Status = "stopped"
		if e := s.store.Err(); e != nil {
			retErr = e
		}
		if retErr != nil {
			s.setError(retErr)
		}
		if e := s.publishCheckpoint(); e != nil {
			retErr = e
		}
		if e := s.finalizeStopControl(); e != nil {
			retErr = errors.Join(retErr, e)
		}
	}()
	taskTick := time.NewTicker(duration(s.Config.TaskPoll))
	defer taskTick.Stop()
	prTick := time.NewTicker(duration(s.Config.PRPoll))
	defer prTick.Stop()
	decisionTick := time.NewTicker(2 * time.Second)
	defer decisionTick.Stop()
	controlTick := time.NewTicker(500 * time.Millisecond)
	defer controlTick.Stop()
	stepWithChange := func(fn func(context.Context) error, onlyWhenChanged bool) error {
		var before []byte
		var beforeDatabaseID string
		var beforeDatabaseRevision uint64
		if onlyWhenChanged {
			var snapshotErr, versionErr error
			before, snapshotErr = json.Marshal(s.State)
			beforeDatabaseID, beforeDatabaseRevision, versionErr = s.domainStore.DatabaseVersion(ctx)
			if snapshotErr != nil || versionErr != nil {
				onlyWhenChanged = false // Fall back to the existing durable publication path.
			}
		}
		e := fn(ctx)
		if s.store.Err() != nil {
			return s.store.Err()
		}
		if e != nil {
			s.setError(e)
		} else {
			s.State.Service.Status = "running"
			s.State.Service.Error = nil
		}
		if onlyWhenChanged && e == nil {
			after, snapshotErr := json.Marshal(s.State)
			databaseID, databaseRevision, versionErr := s.domainStore.DatabaseVersion(ctx)
			if snapshotErr == nil && versionErr == nil && bytes.Equal(before, after) &&
				beforeDatabaseID == databaseID && beforeDatabaseRevision == databaseRevision {
				return nil
			}
		}
		if e := s.publishCheckpoint(); e != nil {
			return e
		}
		if s.State.Service.Ready {
			return s.publishStartupReady()
		}
		return nil
	}
	step := func(fn func(context.Context) error) error { return stepWithChange(fn, false) }
	stepIfChanged := func(fn func(context.Context) error) error { return stepWithChange(fn, true) }
	for s.State.Control.Operation != nil {
		if e := s.recoverControlOperation(); e != nil {
			if e = s.retainControlPublicationFailure(e); e != nil {
				return e
			}
			select {
			case <-ctx.Done():
				return nil
			case <-controlTick.C:
			}
		}
	}
	// Recovery observes the original workers before accepting new input.
	if e := step(s.pollPRs); e != nil {
		return e
	}
	if e := s.reconcile(ctx); e != nil {
		s.setError(e)
		if saveErr := s.publishCheckpoint(); saveErr != nil {
			return saveErr
		}
	}
	if e := step(s.resumeConfiguredWork); e != nil {
		return e
	}
	if e := step(s.processCommandsAndDecisions); e != nil {
		return e
	}
	candidateErr := s.refreshCandidates(ctx)
	if candidateErr != nil {
		s.setError(candidateErr)
		if saveErr := s.publishCheckpoint(); saveErr != nil {
			return saveErr
		}
	}
	s.State.Service.Ready = false
	if e := s.publishCheckpoint(); e != nil {
		return e
	}
	if e := s.pollControl(ctx, cancel); e != nil {
		return e
	}
	s.State.Service.Ready = candidateErr == nil && ctx.Err() == nil && s.State.Control.Operation == nil
	if e := s.publishCheckpoint(); e != nil {
		return e
	}
	if e := s.publishStartupReady(); e != nil {
		return e
	}
	if s.State.Control.Operation == nil {
		if e := step(s.dispatch); e != nil {
			return e
		}
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-historyTick.C:
			historySources, discoverErr := s.discoverConversationHistory(ctx)
			if discoverErr != nil {
				select {
				case s.conversationReplayErrors <- discoverErr:
				default:
				}
				continue
			}
			select {
			case historyRequests <- historySources:
			default:
			}
		case replayErr := <-s.conversationReplayErrors:
			if replayErr != nil {
				if e := step(func(context.Context) error { return replayErr }); e != nil {
					return e
				}
			}
		case message := <-s.workerMessages:
			if err := s.handleWorkerMessage(message); err != nil && !errors.Is(err, ErrWorkerTurnStale) {
				return err
			}
		case <-decisionTick.C:
			if s.State.Control.Operation != nil {
				continue
			}
			if e := stepIfChanged(s.processCommandsAndDecisions); e != nil {
				return e
			}
		case <-taskTick.C:
			if e := stepIfChanged(s.processCommandsAndDecisions); e != nil {
				return e
			}
			if e := stepIfChanged(s.reconcile); e != nil {
				return e
			}
			if e := s.pollControl(ctx, cancel); e != nil {
				return e
			}
			if s.State.Control.Operation != nil {
				continue
			}
			if e := stepIfChanged(s.resumeConfiguredWork); e != nil {
				return e
			}
			if len(s.workers) != 0 && s.State.Control.DispatchMode == "auto" {
				continue
			}
			if e := stepIfChanged(s.dispatch); e != nil {
				return e
			}
		case <-controlTick.C:
			if e := stepIfChanged(s.processPendingCommands); e != nil {
				return e
			}
			if e := s.pollControl(ctx, cancel); e != nil {
				return e
			}
		case <-prTick.C:
			if s.State.Control.Operation != nil {
				continue
			}
			if e := stepIfChanged(s.pollPRs); e != nil {
				return e
			}
		}
	}
}

func (s *Service) finishWorkerInLoop(run *Run, worker *Worker, workerErr error) error {
	return s.finishedForRun(run, worker, workerErr)
}

func (s *Service) shutdown() {
	defer func() {
		for runID, owned := range s.ownedProcesses {
			s.interruptOwnedLocalTurn(runID, owned)
			owned.Close()
			delete(s.ownedProcesses, runID)
		}
	}()
	if s.runContext != nil && s.runContext.Err() != nil {
		s.forwarders.Wait()
		s.drainWorkerMessages()
	}
	for _, messages := range s.lifecycleMessages {
		for _, message := range messages {
			if message.event != nil {
				rejectUndeliveredWorkerEvent(*message.event, context.Canceled)
			}
		}
	}
	s.lifecycleMessages, s.lifecycleEffects = nil, nil
	// The actor has stopped. Final owned-resource work still uses the same
	// background result path, awaited here because no message loop remains.
	s.workerMessages = nil
	for len(s.workers) != 0 {
		var runID string
		for id := range s.workers {
			runID = id
			break
		}
		var run *Run
		for _, candidate := range s.runs {
			if candidate != nil && candidate.RunID == runID {
				run = candidate
				break
			}
		}
		if run == nil {
			s.setError(errors.New("active worker has no fixed workflow run"))
			return
		}
		config, err := s.executionConfigForRun(run)
		if err != nil {
			s.setError(err)
			return // Preserve the original worker identity for restart reconciliation.
		}
		stoppingWorker := s.workers[runID]
		ctx, cancel := context.WithTimeout(context.Background(), duration(config.ShutdownGrace))
		err = stoppingWorker.detachLocalObservation(ctx)
		cancel()
		if err != nil {
			s.setError(fmt.Errorf("original worker observation could not be detached: %w", err))
			return
		}
		if s.workers[runID] == stoppingWorker {
			delete(s.workers, runID)
		}
	}
}
