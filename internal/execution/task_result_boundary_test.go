package execution

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

var errCapturedResult = errors.New("hold verified declaration before atomic commit")

type resultCaptureStore struct {
	CommandStore
	commit storageports.ResultCommit
}

func (s *resultCaptureStore) CommitTaskResult(_ context.Context, commit storageports.ResultCommit) (storageports.ResultCommitReceipt, error) {
	s.commit = commit
	return storageports.ResultCommitReceipt{}, errCapturedResult
}

// Capture the real production preflight, including provider and whole PR-set
// verification. Only the final commit is held so races can use the same before.
func captureVerifiedResult(t *testing.T, f mcFixture) storageports.ResultCommit {
	t.Helper()
	s := &resultCaptureStore{CommandStore: f.store}
	graph, err := f.store.TaskGraphSnapshot(context.Background(), f.c.Identity())
	require(t, err)
	_, err = newTestCommandService(s, f.c).CompleteNativeTask(context.Background(), f.c, "8", graph.ByID["8"].Version, f.first.RunID, testPublicationClient(f.c))
	if !errors.Is(err, errCapturedResult) || s.commit.Task.Input.ResultID != f.first.RunID {
		t.Fatalf("real verification did not reach commit: %v", err)
	}
	return s.commit
}

func resultFixture(t *testing.T) mcFixture {
	f := newMCFixture(t)
	mcStartTurn(t, f.s, f.first)
	mcFinishTurn(t, f.s, f.first)
	f.bind(t, f.first, 81, "open")
	return f
}

func TestTM02TaskWorkPort(t *testing.T) {
	f := newMCFixture(t)
	ctx := context.Background()
	graph, err := f.store.TaskGraphSnapshot(ctx, f.c.Identity())
	require(t, err)
	work, err := f.commands.Work.Fixed(ctx, f.c.Identity(), "8", graph.ByID["8"].Version, true)
	require(t, err)
	if work.Record.Claimant != "operator" || work.Record.Status != "in_progress" || work.Record.Version != 2 || work.Contract.ContractID != work.Record.ContractID {
		t.Fatal("later candidate lost original fixed work and claim")
	}
	if _, err := f.commands.Work.Claim(work, "replacement", now()); !isNativeErrorCode(err, "stale_version") {
		t.Fatalf("work port reclaimed task: %v", err)
	}
	commands, err := f.store.Commands(ctx, f.c.Identity(), "accepted", 100)
	require(t, err)
	var original storageports.CommandRecord
	for _, command := range commands {
		if command.Kind == "claim_task" && command.TargetID == "8" {
			original = command
			break
		}
	}
	if original.CommandID == "" {
		t.Fatal("original claim request disappeared")
	}
	options := TaskClaimOptions{TaskID: "8", RequestID: original.CommandID, ExpectedTaskVersion: 1, ExpectedAuthorityVersion: graph.Authority.AuthorityVersion, RequestedBy: "operator", Explicit: true}
	replay, err := f.commands.ClaimNativeTask(ctx, f.c, options)
	require(t, err)
	if replay.WorkflowRunID != f.first.RunID || replay.ExecutionID != f.first.ExecutionID || replay.TaskVersion != work.Record.Version {
		t.Fatal("claim retry changed original reservation")
	}
	options.ExpectedTaskVersion++
	replayed, err := f.commands.ClaimNativeTask(ctx, f.c, options)
	require(t, err)
	if digestValue(replayed) != digestValue(replay) {
		t.Fatal("accepted first claim retried its version preconditions")
	}
	options.RequestedBy = "replacement"
	if _, err := f.commands.ClaimNativeTask(ctx, f.c, options); !isNativeErrorCode(err, "idempotency_conflict") {
		t.Fatalf("changed original claim requester admitted: %v", err)
	}
}

