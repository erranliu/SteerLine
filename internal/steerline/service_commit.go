package steerline

import (
	"context"
	"encoding/json"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/projection"
)

func copyDomainSnapshot(snapshot DomainSnapshot) (DomainSnapshot, error) {
	return projection.CloneDomain(snapshot)
}

// projectState builds independently owned read views from committed records.
// Worker candidates live in domain and are never publication input.
func (s *Service) projectState() error {
	metadata := projection.MetadataOf(s.State)
	metadata.Checkpoint = runtimeCheckpoint(s.State)
	view, err := projection.Build(s.committed, metadata)
	if err != nil {
		return err
	}
	s.State = view
	return s.projectActor()
}

// Working projections are private to the single center actor. Public Runtime
// objects are built from committed rows and share no Run pointers with them.
func (s *Service) projectActor() error {
	runs, err := projection.Runs(s.domain)
	if err != nil {
		return err
	}
	previous := make(map[string]*Run, len(s.runs))
	for _, run := range s.runs {
		if run != nil {
			previous[run.RunID] = run
		}
	}
	for i, view := range runs {
		if existing := previous[view.RunID]; existing != nil {
			*existing = *view
			runs[i] = existing
		}
	}
	s.runs = runs
	return nil
}

func snapshotWorkflow(snapshot *DomainSnapshot, id string) *WorkflowRunRecord {
	for i := range snapshot.WorkflowRuns {
		if snapshot.WorkflowRuns[i].WorkflowRunID == id {
			return &snapshot.WorkflowRuns[i]
		}
	}
	return nil
}
func snapshotExecution(snapshot *DomainSnapshot, id string) *ExecutionRecord {
	for i := range snapshot.Executions {
		if snapshot.Executions[i].ExecutionID == id {
			return &snapshot.Executions[i]
		}
	}
	return nil
}
func snapshotAttempt(snapshot *DomainSnapshot, id string) *StepAttemptRecord {
	for i := range snapshot.Attempts {
		if snapshot.Attempts[i].AttemptID == id {
			return &snapshot.Attempts[i]
		}
	}
	return nil
}
func snapshotDecision(snapshot *DomainSnapshot, id string) *DecisionRecord {
	for i := range snapshot.Decisions {
		if snapshot.Decisions[i].DecisionID == id {
			return &snapshot.Decisions[i]
		}
	}
	return nil
}
func snapshotOperation(snapshot *DomainSnapshot, id string) *OperationRecord {
	for i := range snapshot.Operations {
		if snapshot.Operations[i].OperationID == id {
			return &snapshot.Operations[i]
		}
	}
	return nil
}

// commitWorker declares the records owned by one fixed worker: its workflow,
// execution, current attempt, declared decisions, operation receipts and events.
// It never examines other workers or discovers writes through fingerprints.
func (s *Service) commitWorker(run *Run, operationIDs ...string) error {
	return s.commitWorkerAttached(run, workerCommitAttachment{}, operationIDs...)
}

type workerCommitAttachment struct {
	Binding            *executionapi.ExecutionRef
	ExecutionOperation *recordWrite[executionapi.Operation]
}

