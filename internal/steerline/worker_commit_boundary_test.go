package steerline

import (
	"context"
	"errors"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"path/filepath"
	"testing"
)

func TestWorkerCommitWritesOnlyDeclaredRowsAndCheckpoint(t *testing.T) {
	s, run := waitingDecisionService(t)
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	before, _, err := s.domainStore.LoadProjection(ctx, s.Config.identity)
	require(t, err)
	var old ServiceCheckpoint
	require(t, decode(before, &old))
	unlisted := OperationRecord{ProjectIdentity: s.Config.identity, OperationID: newID(), IdempotencyKey: newID(), Kind: "unlisted", Status: "in_flight", PayloadDigest: operationDigest([]byte("{}")), Request: []byte("{}"), CreatedAt: now(), UpdatedAt: now()}
	s.domain.Operations = append(s.domain.Operations, unlisted)
	s.decisionRecord(run).Status = "superseded" // A modified but undeclared row stays private to the actor.
	s.executionRecord(run).LastEvent = ptr("committed worker diagnostic")
	s.note(run, "steer", "worker", "observed", "explicit commit", "", nil)
	require(t, s.commitWorker(run))
	snapshot, err := s.domainStore.Query(ctx, s.Config.identity)
	require(t, err)
	for _, op := range snapshot.Operations {
		if op.OperationID == unlisted.OperationID {
			t.Fatal("unlisted operation was persisted")
		}
	}
	if snapshot.Decisions[0].Status != "pending" {
		t.Fatal("unlisted decision was persisted")
	}
	if snapshot.Executions[0].LastEvent == nil || *snapshot.Executions[0].LastEvent != "committed worker diagnostic" {
		t.Fatal("declared execution was not persisted")
	}
	events, err := s.domainStore.Events(ctx, s.Config.identity, run.ExecutionID, run.AttemptID, 0, 100, MaxEventPageBytes)
	require(t, err)
	if len(events) == 0 {
		t.Fatal("worker event was not persisted")
	}
	after, _, err := s.domainStore.LoadProjection(ctx, s.Config.identity)
	require(t, err)
	var checkpoint ServiceCheckpoint
	require(t, decode(after, &checkpoint))
	if checkpoint.Revision != old.Revision+1 || len(s.pendingEvents) != 0 {
		t.Fatal("event and checkpoint did not commit together")
	}
	s.State.Service.Error = ptr("service metadata after the worker transition")
	require(t, s.publishCheckpoint())
	snapshot, err = s.domainStore.Query(ctx, s.Config.identity)
	require(t, err)
	if snapshot.Decisions[0].Status != "pending" {
		t.Fatal("scheduler checkpoint publication flushed an undeclared decision")
	}
	for _, op := range snapshot.Operations {
		if op.OperationID == unlisted.OperationID {
			t.Fatal("scheduler checkpoint publication flushed an undeclared operation")
		}
	}
	if snapshotOperation(&s.committed, unlisted.OperationID) != nil {
		t.Fatal("checkpoint publication advanced an undeclared operation baseline")
	}
}

func TestWorkerCommitRollsBackOnStaleRecordAndCheckpointFailure(t *testing.T) {
	for _, stage := range []string{"stale", "event", "checkpoint"} {
		t.Run(stage, func(t *testing.T) {
			s, run := waitingDecisionService(t)
			t.Cleanup(func() { _ = s.Close() })
			ctx := context.Background()
			before, _, err := s.domainStore.LoadProjection(ctx, s.Config.identity)
			require(t, err)
			s.executionRecord(run).LastEvent = ptr("must roll back")
			s.note(run, "steer", "worker", "observed", "must roll back", "", nil)
			if stage == "stale" {
				current := *s.executionRecord(run)
				current.LastEvent = ptr("concurrent writer")
				require(t, s.domainStore.Transact(ctx, func(tx DomainTx) error { return tx.PutExecution(current) }))
			} else if stage == "event" {
				_, err := s.domainStore.db.ExecContext(ctx, `CREATE TRIGGER reject_worker_event BEFORE INSERT ON events BEGIN SELECT RAISE(FAIL, 'event failure'); END`)
				require(t, err)
			} else {
				_, err := s.domainStore.db.ExecContext(ctx, `CREATE TRIGGER reject_worker_checkpoint BEFORE UPDATE ON projection BEGIN SELECT RAISE(FAIL, 'checkpoint failure'); END`)
				require(t, err)
			}
			commitErr := s.commitWorker(run)
			if commitErr == nil {
				t.Fatal("worker commit unexpectedly succeeded")
			}
			after, _, err := s.domainStore.LoadProjection(ctx, s.Config.identity)
			require(t, err)
			if string(after) != string(before) {
				t.Fatal("failed worker commit advanced checkpoint")
			}
			snapshot, err := s.domainStore.Query(ctx, s.Config.identity)
			require(t, err)
			if snapshot.Executions[0].LastEvent != nil && *snapshot.Executions[0].LastEvent == "must roll back" {
				t.Fatalf("failed worker commit wrote candidate execution: %v", commitErr)
			}
			if len(s.pendingEvents) == 0 {
				t.Fatal("failed worker commit cleared pending event")
			}
			if value := s.executionRecord(run).LastEvent; value != nil && *value == "must roll back" {
				t.Fatal("actor exposed the uncommitted candidate after failure")
			}
		})
	}
}

