package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// commitWorker declares the fixed worker row set at each business boundary.
// Operation IDs are supplied by the caller; unrelated operations are excluded.
func (s *Service) commitWorker(run *Run, operationIDs ...string) error {
	if run == nil || s.store == nil || s.domainStore == nil {
		return errors.New("worker commit has no active store or run")
	}
	if err := projectRuntime(&s.State); err != nil {
		return err
	}
	workflow, execution := s.workflowRecords(run)
	attempt := s.workflowAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil {
		return errors.New("worker commit has no fixed records")
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if s.store.failure != nil {
		return s.store.failure
	}
	base := s.store.baseline
	if base == nil {
		return errors.New("worker commit has no observed baseline")
	}
	s.State.UpdatedAt, s.State.HeartbeatAt = now(), now()
	request := workerCommitRequest{
		Project: s.Config.identity, TaskID: workflow.TaskID, WorkflowRunID: workflow.WorkflowRunID,
		ExecutionID: execution.ExecutionID, AttemptID: attempt.AttemptID,
		AuthorityRevision: s.State.Control.Revision, ControlRevision: workflow.ControlRevision,
		Workflow:   workerRecordWrite[WorkflowRunRecord]{Expected: base["workflow:"+workflow.WorkflowRunID], Record: *workflow},
		Execution:  workerRecordWrite[ExecutionRecord]{Expected: base["execution:"+execution.ExecutionID], Record: *execution},
		Attempt:    workerRecordWrite[StepAttemptRecord]{Expected: base["attempt:"+attempt.AttemptID], Record: *attempt},
		Checkpoint: runtimeCheckpoint(s.State),
	}
	for _, decision := range s.State.Decisions {
		if s.workerDecisionWrites[decision.DecisionID] && decision.WorkflowRunID == workflow.WorkflowRunID && decision.ExecutionID == execution.ExecutionID && decision.AttemptID == attempt.AttemptID {
			request.Decisions = append(request.Decisions, workerRecordWrite[DecisionRecord]{Expected: base["decision:"+decision.DecisionID], Record: decision})
		} else if decision.DecisionID == execution.DecisionID && decision.WorkflowRunID == workflow.WorkflowRunID && decision.ExecutionID == execution.ExecutionID && decision.AttemptID == attempt.AttemptID {
			request.DecisionReads = append(request.DecisionReads, workerRecordRead{ID: decision.DecisionID, Expected: base["decision:"+decision.DecisionID]})
		}
	}
	seen := map[string]bool{}
	for _, id := range operationIDs {
		if id == "" || seen[id] {
			return errors.New("worker operation IDs must be unique and nonempty")
		}
		seen[id] = true
		found := false
		for _, op := range s.State.Operations {
			if op.OperationID == id {
				request.Operations = append(request.Operations, workerRecordWrite[OperationRecord]{Expected: base["operation:"+id], Record: op})
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("worker operation %s is unavailable", id)
		}
	}
	remaining := make([]DomainEvent, 0, len(s.State.PendingEvents))
	for _, event := range s.State.PendingEvents {
		if event.WorkflowRunID == workflow.WorkflowRunID && event.ExecutionID == execution.ExecutionID && (event.AttemptID == "" || event.AttemptID == attempt.AttemptID) {
			request.Events = append(request.Events, event)
		} else {
			remaining = append(remaining, event)
		}
	}
	result, err := s.domainStore.commitWorkerTransition(context.Background(), request)
	if err != nil {
		// Discard the candidate domain rows before any reader can treat them as
		// committed. Keep pending events for diagnosis; the actor is stopped.
		if snapshot, readErr := s.domainStore.Query(context.Background(), request.Project); readErr == nil {
			hydrateRuntime(&s.State, snapshot)
			for _, d := range request.Decisions {
				delete(s.workerDecisionWrites, d.Record.DecisionID)
			}
			_ = projectRuntime(&s.State)
		}
		s.store.failure = fmt.Errorf("worker commit failed; actor must be reloaded: %w", err)
		return s.store.failure
	}
	*workflow = result.Workflow
	base["workflow:"+workflow.WorkflowRunID] = recordFingerprint(*workflow)
	base["execution:"+execution.ExecutionID] = recordFingerprint(*execution)
	base["attempt:"+attempt.AttemptID] = recordFingerprint(*attempt)
	for _, d := range request.Decisions {
		base["decision:"+d.Record.DecisionID] = recordFingerprint(d.Record)
		delete(s.workerDecisionWrites, d.Record.DecisionID)
	}
	for _, o := range request.Operations {
		base["operation:"+o.Record.OperationID] = recordFingerprint(o.Record)
	}
	s.State.PendingEvents = remaining
	s.State.Revision = result.Revision
	s.State.UpdatedAt = now()
	s.State.HeartbeatAt = s.State.UpdatedAt
	return projectRuntime(&s.State)
}

func (s *Service) startWorkerOperation(run *Run, kind, key string, payload any) (string, error) {
	if kind == "" || key == "" {
		return "", errors.New("operation kind and idempotency key are required")
	}
	request, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	for i := range s.State.Operations {
		op := &s.State.Operations[i]
		if op.IdempotencyKey != key {
			continue
		}
		if op.Kind != kind || op.PayloadDigest != operationDigest(request) || string(op.Request) != string(request) {
			return "", errors.New("operation idempotency key conflicts with the original request")
		}
		if op.Status != "pending" {
			return "", fmt.Errorf("operation %s is already %s; reconcile the original operation before another action", op.OperationID, op.Status)
		}
		if err := TransitionOperation(op, "in_flight", "", nil, ""); err != nil {
			return "", err
		}
		if err := s.commitWorker(run, op.OperationID); err != nil {
			return "", err
		}
		return op.OperationID, nil
	}
	op := OperationRecord{ProjectIdentity: s.Config.identity, OperationID: newID(), IdempotencyKey: key, Kind: kind, Status: "in_flight", PayloadDigest: operationDigest(request), Request: request, CreatedAt: now(), UpdatedAt: now()}
	s.State.Operations = append(s.State.Operations, op)
	if err := s.commitWorker(run, op.OperationID); err != nil {
		return "", err
	}
	return op.OperationID, nil
}

func (s *Service) changeWorkerOperation(run *Run, id, next, requestID string, receipt []byte, reason string) error {
	if err := s.setOperationState(id, next, requestID, receipt, reason); err != nil {
		return err
	}
	return s.commitWorker(run, id)
}
