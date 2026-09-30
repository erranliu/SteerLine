package steerline

import (
	"context"
	"errors"
)

// commitWorkerFeedbackPhase is retained as a narrow storage test entrance. The
// service uses commitWorker so the phase and checkpoint share one transaction.
func (s *SQLiteStore) commitWorkerFeedbackPhase(ctx context.Context, expectedWorkflow WorkflowRunRecord, expectedExecution ExecutionRecord, expectedDecision DecisionRecord) (WorkflowRunRecord, error) {
	var zero WorkflowRunRecord
	project := expectedWorkflow.ProjectIdentity
	if project == "" || expectedWorkflow.WorkflowRunID == "" || expectedExecution.ExecutionID == "" || expectedDecision.DecisionID == "" {
		return zero, errors.New("worker feedback phase requires fixed workflow, execution and decision identities")
	}
	snapshot, err := s.Query(ctx, project)
	if err != nil {
		return zero, err
	}
	var attempt *StepAttemptRecord
	for i := range snapshot.Attempts {
		v := &snapshot.Attempts[i]
		if v.AttemptID == expectedDecision.AttemptID && v.ExecutionID == expectedExecution.ExecutionID {
			attempt = v
			break
		}
	}
	if attempt == nil {
		return zero, nativeErr("stale_revision", "saved worker feedback attempt is unavailable")
	}
	payload, _, err := s.LoadProjection(ctx, project)
	if err != nil {
		return zero, err
	}
	var checkpoint ServiceCheckpoint
	if err := decode(payload, &checkpoint); err != nil {
		return zero, err
	}
	authority, err := s.nativeAuthority(ctx, project)
	if err != nil {
		return zero, err
	}
	workflow := expectedWorkflow
	if err := BeginWorkerFeedbackDelivery(&workflow, expectedExecution, expectedDecision, now()); err != nil {
		return zero, err
	}
	request := workerCommitRequest{
		Project: project, TaskID: workflow.TaskID, WorkflowRunID: workflow.WorkflowRunID,
		ExecutionID: expectedExecution.ExecutionID, AttemptID: attempt.AttemptID,
		AuthorityRevision: authority.AuthorityVersion, ControlRevision: expectedWorkflow.ControlRevision,
		Workflow:   workerRecordWrite[WorkflowRunRecord]{Expected: recordFingerprint(expectedWorkflow), Record: workflow},
		Execution:  workerRecordWrite[ExecutionRecord]{Expected: recordFingerprint(expectedExecution), Record: expectedExecution},
		Attempt:    workerRecordWrite[StepAttemptRecord]{Expected: recordFingerprint(*attempt), Record: *attempt},
		Decisions:  []workerRecordWrite[DecisionRecord]{{Expected: recordFingerprint(expectedDecision), Record: expectedDecision}},
		Checkpoint: checkpoint,
	}
	result, err := s.commitWorkerTransition(ctx, request)
	return result.Workflow, err
}