func TestWorkerReceiptFailureRetainsIntentAndCancelsUnregisteredObserver(t *testing.T) {
	s, run := neutralAuditService(t)
	commandID, turnID := newID(), "00000000-0000-0000-0000-000000000001"
	execution := s.executionRecord(run)
	workflow, _ := s.workflowRecords(run)
	execution.Workspace.Worktree = run.WorkspaceIntent.Path
	execution.PolicyResources = neutralPolicyReceipt(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
	execution.PendingCommandID = commandID
	setNeutralBinding(t, execution, "ws://127.0.0.1:12345", "generation", workerFlowMockSession)
	opID, err := s.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+run.ExecutionID+":"+commandID, map[string]string{"command_id": commandID})
	require(t, err)
	baseline := recordFingerprint(*snapshotOperation(&s.committed, opID))
	_, err = s.domainStore.db.Exec(`CREATE TRIGGER reject_worker_receipt_event BEFORE INSERT ON events BEGIN SELECT RAISE(FAIL, 'receipt event failure'); END`)
	require(t, err)
	identity, err := currentProcessIdentity()
	require(t, err)
	cancelled := false
	worker := &Worker{proc: &durableWorkerProcess{identity: *identity}, cancelObservation: func() { cancelled = true }}
	if err := s.acceptStartedWorkerTurn(run, worker, filepath.Join(t.TempDir(), "events.jsonl"), turnID, opID, commandID, true); err == nil {
		t.Fatal("receipt was accepted without its event")
	}
	if !cancelled || s.workers[run.RunID] != nil || s.store.Err() == nil || recordFingerprint(*snapshotOperation(&s.committed, opID)) != baseline {
		t.Fatal("receipt failure attached an observer or advanced the persistence baseline")
	}
	snapshot, err := s.domainStore.Query(context.Background(), s.Config.identity)
	require(t, err)
	for _, op := range snapshot.Operations {
		if op.OperationID == opID {
			if op.Status != "in_flight" || snapshot.Executions[0].PendingTurnID != "" || snapshot.Executions[0].Process != nil {
				t.Fatal("failed receipt transaction did not retain the original intent")
			}
			return
		}
	}
	t.Fatal("original send intent disappeared after failed receipt commit")
}

