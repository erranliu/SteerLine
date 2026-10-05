package execution

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/product/query"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestWorkerCommitRollsBackRecordsReceiptsEventsAndCheckpoint(t *testing.T) {
	service, run := waitingDecisionService(t)
	ctx := context.Background()
	before, revision, err := testSQLite(service).LoadProjection(ctx, service.Config.Identity())
	require(t, err)
	original, err := copyDomainSnapshot(service.committed)
	require(t, err)
	event := storageports.DomainEvent{ProjectIdentity: service.Config.Identity(), TaskID: run.DomainTaskID, WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, ExternalSource: "commit-test", ExternalID: newID(), Type: "worker.test", ReceivedAt: now(), Payload: mustJSON(map[string]string{"receipt": "must roll back"})}
	service.pendingEvents = append(service.pendingEvents, event)
	service.executionRecord(run).LastEvent = ptr("must roll back")
	service.setDecisionStatus(run, "superseded")
	_, err = testSQLite(service).db.ExecContext(ctx, `CREATE TRIGGER fail_worker_checkpoint BEFORE UPDATE ON projection BEGIN SELECT RAISE(ABORT,'checkpoint unavailable'); END`)
	require(t, err)

	// The checkpoint is the final write; its failure must undo the entire intent.
	_, err = service.startWorkerOperation(run, "start_worker_turn", "worker-test:"+run.ExecutionID, map[string]string{"intent": "original"})
	if err == nil || !strings.Contains(err.Error(), "checkpoint unavailable") {
		t.Fatalf("checkpoint fault was not injected: %v", err)
	}
	after, nextRevision, err := testSQLite(service).LoadProjection(ctx, service.Config.Identity())
	require(t, err)
	durable, err := service.domainStore.Query(ctx, service.Config.Identity())
	require(t, err)
	if revision != nextRevision || !bytes.Equal(before, after) || recordFingerprint(original) != recordFingerprint(durable) {
		t.Fatal("failed checkpoint changed durable business state or receipt")
	}
	events, err := testSQLite(service).Events(ctx, service.Config.Identity(), run.RunID, "", 0, 1000, query.MaxEventPageBytes)
	require(t, err)
	for _, saved := range events {
		if saved.ExternalID == event.ExternalID {
			t.Fatal("failed commit persisted its event")
		}
	}
	if len(service.pendingEvents) == 0 || len(service.pendingOperations) != 0 || recordFingerprint(service.domain) != recordFingerprint(original) || run.Decision.Status != "pending" {
		t.Fatal("failed commit consumed facts or retained actor candidates")
	}
	if service.store.Err() == nil {
		t.Fatal("database failure did not block subsequent writes")
	}
	if err := service.publishCheckpoint(); err == nil {
		t.Fatal("failed writer published a successful checkpoint")
	}
}

func TestCheckpointAndReadViewsCannotPublishWorkerCandidates(t *testing.T) {
	service, run := waitingDecisionService(t)
	ctx := context.Background()
	original := recordFingerprint(service.committed)
	// Mutate nested public slices/pointers, as well as an uncommitted actor candidate.
	service.State.Executions[0].WorkspaceIntent.Path = "public mutation"
	service.State.Decisions[0].Request[0] = '!'
	run.Decision.Request.Options[0].Label = "view mutation"
	service.refreshRun(run)
	run.WorkspaceIntent.Path = "refreshed view mutation"
	if recordFingerprint(service.domain) != original || recordFingerprint(service.committed) != original {
		t.Fatal("public read views alias private worker records")
	}
	service.executionRecord(run).LastEvent = ptr("uncommitted candidate")
	service.pendingEvents = append(service.pendingEvents, storageports.DomainEvent{ProjectIdentity: service.Config.Identity(), ExternalSource: "unpublished", ExternalID: newID(), Type: "worker.test", ReceivedAt: now(), Payload: mustJSON(map[string]bool{"pending": true})})
	require(t, service.publishCheckpoint())
	require(t, service.store.Heartbeat())
	durable, err := service.domainStore.Query(ctx, service.Config.Identity())
	require(t, err)
	if recordFingerprint(durable) != original || len(service.pendingEvents) != 1 {
		t.Fatal("checkpoint or heartbeat wrote or consumed business candidates")
	}
	events, err := testSQLite(service).Events(ctx, service.Config.Identity(), "", "", 0, 1000, query.MaxEventPageBytes)
	require(t, err)
	for _, event := range events {
		if event.ExternalSource == "unpublished" {
			t.Fatal("checkpoint implicitly published an event")
		}
	}
}

func TestWorkerCommitChecksSettledTaskInsideTransaction(t *testing.T) {
	service, run := waitingDecisionService(t)
	ctx := context.Background()
	// A concurrent discard cannot be inferred from the actor's cached task.
	task := *nativeTaskRecord(service.domain.Tasks, run.DomainTaskID)
	discarded, err := taskmodel.Discard(task, "concurrent settlement", "operator", newID(), nil, now())
	require(t, err)
	require(t, testSQLite(service).transactRecords(ctx, func(tx *fixtureTransaction) error { return tx.UpdateNativeTaskCAS(discarded, task.Version) }))
	before, revision, err := testSQLite(service).LoadProjection(ctx, service.Config.Identity())
	require(t, err)
	service.setDecisionStatus(run, "reply_accepted")
	if err := service.commitWorker(run); !isNativeErrorCode(err, "task_not_active") {
		t.Fatalf("settled task resumed from stale actor state: %v", err)
	}
	after, nextRevision, err := testSQLite(service).LoadProjection(ctx, service.Config.Identity())
	require(t, err)
	if revision != nextRevision || !bytes.Equal(before, after) {
		t.Fatal("settled-task rejection advanced the checkpoint")
	}
}
