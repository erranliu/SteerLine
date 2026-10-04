package steerline

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
	tasklegacy "github.com/erranliu/SteerLine/internal/steerline/task"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type fixtureTx interface {
	AcceptDecisionAnswer(command storageports.CommandRecord, answer executionmodel.DecisionAnswer) (executionmodel.DecisionRecord, bool, error)
	AdvanceTaskGraphRevision(project string, expectedAuthorityVersion, expectedGraphRevision uint64) (uint64, error)
	AppendEvent(v storageports.DomainEvent) (int64, error)
	Attempt(ctx context.Context, project, id, execution string) (executionmodel.StepAttemptRecord, error)
	BumpOpenTaskVersionCAS(next taskmodel.TaskRecord, expectedVersion uint64) error
	DeliveryCount(ctx context.Context, project, task string) (int, error)
	EnqueueCommand(v storageports.CommandRecord) (storageports.CommandRecord, bool, error)
	Execution(ctx context.Context, project, id, workflow string) (executionmodel.ExecutionRecord, error)
	LookupCommand(ctx context.Context, project, key string) (storageports.CommandRecord, bool, error)
	NativeAuthority(ctx context.Context, project string) (tasklegacy.ProjectTaskAuthority, error)
	OccupiedExecutions(ctx context.Context, project string) (int, error)
	PRBindingsOnOtherTasks(ctx context.Context, project, task, url string) (int, error)
	PutDecision(v executionmodel.DecisionRecord) error
	PutExecution(v executionmodel.ExecutionRecord) error
	PutOperation(v executionmodel.OperationRecord) (executionmodel.OperationRecord, bool, error)
	PutProjectTaskAuthority(v tasklegacy.ProjectTaskAuthority) error
	PutProjection(project string, revision uint64, payload []byte) error
	PutStepAttempt(v executionmodel.StepAttemptRecord) error
	PutTask(v taskmodel.TaskRecord) error
	PutTaskBatch(v taskmodel.TaskBatchRecord, expectedAuthorityVersion, expectedGraphRevision uint64) error
	PutTaskContract(project, taskID string, v taskmodel.TaskContract) error
	PutTaskDelivery(v TaskDeliveryRecord) error
	PutTaskDependency(v taskmodel.TaskDependencyRecord) error
	PutWorkflowRun(v executionmodel.WorkflowRunRecord) error
	Query(ctx context.Context, project string) (DomainSnapshot, error)
	Task(ctx context.Context, project, id string) (taskmodel.TaskRecord, error)
	TaskContract(ctx context.Context, project string, task taskmodel.TaskRecord) (taskmodel.TaskContract, error)
	TaskGraphSnapshot(ctx context.Context, project string) (taskGraphSnapshot, error)
	UpdateCommand(v storageports.CommandRecord) error
	UpdateDiscardedTaskCAS(v taskmodel.TaskRecord, expected uint64) error
	UpdateNativeTaskCAS(v taskmodel.TaskRecord, expectedVersion uint64) error
	UpdateOperation(v executionmodel.OperationRecord) error
	UpdateProjectTaskAuthorityCAS(v tasklegacy.ProjectTaskAuthority, expectedVersion uint64) error
	Workflow(ctx context.Context, project, id, task string) (executionmodel.WorkflowRunRecord, error)
}

type fixtureTransaction struct {
	fixtureTx
}

type sqliteFixture struct {
	persistence.Connection
	db *sql.DB
}

func fixtureStore(conn persistence.Connection, path string) *sqliteFixture {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		panic(err)
	}
	db.SetMaxIdleConns(0)
	return &sqliteFixture{Connection: conn, db: db}
}

func openSQLiteFixture(path string) (*sqliteFixture, error) {
	conn, err := sqlite.OpenSQLiteStore(path)
	if err != nil {
		return nil, err
	}
	return fixtureStore(conn, path), nil
}

func openSQLiteReaderFixture(path string) (*sqliteFixture, error) {
	conn, err := sqlite.OpenSQLiteReader(path)
	if err != nil {
		return nil, err
	}
	return fixtureStore(conn, path), nil
}

func (s *sqliteFixture) Close() error { return errorsJoin(s.db.Close(), s.Connection.Close()) }

func errorsJoin(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

func (s *sqliteFixture) transactRecords(ctx context.Context, fn func(*fixtureTransaction) error) error {
	return s.TransactClaim(ctx, func(tx persistence.ClaimTx) error {
		return fn(&fixtureTransaction{fixtureTx: tx.(fixtureTx)})
	})
}

func testSQLite(s *Service) *sqliteFixture {
	return fixtureStore(s.domainStore.(persistence.Connection), filepath.Join(s.Config.RunDir, "steerline.db"))
}

func openDomainTestStore(t *testing.T) *sqliteFixture {
	t.Helper()
	s, err := openSQLiteFixture(filepath.Join(t.TempDir(), "steerline.db"))
	require(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func transactFixture(s persistence.Connection, ctx context.Context, fn func(*fixtureTransaction) error) error {
	return s.TransactClaim(ctx, func(tx persistence.ClaimTx) error { return fn(&fixtureTransaction{fixtureTx: tx.(fixtureTx)}) })
}