func TestTM02ResultCAS(t *testing.T) {
	ctx := context.Background()
	t.Run("duplicate_original_receipt_and_content_conflict", func(t *testing.T) {
		f := resultFixture(t)
		f.proof(t, f.first, 81, "merged")
		commit := captureVerifiedResult(t, f)
		first, err := f.store.CommitTaskResult(ctx, commit)
		require(t, err)
		beforeID, beforeRevision, err := f.store.DatabaseVersion(ctx)
		require(t, err)
		// A concurrent retry's sampling timestamp is not new result content.
		commit.Delivery.RecordedAt = commit.Delivery.RecordedAt.Add(time.Second)
		replay, err := f.store.CommitTaskResult(ctx, commit)
		require(t, err)
		afterID, afterRevision, err := f.store.DatabaseVersion(ctx)
		require(t, err)
		if beforeID != afterID || beforeRevision != afterRevision {
			t.Fatal("duplicate result changed database revision")
		}
		if digestValue(first) != digestValue(replay) {
			t.Fatal("duplicate result lost its original complete receipt")
		}
		commit.Task.Input.RequestID = "replacement-request"
		if _, err = f.store.CommitTaskResult(ctx, commit); !isNativeErrorCode(err, "idempotency_conflict") {
			t.Fatalf("different original request admitted: %v", err)
		}
		var count int
		require(t, f.store.db.QueryRow(`SELECT COUNT(*) FROM task_results`).Scan(&count))
		if count != 1 {
			t.Fatalf("result dedup wrote %d rows", count)
		}
		t.Logf("original result and delivery receipt: %s", mustJSON(first))
	})
	for _, stale := range []string{"task_version", "graph_version", "candidate_set", "claim_identity", "execution_identity", "failure", "unknown", "reply", "stopped"} {
		t.Run(stale, func(t *testing.T) {
			f := resultFixture(t)
			f.proof(t, f.first, 81, "merged")
			commit := captureVerifiedResult(t, f)
			switch stale {
			case "task_version":
				commit.Task.Input.ExpectedTaskVersion++
			case "graph_version":
				commit.Task.Input.ExpectedGraphRevision++
			case "candidate_set":
				commit.Task.Input.ExpectedSetVersion = "old-set"
			case "claim_identity":
				commit.Task.Input.Claimant = "another-worker"
			case "execution_identity":
				commit.Execution.ProjectIdentity = "another-project"
			default:
				commit.Task.Input.Conclusion = stale
			}
			if _, err := f.store.CommitTaskResult(ctx, commit); err == nil {
				t.Fatalf("%s released the successor", stale)
			}
			graph, err := f.store.TaskGraphSnapshot(ctx, f.c.Identity())
			require(t, err)
			if graph.ByID["8"].Status != "in_progress" || len(graph.Deliveries) != 0 {
				t.Fatal("rejected result partially committed")
			}
			ready, err := f.commands.Work.Ready(ctx, f.c.Identity())
			require(t, err)
			if len(ready) != 0 {
				t.Fatal("rejected result unlocked task work")
			}
		})
	}
	t.Run("merged_discard_rejected", func(t *testing.T) {
		f := resultFixture(t)
		f.proof(t, f.first, 81, "merged")
		graph, err := f.store.TaskGraphSnapshot(ctx, f.c.Identity())
		require(t, err)
		receipt, err := f.commands.DiscardNativeTask(ctx, f.c, "8", newID(), graph.ByID["8"].Version, "stop comparing", "operator")
		require(t, err)
		if receipt.Status != "rejected" || receipt.Code != "already_merged" {
			t.Fatalf("merged task was discarded: %+v", receipt)
		}
	})
	for _, order := range []string{"delivery_first", "discard_first", "concurrent"} {
		t.Run(order, func(t *testing.T) {
			f := resultFixture(t)
			// Persist the original discard intent while external outcome is unknown.
			f.proof(t, f.first, 81, "unknown")
			graph, err := f.store.TaskGraphSnapshot(ctx, f.c.Identity())
			require(t, err)
			requestID := newID()
			receipt, err := f.commands.DiscardNativeTask(ctx, f.c, "8", requestID, graph.ByID["8"].Version, "stop comparing", "operator")
			require(t, err)
			if receipt.Status != "pending_verification" {
				t.Fatalf("missing original intent: %+v", receipt)
			}
			operation, err := f.store.Operation(ctx, f.c.Identity(), requestID)
			require(t, err)
			snapshot, err := f.store.Query(ctx, f.c.Identity())
			require(t, err)
			facts := executionmodel.CandidateDiscardFacts(snapshot.WorkflowRuns, snapshot.Executions, "8")
			unmerged := f.proof(t, f.first, 81, "open")
			intent := taskmodel.DiscardIntent{TaskID: "8", ExpectedVersion: graph.ByID["8"].Version, Reason: "stop comparing", RequestedBy: "operator"}
			cleanup := uniqueSorted(append(facts.CleanupPending, "pull_request_close:"+unmerged.URL))
			after, err := f.commands.taskResults.Discard(graph.ByID["8"], intent, requestID, cleanup, now())
			require(t, err)
			discard := storageports.DiscardCommit{Before: graph.ByID["8"], After: after, Intent: intent, AuthorityVersion: graph.Authority.AuthorityVersion, GraphRevision: graph.Authority.GraphRevision, Facts: facts, Known: []executionmodel.VerifiedCandidatePR{{WorkflowID: f.first.RunID, URL: unmerged.URL, Info: unmerged}}, Operation: operation}
			f.proof(t, f.first, 81, "merged")
			delivery := captureVerifiedResult(t, f)
			var deliveryErr, discardErr error
			if order == "delivery_first" {
				_, deliveryErr = f.store.CommitTaskResult(ctx, delivery)
				require(t, deliveryErr)
				_, discardErr = f.store.CommitTaskDiscard(ctx, discard)
			} else if order == "discard_first" {
				_, discardErr = f.store.CommitTaskDiscard(ctx, discard)
				require(t, discardErr)
				_, deliveryErr = f.store.CommitTaskResult(ctx, delivery)
			} else {
				var wg sync.WaitGroup
				start := make(chan struct{})
				wg.Add(2)
				go func() { defer wg.Done(); <-start; _, deliveryErr = f.store.CommitTaskResult(ctx, delivery) }()
				go func() { defer wg.Done(); <-start; _, discardErr = f.store.CommitTaskDiscard(ctx, discard) }()
				close(start)
				wg.Wait()
			}
			if (deliveryErr == nil) == (discardErr == nil) {
				t.Fatalf("race did not choose exactly one conclusion: delivery=%v discard=%v", deliveryErr, discardErr)
			}
			loser := deliveryErr
			if loser == nil {
				loser = discardErr
			}
			if !isNativeErrorCode(loser, "stale_version", "task_discarded", "already_merged") {
				t.Fatalf("loser failed without version arbitration: %v", loser)
			}
			final, err := f.store.TaskGraphSnapshot(ctx, f.c.Identity())
			require(t, err)
			status := final.ByID["8"].Status
			if status != "closed" && status != "discarded" || (status == "closed") != (len(final.Deliveries) == 1) || final.ByID["8"].Version != graph.ByID["8"].Version+1 || final.Authority.GraphRevision != graph.Authority.GraphRevision+1 {
				t.Fatal("race partially committed task, unique result or graph revision")
			}
			t.Logf("arbitrated result: status=%s delivery_error=%v discard_error=%v", status, deliveryErr, discardErr)
		})
	}
}
