package execution

import (
	"context"
	"encoding/json"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (s *Service) find(id string) *reconstructiontypes.RunView {
	var selected *reconstructiontypes.RunView
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

func (s *Service) findExecution(id string) *reconstructiontypes.RunView {
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
	if err := s.reconcileDiscardedExecutionStops(ctx); err != nil {
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
	if s.hasActiveWorkers() {
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
		if run != nil && !s.candidateBusinessClosed(run) && !resumingDecision && run.FinishTaskStatus == "waiting_decision" {
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
	s.State.SlotOccupied = s.State.SlotOccupied || s.hasActiveWorkers()
}

func (s *Service) refreshCandidates(ctx context.Context) error {
	_, err := s.taskWork.Ready(ctx, s.Config.identity)
	return err
}

func decisionMeta(r *reconstructiontypes.RunView) map[string]any {
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

func (s *Service) finishedForRun(run *reconstructiontypes.RunView, worker *Worker, err error) error {
	if worker != nil {
		for ev := range worker.Events {
			if eventErr := s.eventForRun(run, ev); eventErr != nil {
				s.setError(eventErr)
			}
		}
	}
	if run == nil || run.WorkflowVersion != executionmodel.Version {
		return nativeErr("unsupported_schema", "native service observed a run without the current worker workflow; preserve its execution identity")
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

func (s *Service) eventForRun(run *reconstructiontypes.RunView, ev WorkerEvent) error {
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
	if run.WorkflowVersion != executionmodel.Version {
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
	if !executionbackend.StableExecutionID(ev.RunID) || !executionbackend.StableExecutionID(ev.AttemptID) || ev.Activity.Kind == "" {
		if ev.Persisted != nil {
			return errors.New("conversation source identity is incomplete")
		}
		return nil
	}
	var run *reconstructiontypes.RunView
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
	attempt := (*executionmodel.StepAttemptRecord)(nil)
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
	s.pendingEvents = append(s.pendingEvents, storageports.DomainEvent{ProjectIdentity: s.Config.identity, TaskID: run.DomainTaskID, WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: ev.AttemptID, ExternalSource: "center:" + run.RunID, ExternalID: newID(), Type: "activity." + ev.Activity.Kind, ReceivedAt: ev.Activity.TS, Payload: payload})
	return s.commitEvents()
}

func (s *Service) finishWorkerInLoop(run *reconstructiontypes.RunView, worker *Worker, workerErr error) error {
	return s.finishedForRun(run, worker, workerErr)
}

// A retired observer may still drain original output after physical stop.
func (s *Service) hasActiveWorkers() bool {
	for id := range s.workers {
		run := s.find(id)
		if run == nil {
			return true
		}
		e := s.executionRecord(run)
		if e == nil || e.ResourceStoppedAt == nil || e.ResourceStopOperationID == "" || (e.Status != "stopped" && !s.candidateBusinessClosed(run)) {
			return true
		}
	}
	return false
}
