package steerline

import (
	"context"
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
	s.State.Operations = append(s.State.Operations, unlisted)
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
	events, err := s.domainStore.Events(ctx, s.Config.identity, run.ExecutionID, run.AttemptID, 0, 100)
	require(t, err)
	if len(events) == 0 {
		t.Fatal("worker event was not persisted")
	}
	after, _, err := s.domainStore.LoadProjection(ctx, s.Config.identity)
	require(t, err)
	var checkpoint ServiceCheckpoint
	require(t, decode(after, &checkpoint))
	if checkpoint.Revision != old.Revision+1 || len(s.State.PendingEvents) != 0 {
		t.Fatal("event and checkpoint did not commit together")
	}
}

func TestWorkerCommitRollsBackOnStaleRecordAndCheckpointFailure(t *testing.T) {
	for _, stage := range []string{"stale", "checkpoint"} {
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
			if len(s.State.PendingEvents) == 0 {
				t.Fatal("failed worker commit cleared pending event")
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