func (s *Service) commitWorkerAttached(run *Run, attachment workerCommitAttachment, operationIDs ...string) error {
	if run == nil || s.domainStore == nil {
		return errors.New("worker commitment requires a run and store")
	}
	workflow, execution := s.workflowRecords(run)
	attempt := s.workflowAttempt(run, run.AttemptID)
	before := snapshotWorkflow(&s.committed, run.RunID)
	if workflow == nil || execution == nil || attempt == nil || before == nil {
		return errors.New("worker commitment has no authoritative fixed records")
	}
	request := workerCommit{Project: workflow.ProjectIdentity, TaskID: workflow.TaskID,
		Binding: attachment.Binding, ExecutionOperation: attachment.ExecutionOperation,
		WorkflowID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, AttemptID: attempt.AttemptID,
		ControlRevision:   before.ControlRevision,
		AuthorityRevision: s.State.Control.Revision,
		ExpectedAttemptID: attempt.AttemptID,
		Workflow:          &recordWrite[WorkflowRunRecord]{Before: before, After: *workflow},
		Execution:         &recordWrite[ExecutionRecord]{Before: snapshotExecution(&s.committed, execution.ExecutionID), After: *execution},
		Attempt:           &recordWrite[StepAttemptRecord]{Before: snapshotAttempt(&s.committed, attempt.AttemptID), After: *attempt},
		Checkpoint:        runtimeCheckpoint(s.State)}
	previousExecution := snapshotExecution(&s.committed, execution.ExecutionID)
	if previousExecution != nil && previousExecution.CurrentAttemptID != "" && previousExecution.CurrentAttemptID != attempt.AttemptID {
		request.ExpectedAttemptID = previousExecution.CurrentAttemptID
		retired := snapshotAttempt(&s.domain, previousExecution.CurrentAttemptID)
		if retired == nil {
			return errors.New("recovery lost the original Attempt")
		}
		request.RetiredAttempt = &recordWrite[StepAttemptRecord]{Before: snapshotAttempt(&s.committed, retired.AttemptID), After: *retired}
	}
	for _, decision := range s.domain.Decisions {
		if decision.WorkflowRunID != run.RunID {
			continue
		}
		before := snapshotDecision(&s.committed, decision.DecisionID)
		if s.workerDecisionWrites[decision.DecisionID] {
			request.Decisions = append(request.Decisions, recordWrite[DecisionRecord]{Before: before, After: decision})
		} else if decision.DecisionID == execution.DecisionID && before != nil {
			// Validate the durable answer read without adopting an undeclared candidate.
			request.Decisions = append(request.Decisions, recordWrite[DecisionRecord]{Before: before, After: *before})
		}
	}
	for _, id := range operationIDs {
		if id == "" {
			continue
		}
		if s.pendingOperations == nil {
			s.pendingOperations = map[string]string{}
		}
		s.pendingOperations[id] = run.RunID
	}
	for id, owner := range s.pendingOperations {
		if owner != run.RunID {
			continue
		}
		operation := snapshotOperation(&s.domain, id)
		if operation == nil {
			return errors.New("staged worker operation disappeared")
		}
		request.Operations = append(request.Operations, recordWrite[OperationRecord]{Before: snapshotOperation(&s.committed, id), After: *operation})
	}
	for _, event := range s.pendingEvents {
		if event.WorkflowRunID == run.RunID {
			request.Events = append(request.Events, event)
		}
	}
	if err := s.commitDeclared(request); err != nil {
		return err
	}
	return nil
}

func (s *Service) commitDeclared(request workerCommit) error {
	request.Checkpoint.UpdatedAt, request.Checkpoint.HeartbeatAt = now(), now()
	result, err := s.store.commitWorker(request)
	if err != nil {
		// Candidates have no durable authority. Restore the actor's working copy
		// without publishing a successful checkpoint or advancing consumption.
		if restored, restoreErr := copyDomainSnapshot(s.committed); restoreErr == nil {
			s.domain = restored
		}
		for _, write := range request.Operations {
			delete(s.pendingOperations, write.After.OperationID)
		}
		_ = s.projectState()
		return err
	}
	committed, copyErr := copyDomainSnapshot(s.committed)
	if copyErr != nil {
		return copyErr
	}
	if request.Workflow != nil {
		request.Workflow.After.ControlRevision = result.ControlRevision
		*snapshotWorkflow(&s.domain, request.WorkflowID) = request.Workflow.After
		*snapshotWorkflow(&committed, request.WorkflowID) = request.Workflow.After
	}
	if request.Execution != nil {
		*snapshotExecution(&committed, request.ExecutionID) = request.Execution.After
	}
	for _, write := range []*recordWrite[StepAttemptRecord]{request.RetiredAttempt, request.Attempt} {
		if write == nil {
			continue
		}
		if old := snapshotAttempt(&committed, write.After.AttemptID); old != nil {
			*old = write.After
		} else {
			committed.Attempts = append(committed.Attempts, write.After)
		}
	}
	for _, write := range request.Decisions {
		delete(s.workerDecisionWrites, write.After.DecisionID)
		if old := snapshotDecision(&committed, write.After.DecisionID); old != nil {
			*old = write.After
		} else {
			committed.Decisions = append(committed.Decisions, write.After)
		}
	}
	for _, write := range request.Operations {
		if old := snapshotOperation(&committed, write.After.OperationID); old != nil {
			*old = write.After
		} else {
			committed.Operations = append(committed.Operations, write.After)
		}
		delete(s.pendingOperations, write.After.OperationID)
	}
	s.committed, err = copyDomainSnapshot(committed)
	if err != nil {
		return err
	}
	consumed := make(map[string]bool, len(request.Events))
	for _, event := range request.Events {
		consumed[event.ExternalSource+"\x00"+event.ExternalID] = true
	}
	remaining := s.pendingEvents[:0]
	for _, event := range s.pendingEvents {
		if !consumed[event.ExternalSource+"\x00"+event.ExternalID] {
			remaining = append(remaining, event)
		}
	}
	s.pendingEvents = remaining
	s.State.Revision, s.State.UpdatedAt, s.State.HeartbeatAt = result.Checkpoint.Revision, result.Checkpoint.UpdatedAt, result.Checkpoint.HeartbeatAt
	return s.projectState()
}

