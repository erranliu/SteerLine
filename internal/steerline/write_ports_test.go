package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/persistence"
)

type commandPortProbe struct {
	persistence.CommandStore
	submitted CommandRecord
	updated   CommandRecord
	failure   error
}

func (p *commandPortProbe) EnqueueCommand(_ context.Context, v CommandRecord) (CommandRecord, bool, error) {
	p.submitted = v
	return v, true, p.failure
}
func (p *commandPortProbe) UpdateCommand(_ context.Context, v CommandRecord) error {
	p.updated = v
	return p.failure
}

func TestCM69CommandPortUsesFixedProjectAndPropagatesRejection(t *testing.T) {
	rejected := errors.New("rejected request")
	port := &commandPortProbe{failure: rejected}
	service := CommandService{Store: port, Project: "fixed-project"}
	original := CommandRecord{ProjectIdentity: "untrusted", CommandID: "original-request", IdempotencyKey: "same-request", ExpectedVersion: 7, Payload: []byte(`{"value":1}`)}
	_, _, err := service.Submit(context.Background(), original)
	if !errors.Is(err, rejected) || port.submitted.ProjectIdentity != "fixed-project" || port.submitted.CommandID != original.CommandID || port.submitted.IdempotencyKey != original.IdempotencyKey || port.submitted.ExpectedVersion != 7 || string(port.submitted.Payload) != string(original.Payload) {
		t.Fatal("command port lost fixed project, request identity, precondition or rejection")
	}
	result := []byte("receipt")
	if err = service.Complete(context.Background(), original, result); !errors.Is(err, rejected) {
		t.Fatal("command completion concealed rejection")
	}
	result[0] = 'X'
	if port.updated.ProjectIdentity != "fixed-project" || port.updated.Status != "completed" || string(port.updated.Result) != "receipt" {
		t.Fatal("completion lost identity or shared its caller's receipt bytes")
	}
}

type workerPortProbe struct {
	persistence.Metadata
	request persistence.WorkerCommit
}

func (p *workerPortProbe) CommitWorker(_ context.Context, v persistence.WorkerCommit) (persistence.WorkerCommitResult, error) {
	p.request = v
	return persistence.WorkerCommitResult{}, nativeErr("stale_revision", "original Attempt changed")
}

func TestCM69WorkerPortRejectsCandidateWithoutPublishing(t *testing.T) {
	service, run := neutralAuditService(t)
	port := &workerPortProbe{}
	service.store = &Store{Domain: port, Project: service.Config.identity}
	previous := recordFingerprint(service.committed)
	revision := service.State.Revision
	service.executionRecord(run).LastEvent = ptr("uncommitted candidate")
	service.note(run, "steer", "worker", "observed", "candidate event", "", nil)
	if err := service.commitWorker(run); !isNativeErrorCode(err, "stale_revision") {
		t.Fatalf("rejection was not returned: %v", err)
	}
	request := port.request
	if request.Project != service.Config.identity || request.WorkflowID != run.RunID || request.ExecutionID != run.ExecutionID || request.ExpectedAttemptID != run.AttemptID || request.Execution == nil || request.Execution.Before == nil || deref(request.Execution.After.LastEvent) != "uncommitted candidate" || len(request.Events) == 0 || request.Checkpoint.Repository.Identity != service.Config.identity {
		t.Fatal("worker port did not receive the original identity, Before/After and atomic event/checkpoint declaration")
	}
	if recordFingerprint(service.committed) != previous || service.State.Revision != revision || deref(service.executionRecord(run).LastEvent) == "uncommitted candidate" || len(service.pendingEvents) == 0 {
		t.Fatal("rejected port request published a candidate, advanced revision or acknowledged its event")
	}
}

func TestCM69WorkerCommitRejectsSupersededAttempt(t *testing.T) {
	service, run := neutralAuditService(t)
	db := testSQLite(service)
	ctx := context.Background()
	original := run.AttemptID
	old := *service.workflowAttempt(run, original)
	next := old
	old.Status = "stopped"
	next.AttemptID, next.AttemptNumber, next.Status = newID(), old.AttemptNumber+1, "planned"
	execution := *service.executionRecord(run)
	execution.CurrentAttemptID = next.AttemptID
	require(t, db.transactSQLite(ctx, func(tx *sqliteTx) error {
		if err := tx.PutStepAttempt(old); err != nil {
			return err
		}
		if err := tx.PutStepAttempt(next); err != nil {
			return err
		}
		return tx.PutExecution(execution)
	}))
	current, err := db.Query(ctx, service.Config.identity)
	require(t, err)
	before, revision, err := db.LoadProjection(ctx, service.Config.identity)
	require(t, err)
	workflow := snapshotWorkflow(&current, run.RunID)
	// Use the current control version so the rejection proves the transaction's
	// Attempt identity check rather than an earlier version check.
	request := workerCommit{Project: service.Config.identity, TaskID: run.DomainTaskID, WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: original, ExpectedAttemptID: original, ControlRevision: workflow.ControlRevision, Checkpoint: runtimeCheckpoint(service.State)}
	_, err = service.store.Domain.CommitWorker(ctx, request)
	if !isNativeErrorCode(err, "stale_revision") || !strings.Contains(err.Error(), "attempt changed") {
		t.Fatalf("superseded Attempt was not rejected inside commitment: %v", err)
	}
	after, nextRevision, err := db.LoadProjection(ctx, service.Config.identity)
	require(t, err)
	if string(after) != string(before) || nextRevision != revision {
		t.Fatal("superseded Attempt advanced its checkpoint")
	}
}

