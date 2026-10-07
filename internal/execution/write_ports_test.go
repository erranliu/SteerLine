package execution

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type commandPortProbe struct {
	CommandStore
	submitted storageports.CommandRecord
	updated   storageports.CommandRecord
	failure   error
}

func (p *commandPortProbe) EnqueueCommand(_ context.Context, v storageports.CommandRecord) (storageports.CommandRecord, bool, error) {
	p.submitted = v
	return v, true, p.failure
}

func (p *commandPortProbe) UpdateCommand(_ context.Context, v storageports.CommandRecord) error {
	p.updated = v
	return p.failure
}

func TestCM69CommandPortUsesFixedProjectAndPropagatesRejection(t *testing.T) {
	rejected := errors.New("rejected request")
	port := &commandPortProbe{failure: rejected}
	service := newTestCommandService(port, Config{identity: "fixed-project"})
	original := storageports.CommandRecord{ProjectIdentity: "untrusted", CommandID: "original-request", IdempotencyKey: "same-request", ExpectedVersion: 7, Payload: []byte(`{"value":1}`)}
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
	storageports.Metadata
	request storageports.WorkerCommit
}

func (p *workerPortProbe) CommitWorker(_ context.Context, v storageports.WorkerCommit) (storageports.WorkerCommitResult, error) {
	p.request = v
	return storageports.WorkerCommitResult{}, nativeErr("stale_revision", "original Attempt changed")
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
	require(t, transactFixture(db.SQLiteStore, ctx, func(tx *fixtureTransaction) error {
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
	var workflow *executionmodel.WorkflowRunRecord
	for i := range current.WorkflowRuns {
		if current.WorkflowRuns[i].WorkflowRunID == run.RunID {
			workflow = &current.WorkflowRuns[i]
			break
		}
	}
	if workflow == nil {
		t.Fatal("authoritative workflow is missing")
	}
	// Use the current control version so the rejection proves the transaction's
	// Attempt identity check rather than an earlier version check.
	request := storageports.WorkerCommit{Project: service.Config.identity, TaskID: run.DomainTaskID, WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: original, ExpectedAttemptID: original, ControlRevision: workflow.ControlRevision, Checkpoint: runtimeCheckpoint(service.State)}
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

func TestCM69WriteConsumersDoNotAccessConcreteTransactions(t *testing.T) {
	// Storage, project connection ownership and read adapters are separate owners.
	files := []string{"store.go", "command_processor.go", "control_service.go", "native_claim_command.go", "native_dispatch_command.go", "native_discard_command.go", "native_delivery_command.go", "native_cleanup.go", "worker_start.go", "worker_recovery.go", "conversation_service.go", "conversation_sources.go", "conversation_replay.go", "conversation_position.go"}
	// Follow the split service and the task use cases at their current owners.
	// Storage and read adapters remain outside this consumer boundary.
	for _, pattern := range []string{"service_*.go", "../task/*.go"} {
		owned, err := filepath.Glob(pattern)
		require(t, err)
		if len(owned) == 0 {
			t.Fatalf("write consumer owner has no source files: %s", pattern)
		}
		for _, name := range owned {
			if !strings.HasSuffix(name, "_test.go") {
				files = append(files, name)
			}
		}
	}
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
	var contracts []string
	owned, err := filepath.Glob("../storage/ports/*.go")
	require(t, err)
	if len(owned) == 0 {
		t.Fatal("pure storage ports have no source files")
	}
	for _, name := range owned {
		if !strings.HasSuffix(name, "_test.go") {
			contracts = append(contracts, name)
		}
	}
	for _, name := range contracts {
		parsed, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		require(t, err)
		ast.Inspect(parsed, func(node ast.Node) bool {
			if v, ok := node.(*ast.Ident); ok && strings.HasPrefix(v.Name, "Domain") && v.Name != "DomainEvent" {
				t.Errorf("%s exports a domain writer: %s", name, v.Name)
			}
			return true
		})
	}
}
