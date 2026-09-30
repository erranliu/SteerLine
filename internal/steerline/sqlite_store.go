package steerline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	_ "modernc.org/sqlite"
)

type DomainTx interface {
	PutTask(TaskRecord) error
	PutWorkflowRun(WorkflowRunRecord) error
	PutExecution(ExecutionRecord) error
	PutStepAttempt(StepAttemptRecord) error
	PutDecision(DecisionRecord) error
	AppendEvent(DomainEvent) (int64, error)
	PutOperation(OperationRecord) (OperationRecord, bool, error)
	UpdateOperation(OperationRecord) error
	EnqueueCommand(CommandRecord) (CommandRecord, bool, error)
	UpdateCommand(CommandRecord) error
	AcceptDecisionAnswer(CommandRecord, DecisionAnswer) (DecisionRecord, bool, error)
	PutProjection(project string, revision uint64, payload []byte) error
	PutProjectTaskAuthority(ProjectTaskAuthority) error
	UpdateProjectTaskAuthorityCAS(ProjectTaskAuthority, uint64) error
	PutTaskContract(string, string, TaskContract) error
	PutTaskBatch(TaskBatchRecord, uint64, uint64) error
	PutTaskDependency(TaskDependencyRecord) error
	PutTaskDelivery(TaskDeliveryRecord) error
	UpdateNativeTaskCAS(TaskRecord, uint64) error
	AdvanceTaskGraphRevision(string, uint64, uint64) (uint64, error)
}

type DomainStore interface {
	Close() error
	Transact(context.Context, func(DomainTx) error) error
	LoadProjection(context.Context, string) ([]byte, uint64, error)
	Query(context.Context, string) (DomainSnapshot, error)
	Events(context.Context, string, string, string, int64, int) ([]DomainEvent, error)
	EventPayload(context.Context, string, int64, int64, int) ([]byte, int64, string, error)
	Commands(context.Context, string, string, int) ([]CommandRecord, error)
	Command(context.Context, string, string) (CommandRecord, error)
	CommandByIdempotency(context.Context, string, string) (CommandRecord, error)
	NativeTaskPage(context.Context, string, string, string, uint64, int, int) (NativeTaskPage, error)
}

type DomainSnapshot struct {
	Deliveries      []TaskDeliveryRecord `json:"deliveries"`
	ProjectIdentity string               `json:"project_identity"`
	Tasks           []TaskRecord         `json:"tasks"`
	WorkflowRuns    []WorkflowRunRecord  `json:"workflow_runs"`
	Executions      []ExecutionRecord    `json:"executions"`
	Attempts        []StepAttemptRecord  `json:"attempts"`
	Decisions       []DecisionRecord     `json:"decisions"`
	Operations      []OperationRecord    `json:"operations"`
}

type SQLiteStore struct {
	db            *sql.DB
	readOnly      bool
	nativeBinding *ProjectTaskAuthority
}

func OpenSQLiteStore(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, errors.New("sqlite path is required")
	}
	createNew := false
	if _, err := os.Stat(path); err == nil {
		if err = validateExistingDatabase(path); err != nil {
			return nil, err
		}
	} else if os.IsNotExist(err) {
		createNew = true
	} else {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQLite store: %w", err)
	}
	db.SetMaxOpenConns(1)
	// Closing idle connections avoids keeping runtime directories undeletable on
	// Windows when a caller forgets to close an otherwise idle Service.
	db.SetMaxIdleConns(0)
	store := &SQLiteStore{db: db}
	if err := store.ensureSchema(context.Background(), createNew); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func OpenSQLiteReader(path string) (*SQLiteStore, error) {
	reader, err := openSQLiteInspector(path)
	if err != nil {
		return nil, err
	}
	if err = validateDatabaseSchema(context.Background(), reader.db); err != nil {
		_ = reader.Close()
		return nil, err
	}
	return reader, nil
}

// openSQLiteInspector reads only metadata needed to reject an unsupported file.
// Callers must validate the current schema before exposing business data.
func openSQLiteInspector(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, errors.New("sqlite path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(abs)
	if runtime.GOOS == "windows" {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	db, err := sql.Open("sqlite", u.String()+"?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	if err = db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &SQLiteStore{db: db, readOnly: true}, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) Transact(ctx context.Context, fn func(DomainTx) error) error {
	if s.readOnly {
		return errors.New("SQLite reader is read-only")
	}
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(&sqliteTx{ctx: ctx, tx: tx}); err != nil {
		return err
	}
	if err = validateNativeTaskStructure(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE database_state SET revision=revision+1 WHERE singleton=1`); err != nil {
		return err
	}
	return tx.Commit()
}