func (s *Service) commitEvents() error {
	return s.commitDeclared(workerCommit{Project: s.Config.identity, Events: append([]DomainEvent(nil), s.pendingEvents...), Checkpoint: runtimeCheckpoint(s.State)})
}

func (s *Service) commitOperation(id string) error {
	operation := snapshotOperation(&s.domain, id)
	if operation == nil {
		return errors.New("operation commitment has no original intent")
	}
	return s.commitDeclared(workerCommit{Project: operation.ProjectIdentity,
		Operations: []recordWrite[OperationRecord]{{Before: snapshotOperation(&s.committed, id), After: *operation}},
		Checkpoint: runtimeCheckpoint(s.State)})
}

func (s *Service) stageWorkerOperation(run *Run, id, next, requestID string, receipt []byte, reason string) error {
	if run == nil {
		return errors.New("worker operation has no fixed run")
	}
	if err := s.setOperationState(id, next, requestID, receipt, reason); err != nil {
		return err
	}
	if s.pendingOperations == nil {
		s.pendingOperations = make(map[string]string)
	}
	s.pendingOperations[id] = run.RunID
	return nil
}

func (s *Service) changeWorkerOperation(run *Run, id, next, requestID string, receipt []byte, reason string) error {
	if err := s.stageWorkerOperation(run, id, next, requestID, receipt, reason); err != nil {
		return err
	}
	return s.commitWorker(run)
}

func (s *Service) startWorkerOperation(run *Run, kind, key string, payload any) (string, error) {
	if run == nil || kind == "" || key == "" {
		return "", errors.New("worker intent has no fixed run")
	}
	request, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	workflow := s.workflowRecord(run)
	if workerOperationRequiresRequirements(kind) && workflow != nil && workflow.ExecutionProfile != nil && !workflow.ExecutionProfile.ExecutionRequirements.Empty() {
		fixed, err := s.workerFlowContext(run)
		if err != nil {
			return "", err
		}
		// Existing outcomes remain read-only reconcileable; a pending intent has
		// not been sent and still needs current requirements before transmission.
		needsCheck := true
		for _, op := range s.domain.Operations {
			if op.IdempotencyKey == key && op.Status != "pending" {
				needsCheck = false
				break
			}
		}
		if needsCheck {
			ctx := s.runContext
			if ctx == nil {
				ctx = context.Background()
			}
			if err = s.checkCurrentExecutionRequirements(ctx, fixed); err != nil {
				return "", err
			}
		}
	}
	for _, existing := range s.domain.Operations {
		if existing.IdempotencyKey != key {
			continue
		}
		if existing.Kind != kind || existing.PayloadDigest != operationDigest(request) || string(existing.Request) != string(request) {
			return "", errors.New("worker operation idempotency key conflicts with its original request")
		}
		if existing.Status != "pending" {
			return "", errors.New("original worker operation must be reconciled before another action")
		}
		return existing.OperationID, s.changeWorkerOperation(run, existing.OperationID, "in_flight", "", nil, "")
	}
	operation := OperationRecord{ProjectIdentity: s.Config.identity, OperationID: newID(), IdempotencyKey: key, Kind: kind,
		Status: "in_flight", PayloadDigest: operationDigest(request), Request: request, CreatedAt: now(), UpdatedAt: now()}
	if lifecycleOperationKind(kind) {
		fixed, err := s.workerFlowContext(run)
		if err != nil {
			return "", err
		}
		snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
		if err != nil {
			return "", err
		}
		operation.LifecycleIdentity = &executionapi.LifecycleOperationIdentity{
			ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID,
			ConfigurationDigest: snapshot.ConfigurationDigest, ResourceGeneration: snapshot.ResourceGeneration,
			InputBindingDigest: executionapi.LifecycleBindingDigest(snapshot), Ref: snapshot.Ref,
		}
	}
	s.domain.Operations = append(s.domain.Operations, operation)
	if s.pendingOperations == nil {
		s.pendingOperations = make(map[string]string)
	}
	s.pendingOperations[operation.OperationID] = run.RunID
	return operation.OperationID, s.commitWorker(run)
}

func lifecycleOperationKind(kind string) bool {
	return knownLifecycleAction(executionapi.LifecycleAction(kind)) || kind == "prepare_worker_session" ||
		kind == "start_worker_turn" || kind == "submit_worker_input" || kind == "reply_execution_question" || kind == "stop_worker_observation"
}

func workerOperationRequiresRequirements(kind string) bool {
	switch kind {
	case "stop_worker_observation", "cleanup_resources", "acknowledge_delivery":
		return false
	default:
		return true
	}
}