func TestAppServerFailedPreparationKeepsPartialSessionBinding(t *testing.T) {
	s, run := neutralAuditService(t)
	commandID := newID()
	execution := s.executionRecord(run)
	workflow, _ := s.workflowRecords(run)
	execution.Workspace.Worktree = run.WorkspaceIntent.Path
	execution.Generation = newID()
	execution.ConfigurationDigest = workflow.ExecutionProfile.Digest
	execution.PendingCommandID = commandID
	input := executionapi.TurnInput{CommandID: commandID, Prompt: "original saved prompt"}
	resource := neutralPolicyReceipt(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
	execution.PolicyResources = resource
	s.refreshRun(run)
	opID, err := s.startWorkerOperation(run, "prepare_worker_session", "worker-session:"+run.ExecutionID+":"+commandID, &input)
	require(t, err)
	identity, err := currentProcessIdentity()
	require(t, err)
	partial := *s.executionRecord(run)
	partial.Process = identity
	setNeutralBinding(t, &partial, "ws://127.0.0.1:12345", "partial-generation", workerFlowMockSession)
	lifeSnapshot, err := lifecycleSnapshot(run, workflow, execution, s.workflowAttempt(run, run.AttemptID), &input)
	require(t, err)
	operation := s.operationByKey("worker-session:" + run.ExecutionID + ":" + commandID)
	request := lifecycleRequest(lifeSnapshot, *operation, executionapi.LifecyclePrepareSession, executionapi.LifecycleInput{Turn: &input})
	receipt := executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationUnknown, Process: identity,
		Implementation: cloneImplementationData(partial.ImplementationData),
		State:          executionapi.LifecycleStateFacts{SessionID: workerFlowMockSession, SessionGeneration: "partial-generation"}}
	message := &appServerLaunchMessage{phase: "session_prepared", request: request, receipt: receipt, err: errors.New("session receipt persistence failed")}
	require(t, s.handleAppServerLaunchMessage(run.RunID, message))
	snapshot, err := s.domainStore.Query(context.Background(), s.Config.identity)
	require(t, err)
	got := snapshot.Executions[0]
	gotBinding, bindingErr := decodeNeutralBinding(got.ImplementationData[0], &got)
	partialBinding, partialBindingErr := decodeNeutralBinding(partial.ImplementationData[0], &partial)
	if bindingErr != nil || partialBindingErr != nil || got.Process == nil || *got.Process != *identity || gotBinding.Endpoint != partialBinding.Endpoint || gotBinding.Generation != partialBinding.Generation || got.SessionID != partial.SessionID {
		t.Fatalf("partial native binding was discarded: %+v", got)
	}
	if got.PendingTurnID != "" || s.workers[run.RunID] != nil || snapshotOperation(&snapshot, opID).Status != "outcome_unknown" {
		t.Fatal("failed preparation advanced the original turn or registered an observer")
	}
}

func TestWorkerCommitRejectsSavedAnswerRegression(t *testing.T) {
	for _, change := range []string{"status", "answer"} {
		t.Run(change, func(t *testing.T) {
			s, run := waitingDecisionService(t)
			t.Cleanup(func() { _ = s.Close() })
			record := s.decisionRecord(run)
			answer := DecisionAnswer{DecisionID: record.DecisionID, RunID: run.RunID, Note: "keep this answer", AnsweredBy: "operator", AnsweredAt: now()}
			record.Status, record.AnsweredBy, record.Answer = "answer_saved", "operator", mustJSON(answer)
			record.Version++
			s.markWorkerDecisionWrite(record.DecisionID)
			require(t, s.commitWorker(run))
			original := *s.decisionRecord(run)
			if change == "status" {
				s.setDecisionStatus(run, "pending")
			} else {
				answer.Note = "replacement"
				s.decisionRecord(run).Answer = mustJSON(answer)
				s.markWorkerDecisionWrite(original.DecisionID)
			}
			if err := s.commitWorker(run); err == nil {
				t.Fatal("invalid worker change overwrote a saved answer")
			}
			snapshot, err := s.domainStore.Query(context.Background(), s.Config.identity)
			require(t, err)
			if snapshot.Decisions[0].Status != original.Status || string(snapshot.Decisions[0].Answer) != string(original.Answer) {
				t.Fatal("rejected worker transition changed the saved answer")
			}
		})
	}
}

func TestWorkerIntentIsDurableBeforeDispatchAndFailurePreventsDispatch(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "committed", true: "checkpoint_failure"}[fail], func(t *testing.T) {
			s, run := waitingDecisionService(t)
			t.Cleanup(func() { _ = s.Close() })
			ctx := context.Background()
			key := "worker-turn:" + run.ExecutionID + ":" + newID()
			if fail {
				_, err := s.domainStore.db.ExecContext(ctx, `CREATE TRIGGER reject_worker_intent BEFORE UPDATE ON projection BEGIN SELECT RAISE(FAIL, 'checkpoint failure'); END`)
				require(t, err)
			}
			calls := 0
			id, err := s.startWorkerOperation(run, "start_worker_turn", key, map[string]string{"execution_id": run.ExecutionID})
			if err == nil {
				snapshot, readErr := s.domainStore.Query(ctx, s.Config.identity)
				require(t, readErr)
				found := false
				for _, op := range snapshot.Operations {
					if op.OperationID == id && op.IdempotencyKey == key && op.Status == "in_flight" {
						found = true
					}
				}
				if !found {
					t.Fatal("platform dispatch could run before its durable intent")
				}
				calls++
				if _, again := s.startWorkerOperation(run, "start_worker_turn", key, map[string]string{"execution_id": run.ExecutionID}); again == nil {
					t.Fatal("original in-flight request was offered for resend")
				}
			} else if !fail {
				t.Fatalf("intent could not commit: %v", err)
			}
			if fail && calls != 0 {
				t.Fatal("platform dispatch ran after failed intent commit")
			}
		})
	}
}
