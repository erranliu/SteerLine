package steerline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Service struct {
	Config               Config
	baseConfig           Config
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
	domainStore          *SQLiteStore
	prs                  *PRClient
	// workers owns live observers by authoritative workflow run ID.
	workers             map[string]*Worker
	ownedLocalProcesses map[string]*childProcess
	appServerLaunching  map[string]string // In-flight goroutines, never persisted execution authority.
	appServerRecovering map[string]bool
	workerMessages      chan workerMessage
	runContext          context.Context
	forwarders          sync.WaitGroup
	forwardMu           sync.Mutex
	forwardedDone       []workerMessage
	// activities holds one journal per run so PR and delivery events always land in
	// their own run's file, even after the worker exited.
	activities                 map[string]*ActivityWriter
	conversationMappers        map[string]*ConversationMapper
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

func NewService(c Config) (_ *Service, retErr error) {
	if err := requireNativeTaskAuthority(c); err != nil {
		return nil, err
	}
	if c.WorkflowProtocol != WorkerFlowVersion {
		return nil, nativeErr("unsupported_schema", "only worker-flow-v1 may start a service; preserve older execution state")
	}
	if err := validateWorkflowConfig(&c, nil); err != nil {
		return nil, err
	}
	if err := validateExecutionProfiles(c); err != nil {
		return nil, err
	}
	if err := c.ResolveIdentity(context.Background()); err != nil {
		return nil, err
	}
	if err := validateNativeStateLocation(c); err != nil {
		return nil, err
	}
	if err := preflightNativeStatePath(c); err != nil {
		return nil, err
	}
	if err := preflightWorkerFlowState(context.Background(), c); err != nil {
		return nil, err
	}
	if e := c.prepareWorktreeRoot(); e != nil {
		return nil, e
	}
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	db, e := OpenNativeTaskCommandStore(context.Background(), &c)
	if e != nil {
		return nil, e
	}
	defer func() {
		if retErr != nil {
			_ = db.Close()
		}
	}()
	s := &Service{Config: c, baseConfig: c, domainStore: db, store: &Store{Domain: db, Project: c.identity}, prs: &PRClient{Config: c.PR, Runner: r}, activities: map[string]*ActivityWriter{}, conversationMappers: map[string]*ConversationMapper{}, workers: map[string]*Worker{}, ownedLocalProcesses: map[string]*childProcess{}}
	old, e := loadRuntimeSnapshot(db, c.identity, false)
	if errors.Is(e, sql.ErrNoRows) {
		old, e = nil, nil
	}
	if e != nil {
		return nil, e
	}
	s.State = Runtime{SchemaVersion: WorkerFlowRuntimeSchema, Repository: Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.identity}, Tasks: []TaskRecord{}, WorkflowRuns: []WorkflowRunRecord{}, Executions: []ExecutionRecord{}, Attempts: []StepAttemptRecord{}, Decisions: []DecisionRecord{}, Runs: []*Run{}, Control: ControlState{DispatchMode: "auto", Revision: 1, Blockers: []ControlBlocker{}}}
	authority, authErr := db.nativeAuthority(context.Background(), c.identity)
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
	capabilities := []string{"answer_decision", "start_task", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service"}
	s.State.Service = ServiceState{InstanceID: newID(), PID: os.Getpid(), Status: "running", Claimant: c.Claimant, Mode: "maintenance", Ready: false, ControlProtocol: ControlProtocolVersion, Capabilities: capabilities, ConfigDigest: c.configDigest, HeartbeatIntervalMS: duration(c.Heartbeat).Milliseconds()}
	identity, err := currentProcessIdentity()
	if err != nil {
		return nil, fmt.Errorf("identify service process: %w", err)
	}
	s.State.Service.ProcessIdentity = identity
	if err := db.ValidateConversationCheckpointFormat(context.Background(), c.identity); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Close() error {
	if s == nil || s.domainStore == nil {
		return nil
	}
	return s.domainStore.Close()
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

// activityWriter returns the lazily-opened journal for one run. Opening is
// idempotent and append-only, so a run keeps accumulating lifecycle events after
// its worker exits and after a service restart.
func (s *Service) activityWriter(r *Run) *ActivityWriter {
	if r == nil || r.LogPath == nil || r.RunID == "" {
		return nil
	}
	if s.activities == nil {
		s.activities = map[string]*ActivityWriter{}
	}
	if w, ok := s.activities[r.RunID]; ok {
		return w
	}
	path := activityPath(*r.LogPath)
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		s.setError(fmt.Errorf("activity log unavailable: %w", e))
		return nil
	}
	w, e := OpenActivity(path, r.RunID, r.TaskID)
	if e != nil {
		s.setError(fmt.Errorf("activity log unavailable: %w", e))
		return nil
	}
	s.activities[r.RunID] = w
	return w
}

func (s *Service) closeActivity(r *Run) {
	if r == nil {
		return
	}
	w, ok := s.activities[r.RunID]
	if !ok {
		return
	}
	if e := w.Close(); e != nil {
		s.setError(fmt.Errorf("activity log close: %w", e))
	}
	delete(s.activities, r.RunID)
}

func (s *Service) closeAllActivities() {
	for id, w := range s.activities {
		if e := w.Close(); e != nil {
			s.setError(fmt.Errorf("activity log close: %w", e))
		}
		delete(s.activities, id)
	}
}

// note queues a canonical event for the next domain transaction and mirrors it
// to the optional display journal.
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
	if w := s.activityWriter(r); w != nil {
		written, e := w.WriteEvent(ev)
		if e != nil {
			s.setError(e)
		} else {
			ev.Seq = written.Seq
		}
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		s.setError(err)
		return
	}
	domainTask, executionID := r.DomainTaskID, r.ExecutionID
	if domainTask == "" || executionID == "" || !stableExecutionID(r.RunID) || !stableExecutionID(r.AttemptID) {
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
	for _, r := range s.runs {
		if r.TaskID == id {
			return r
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
	tasks, err := (QueryService{Store: s.domainStore, Project: s.Config.identity}).ReadyNativeTasks(ctx)
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

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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
		if err == nil && ev.Acknowledge != nil {
			err = s.acknowledgeRemoteDelivery(run, ev)
		}
		if ev.Processed != nil {
			ev.Processed <- err
			close(ev.Processed)
		}
		return err
	}
	if len(ev.RemoteMessages) > 0 {
		if err := s.persistRemoteWorkerMessages(ev); err != nil {
			return ack(err)
		}
	}
	if ev.RunID != "" && ev.AttemptID != "" && (run == nil || run.RunID != ev.RunID || run.AttemptID != ev.AttemptID) {
		err := s.recordHistoricalWorkerEvent(ev)
		ackWorkerEventPersistence(ev, err)
		if err == nil && len(ev.RemoteMessages) > 0 {
			err = s.commitEvents()
		}
		return ack(err)
	}
	if run == nil {
		if ev.Persisted != nil {
			err := errors.New("conversation source event has no active run binding")
			ackWorkerEventPersistence(ev, err)
			return ack(err)
		}
		if len(ev.RemoteMessages) > 0 {
			return ack(s.commitEvents())
		}
		return ack(nil)
	}
	if run.WorkflowVersion != WorkerFlowVersion {
		return ack(nativeErr("unsupported_schema", "worker event belongs to an unsupported workflow"))
	}
	return ack(s.eventWorkerFlowFor(run, ev))
}

func (s *Service) acknowledgeRemoteDelivery(run *Run, ev WorkerEvent) error {
	if ev.Acknowledge == nil {
		return nil
	}
	if ev.RemoteRunID == "" || ev.RemoteDeliveryID == "" {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "remote Delivery acknowledgement lacks exact Run/Delivery identity"}
	}
	idempotencyKey := "orca-delivery-ack:" + ev.RemoteRunID + ":" + ev.RemoteDeliveryID
	request := mustJSON(struct {
		RunID      string `json:"run_id"`
		DeliveryID string `json:"delivery_id"`
	}{ev.RemoteRunID, ev.RemoteDeliveryID})
	digest := operationDigest(request)
	operationID := "ack-" + operationDigest([]byte(idempotencyKey))[:32]
	persist := func() error { return s.commitOperation(operationID) }
	var operation *OperationRecord
	for i := range s.domain.Operations {
		candidate := &s.domain.Operations[i]
		if candidate.OperationID != operationID && candidate.IdempotencyKey != idempotencyKey {
			continue
		}
		if candidate.OperationID != operationID || candidate.IdempotencyKey != idempotencyKey || candidate.Kind != "ack_orca_delivery" || candidate.PayloadDigest != digest || string(candidate.Request) != string(request) {
			return errors.New("Orca Delivery acknowledgement identity conflicts with its persisted operation")
		}
		operation = candidate
		break
	}
	if operation != nil {
		switch operation.Status {
		case "confirmed":
			return nil
		case "rejected":
			return &BackendError{Code: BackendRejected, Diagnostic: "the exact Orca Delivery acknowledgement was previously rejected: " + operation.UnknownReason}
		case "in_flight":
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "the exact Orca Delivery acknowledgement is already in flight"}
		case "outcome_unknown":
			// Ack is idempotent by the exact Delivery ID; retry only that ID.
		default:
			return &BackendError{Code: BackendProtocolInvalid, Diagnostic: "Orca Delivery acknowledgement has an invalid persisted operation status"}
		}
	} else {
		value := OperationRecord{ProjectIdentity: s.Config.identity, OperationID: operationID, IdempotencyKey: idempotencyKey, Kind: "ack_orca_delivery", Status: "in_flight", PayloadDigest: digest, Request: request, CreatedAt: now(), UpdatedAt: now()}
		s.domain.Operations = append(s.domain.Operations, value)
		operation = &s.domain.Operations[len(s.domain.Operations)-1]
		if err := persist(); err != nil {
			return err
		}
		operation = nil
		for i := range s.domain.Operations {
			if s.domain.Operations[i].OperationID == operationID {
				operation = &s.domain.Operations[i]
				break
			}
		}
		if operation == nil {
			return errors.New("persisted Orca Delivery acknowledgement operation disappeared")
		}
	}
	requestID, ackErr := ev.Acknowledge()
	receipt := mustJSON(map[string]any{"run_id": ev.RemoteRunID, "delivery_id": ev.RemoteDeliveryID, "acknowledged": ackErr == nil, "error": errorString(ackErr)})
	operation.PlatformRequestID = requestID
	operation.Receipt = receipt
	operation.UpdatedAt = now()
	if ackErr != nil {
		var backendErr *BackendError
		if errors.As(ackErr, &backendErr) && (backendErr.Code == BackendOutcomeUnknown || backendErr.Code == BackendUnreachable || backendErr.Code == BackendIdentityMismatch || backendErr.Code == BackendProtocolInvalid) {
			operation.Status = "outcome_unknown"
		} else {
			operation.Status = "rejected"
		}
		operation.UnknownReason = ackErr.Error()
		if saveErr := persist(); saveErr != nil {
			return errors.Join(ackErr, saveErr)
		}
		return ackErr
	}
	if operation.Status != "confirmed" {
		if err := TransitionOperation(operation, "confirmed", requestID, receipt, ""); err != nil {
			return err
		}
	}
	return persist()
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

func (s *Service) persistRemoteWorkerMessages(ev WorkerEvent) error {
	if !stableExecutionID(ev.RunID) || !stableExecutionID(ev.AttemptID) || ev.RemoteRunID == "" {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "remote inbox batch lacks fixed local Run/attempt or external Orca Run identity"}
	}
	var run *Run
	for _, candidate := range s.runs {
		if candidate != nil && candidate.RunID == ev.RunID {
			run = candidate
			break
		}
	}
	if run == nil {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "remote inbox batch belongs to an unknown local Run"}
	}
	taskID := run.DomainTaskID
	if taskID == "" {
		taskID = run.TaskID
	}
	source := "orca-run:" + ev.RemoteRunID
	for _, message := range ev.RemoteMessages {
		if message.ID == "" || message.Type == "" {
			return &BackendError{Code: BackendProtocolInvalid, Diagnostic: "remote inbox message lacks a stable ID or type"}
		}
		duplicate := false
		for _, existing := range s.pendingEvents {
			if existing.ExternalSource == source && existing.ExternalID == message.ID {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		payload, err := json.Marshal(message)
		if err != nil {
			return err
		}
		messageAttempt := ""
		messageTaskID, messageDispatchID := orcaworker.MessageIdentity(message)
		if messageDispatchID != "" {
			for _, attempt := range s.domain.Attempts {
				if attempt.ExecutionID != run.ExecutionID || attempt.ExternalBinding.Dispatch != messageDispatchID {
					continue
				}
				if messageTaskID == "" || attempt.ExternalBinding.ExternalTask == messageTaskID {
					messageAttempt = attempt.AttemptID
					break
				}
			}
		}
		received := message.CreatedAt
		if received.IsZero() {
			received = now()
		}
		event := DomainEvent{ProjectIdentity: s.Config.identity, TaskID: taskID, WorkflowRunID: ev.RunID, ExecutionID: run.ExecutionID, AttemptID: messageAttempt, ExternalSource: source, ExternalID: message.ID, Type: "orca." + message.Type, ReceivedAt: received, PayloadBytes: len(payload), PayloadDigest: operationDigest(payload)}
		if len(payload) <= maxSupervisedMessageBytes {
			event.Payload = payload
		} else {
			event.PayloadOmitted = true
		}
		s.pendingEvents = append(s.pendingEvents, event)
	}
	return nil
}

func (s *Service) recordHistoricalWorkerEvent(ev WorkerEvent) error {
	if !stableExecutionID(ev.RunID) || !stableExecutionID(ev.AttemptID) || ev.Activity.Kind == "" {
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
	dir := filepath.Join(s.Config.RunDir, "runs", ev.RunID)
	if ev.AttemptID != ev.RunID {
		dir = filepath.Join(dir, "attempts", ev.AttemptID)
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
	if err = s.commitEvents(); err != nil {
		return err
	}
	w, err := OpenActivity(filepath.Join(dir, "activity.jsonl"), ev.RunID, run.TaskID)
	if err != nil {
		return err
	}
	return w.Write(ev.Activity)
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
	var codexHistoryDone <-chan struct{}
	defer func() {
		cancel()
		if conversationReplayDone != nil {
			<-conversationReplayDone
		}
		if codexHistoryDone != nil {
			<-codexHistoryDone
		}
		s.conversationReplayRequests = nil
		s.conversationReplayErrors = nil
	}()
	defer s.closeAllActivities()
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
	historyRequests := make(chan []codexHistoryBinding, 1)
	historyDone := make(chan struct{})
	codexHistoryDone = historyDone
	go func() {
		defer close(historyDone)
		s.runCodexHistoryCollector(ctx, historyRequests, s.conversationReplayErrors)
	}()
	historyRequests <- s.codexHistorySources()
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
			select {
			case historyRequests <- s.codexHistorySources():
			default:
			}
		case replayErr := <-s.conversationReplayErrors:
			if replayErr != nil {
				if e := step(func(context.Context) error { return replayErr }); e != nil {
					return e
				}
			}
		case message := <-s.workerMessages:
			if err := s.handleWorkerMessage(message); err != nil {
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
		for runID, owned := range s.ownedLocalProcesses {
			s.interruptOwnedLocalTurn(runID, owned)
			owned.Close()
			delete(s.ownedLocalProcesses, runID)
		}
	}()
	if s.runContext != nil && s.runContext.Err() != nil {
		s.forwarders.Wait()
		s.drainWorkerMessages()
	}
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