func TestCM69CleanupReceiptKeepsTaskAndReceiptAtomic(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "commit"
		if fail {
			name = "receipt_failure"
		}
		t.Run(name, func(t *testing.T) {
			service, run := neutralAuditService(t)
			db := testSQLite(service)
			ctx := context.Background()
			project := service.Config.identity
			workspace := filepath.Join(service.Config.RunDir, "worktrees", "absent-cleanup")
			execution := *service.executionRecord(run)
			execution.Status = "stopped"
			execution.Target.ID, execution.Target.WorktreeID = project, workspace
			execution.Target.Branch, execution.Target.BaseRevision = "fixture/cleanup", execution.Workspace.Base
			execution.Workspace.Worktree = workspace
			require(t, db.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.PutExecution(execution) }))
			commands := CommandService{Store: db, Project: project}
			task := nativeTaskRecord(service.domain.Tasks, run.TaskID)
			discarded, err := commands.DiscardNativeTask(ctx, service.Config, task.TaskID, newID(), task.Version, "cleanup fixture", "operator")
			require(t, err)
			if discarded.Status != "discarded" {
				t.Fatal("task was not discarded")
			}
			before, err := db.TaskGraphSnapshot(ctx, project)
			require(t, err)
			current := before.ByID[task.TaskID]
			item := "workspace_cleanup:" + workspace
			evidence := NativeWorkspaceCleanupReceipt{WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, Repository: project, Worktree: workspace, Branch: execution.Target.Branch, BaseCommit: execution.Target.BaseRevision, WorktreeGone: true, BranchGone: true, ObservedAt: time.Now().UTC()}
			request := newID()
			if fail {
				_, err = db.db.Exec(`CREATE TRIGGER reject_cleanup_receipt BEFORE INSERT ON commands BEGIN SELECT RAISE(FAIL,'cleanup receipt failure'); END`)
				require(t, err)
			}
			receipt, err := commands.RecordNativeCleanupReceipt(ctx, task.TaskID, request, current.Version, item, "operator", mustJSON(evidence))
			after, readErr := db.TaskGraphSnapshot(ctx, project)
			require(t, readErr)
			if fail {
				if err == nil || after.Authority.GraphRevision != before.Authority.GraphRevision || recordFingerprint(after.ByID[task.TaskID]) != recordFingerprint(current) {
					t.Fatal("failed cleanup receipt changed its task or graph")
				}
				if _, found, lookupErr := db.LookupCommand(ctx, project, request); lookupErr != nil || found {
					t.Fatal("failed cleanup published a receipt")
				}
				snapshot, readErr := db.Query(ctx, project)
				require(t, readErr)
				for _, op := range snapshot.Operations {
					if op.OperationID == request {
						t.Fatal("failed cleanup retained its operation receipt")
					}
				}
				return
			}
			require(t, err)
			if receipt.Status != "discarded" || receipt.Version != current.Version+1 || hasString(receipt.CleanupPending, item) || after.Authority.GraphRevision != before.Authority.GraphRevision+1 {
				t.Fatal("cleanup did not settle exactly its pending item with graph and task CAS")
			}
			command, err := db.Command(ctx, project, request)
			require(t, err)
			var saved TaskRecord
			require(t, json.Unmarshal(command.Result, &saved))
			if command.Status != "accepted" || recordFingerprint(saved) != recordFingerprint(receipt) {
				t.Fatal("cleanup receipt differs from its committed task")
			}
			replay, err := commands.RecordNativeCleanupReceipt(ctx, task.TaskID, request, current.Version, item, "operator", mustJSON(evidence))
			require(t, err)
			if recordFingerprint(replay) != recordFingerprint(receipt) {
				t.Fatal("cleanup replay did not preserve its original result")
			}
		})
	}
}

func TestCM69WriteConsumersDoNotAccessConcreteTransactions(t *testing.T) {
	// Storage, project connection ownership and read adapters are separate owners.
	files := []string{"query_command.go", "service.go", "service_commit.go", "store.go", "command_processor.go", "control_service.go", "native_service.go", "native_batch_command.go", "native_claim_command.go", "native_dispatch_command.go", "native_task_predecessors.go", "native_discard_command.go", "native_delivery_command.go", "native_cleanup.go", "worker_start.go", "worker_recovery.go", "conversation_service.go", "conversation_sources.go", "conversation_replay.go", "conversation_position.go"}
	for _, name := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(".", name), nil, 0)
		require(t, err)
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch v := node.(type) {
			case *ast.ImportSpec:
				if v.Path.Value == `"database/sql"` {
					t.Errorf("%s imports concrete SQL", name)
				}
			case *ast.Ident:
				if v.Name == "SQLiteStore" || v.Name == "sqliteTx" || v.Name == "DomainTx" || v.Name == "DomainStore" {
					t.Errorf("%s refers to %s", name, v.Name)
				}
			case *ast.SelectorExpr:
				for _, forbidden := range []string{"transactSQLite", "Transact", "QueryRowContext", "ExecContext"} {
					if v.Sel.Name == forbidden {
						t.Errorf("%s uses broad/concrete transaction %s", name, v.Sel.Name)
					}
				}
				if v.Sel.Name == "db" || v.Sel.Name == "tx" {
					t.Errorf("%s accesses storage internals", name)
				}
			}
			return true
		})
	}
	// Only named, operation-specific callback interfaces cross the write boundary.
	parsed, err := parser.ParseFile(token.NewFileSet(), "persistence/ports.go", nil, 0)
	require(t, err)
	ast.Inspect(parsed, func(node ast.Node) bool {
		if v, ok := node.(*ast.Ident); ok && strings.HasPrefix(v.Name, "Domain") && v.Name != "DomainEvent" && v.Name != "DomainSnapshot" {
			t.Errorf("write contract exports a domain writer: %s", v.Name)
		}
		return true
	})
}
