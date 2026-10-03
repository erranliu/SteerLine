package steerline

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
)

type fixtureTx interface {
	AcceptDecisionAnswer(command CommandRecord, answer DecisionAnswer) (DecisionRecord, bool, error)
	AdvanceTaskGraphRevision(project string, expectedAuthorityVersion, expectedGraphRevision uint64) (uint64, error)
	AppendEvent(v DomainEvent) (int64, error)
	Attempt(ctx context.Context, project, id, execution string) (StepAttemptRecord, error)
	BumpOpenTaskVersionCAS(next TaskRecord, expectedVersion uint64) error
	DeliveryCount(ctx context.Context, project, task string) (int, error)
	EnqueueCommand(v CommandRecord) (CommandRecord, bool, error)
	Execution(ctx context.Context, project, id, workflow string) (ExecutionRecord, error)
	LookupCommand(ctx context.Context, project, key string) (CommandRecord, bool, error)
	NativeAuthority(ctx context.Context, project string) (ProjectTaskAuthority, error)
	OccupiedExecutions(ctx context.Context, project string) (int, error)
	PRBindingsOnOtherTasks(ctx context.Context, project, task, url string) (int, error)
	PutDecision(v DecisionRecord) error
	PutExecution(v ExecutionRecord) error
	PutOperation(v OperationRecord) (OperationRecord, bool, error)
	PutProjectTaskAuthority(v ProjectTaskAuthority) error
	PutProjection(project string, revision uint64, payload []byte) error
	PutStepAttempt(v StepAttemptRecord) error
	PutTask(v TaskRecord) error
	PutTaskBatch(v TaskBatchRecord, expectedAuthorityVersion, expectedGraphRevision uint64) error
	PutTaskContract(project, taskID string, v TaskContract) error
	PutTaskDelivery(v TaskDeliveryRecord) error
	PutTaskDependency(v TaskDependencyRecord) error
	PutWorkflowRun(v WorkflowRunRecord) error
	Query(ctx context.Context, project string) (DomainSnapshot, error)
	Task(ctx context.Context, project, id string) (TaskRecord, error)
	TaskContract(ctx context.Context, project string, task TaskRecord) (TaskContract, error)
	TaskGraphSnapshot(ctx context.Context, project string) (taskGraphSnapshot, error)
	UpdateCommand(v CommandRecord) error
	UpdateDiscardedTaskCAS(v TaskRecord, expected uint64) error
	UpdateNativeTaskCAS(v TaskRecord, expectedVersion uint64) error
	UpdateOperation(v OperationRecord) error
	UpdateProjectTaskAuthorityCAS(v ProjectTaskAuthority, expectedVersion uint64) error
	Workflow(ctx context.Context, project, id, task string) (WorkflowRunRecord, error)
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
