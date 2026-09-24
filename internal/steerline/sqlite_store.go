package steerline

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const domainSchemaVersion = 1

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
	PutProjection(project string, revision uint64, payload []byte) error
}

type DomainStore interface {
	Close() error
	Transact(context.Context, func(DomainTx) error) error
	LoadProjection(context.Context, string) ([]byte, uint64, error)
	Query(context.Context, string) (DomainSnapshot, error)
	Events(context.Context, string, string, string, int64, int) ([]DomainEvent, error)
	EventPayload(context.Context, string, int64, int64, int) ([]byte, int64, string, error)
	Commands(context.Context, string, string, int) ([]CommandRecord, error)
}

type DomainSnapshot struct {
	ProjectIdentity string              `json:"project_identity"`
	Tasks           []TaskRecord        `json:"tasks"`
	WorkflowRuns    []WorkflowRunRecord `json:"workflow_runs"`
	Executions      []ExecutionRecord   `json:"executions"`
	Attempts        []StepAttemptRecord `json:"attempts"`
	Decisions       []DecisionRecord    `json:"decisions"`
}

type SQLiteStore struct {
	db       *sql.DB
	readOnly bool
}

func OpenSQLiteStore(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, errors.New("sqlite path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQLite store: %w", err)
	}
	db.SetMaxOpenConns(1)
	// Closing idle connections avoids keeping runtime directories undeletable on
	// Windows when a caller forgets to close an otherwise idle Service.
	db.SetMaxIdleConns(0)
	store := &SQLiteStore{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func OpenSQLiteReader(path string) (*SQLiteStore, error) {
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

func (s *SQLiteStore) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	var version int
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version)
	if err != nil {
		return err
	}
	if version > domainSchemaVersion {
		return fmt.Errorf("SQLite schema %d is newer than supported schema %d", version, domainSchemaVersion)
	}
	if version < 1 {
		for _, stmt := range domainSchemaV1 {
			if _, err = tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("create domain schema: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(1, ?)`, now().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var domainSchemaV1 = []string{
	`CREATE TABLE tasks (project_identity TEXT NOT NULL, task_id TEXT NOT NULL, external_provider TEXT NOT NULL DEFAULT '', external_project TEXT NOT NULL DEFAULT '', external_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, version INTEGER NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(project_identity, task_id), UNIQUE(project_identity, external_provider, external_project, external_id))`,
	`CREATE TABLE workflow_runs (project_identity TEXT NOT NULL, workflow_run_id TEXT NOT NULL, task_id TEXT NOT NULL, phase TEXT NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(project_identity, workflow_run_id), FOREIGN KEY(project_identity, task_id) REFERENCES tasks(project_identity, task_id))`,
	`CREATE INDEX workflow_runs_task_idx ON workflow_runs(project_identity, task_id)`,
	`CREATE TABLE executions (project_identity TEXT NOT NULL, execution_id TEXT NOT NULL, workflow_run_id TEXT NOT NULL, status TEXT NOT NULL, current_step TEXT NOT NULL DEFAULT '', target_json BLOB NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(project_identity, execution_id), FOREIGN KEY(project_identity, workflow_run_id) REFERENCES workflow_runs(project_identity, workflow_run_id))`,
	`CREATE INDEX executions_workflow_idx ON executions(project_identity, workflow_run_id)`,
	`CREATE TABLE step_attempts (project_identity TEXT NOT NULL, attempt_id TEXT NOT NULL, execution_id TEXT NOT NULL, step_id TEXT NOT NULL, attempt_number INTEGER NOT NULL, operation_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(project_identity, attempt_id), UNIQUE(project_identity, execution_id, step_id, attempt_number), FOREIGN KEY(project_identity, execution_id) REFERENCES executions(project_identity, execution_id))`,
	`CREATE UNIQUE INDEX one_active_attempt_per_step ON step_attempts(project_identity, execution_id, step_id) WHERE status IN ('planned','dispatching','dispatched','running','waiting_decision','stopping','outcome_unknown')`,
	`CREATE TABLE decisions (project_identity TEXT NOT NULL, decision_id TEXT NOT NULL, scope TEXT NOT NULL, workflow_run_id TEXT, execution_id TEXT, attempt_id TEXT, question_id TEXT NOT NULL, status TEXT NOT NULL, version INTEGER NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(project_identity, decision_id), FOREIGN KEY(project_identity, workflow_run_id) REFERENCES workflow_runs(project_identity, workflow_run_id), FOREIGN KEY(project_identity, execution_id) REFERENCES executions(project_identity, execution_id), FOREIGN KEY(project_identity, attempt_id) REFERENCES step_attempts(project_identity, attempt_id))`,
	`CREATE INDEX decisions_scope_idx ON decisions(project_identity, workflow_run_id, execution_id, status)`,
	`CREATE TABLE decision_answers (project_identity TEXT NOT NULL, decision_id TEXT NOT NULL, decision_version INTEGER NOT NULL, command_id TEXT NOT NULL, payload_digest TEXT NOT NULL, created_at TEXT NOT NULL, PRIMARY KEY(project_identity,decision_id,decision_version), FOREIGN KEY(project_identity,decision_id) REFERENCES decisions(project_identity,decision_id))`,
	`CREATE TABLE events (event_id INTEGER PRIMARY KEY AUTOINCREMENT, project_identity TEXT NOT NULL, task_id TEXT NOT NULL DEFAULT '', workflow_run_id TEXT NOT NULL DEFAULT '', execution_id TEXT NOT NULL DEFAULT '', attempt_id TEXT NOT NULL DEFAULT '', external_source TEXT NOT NULL DEFAULT '', external_id TEXT NOT NULL DEFAULT '', type TEXT NOT NULL, received_at TEXT NOT NULL, payload_bytes INTEGER NOT NULL, payload_digest TEXT NOT NULL, payload BLOB NOT NULL)`,
	`CREATE UNIQUE INDEX events_source_identity_idx ON events(project_identity, external_source, external_id) WHERE external_source <> '' AND external_id <> ''`,
	`CREATE INDEX events_execution_idx ON events(project_identity, execution_id, attempt_id, event_id)`,
	`CREATE TABLE operations (project_identity TEXT NOT NULL, operation_id TEXT NOT NULL, idempotency_key TEXT NOT NULL, kind TEXT NOT NULL, status TEXT NOT NULL, payload_digest TEXT NOT NULL, platform_request_id TEXT NOT NULL DEFAULT '', payload BLOB NOT NULL, PRIMARY KEY(project_identity, operation_id), UNIQUE(project_identity, idempotency_key))`,
	`CREATE TABLE commands (project_identity TEXT NOT NULL, command_id TEXT NOT NULL, kind TEXT NOT NULL, target_id TEXT NOT NULL, expected_version INTEGER NOT NULL, idempotency_key TEXT NOT NULL, requested_by TEXT NOT NULL, payload BLOB NOT NULL, status TEXT NOT NULL, result BLOB, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(project_identity, command_id), UNIQUE(project_identity, idempotency_key))`,
	`CREATE INDEX commands_status_idx ON commands(project_identity, status, created_at)`,
	`CREATE TABLE projection (project_identity TEXT PRIMARY KEY, revision INTEGER NOT NULL, payload BLOB NOT NULL, updated_at TEXT NOT NULL)`,
}

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
	return tx.Commit()
}

type sqliteTx struct {
	ctx context.Context
	tx  *sql.Tx
}

func encodePayload(v any) ([]byte, error) { return json.Marshal(v) }

func (t *sqliteTx) PutTask(v TaskRecord) error {
	if v.ProjectIdentity == "" || v.TaskID == "" || v.Status == "" {
		return errors.New("task identity and status are required")
	}
	if !validTaskStatus(v.Status) {
		return fmt.Errorf("invalid task status %q", v.Status)
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO tasks(project_identity,task_id,external_provider,external_project,external_id,status,version,payload) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(project_identity,task_id) DO UPDATE SET external_provider=excluded.external_provider,external_project=excluded.external_project,external_id=excluded.external_id,status=excluded.status,version=excluded.version,payload=excluded.payload`, v.ProjectIdentity, v.TaskID, v.ExternalProvider, v.ExternalProject, v.ExternalID, v.Status, v.Version, b)
	return err
}
func (t *sqliteTx) PutWorkflowRun(v WorkflowRunRecord) error {
	if v.ProjectIdentity == "" || v.WorkflowRunID == "" || v.TaskID == "" || v.Phase == "" {
		return errors.New("workflow run identity, task and phase are required")
	}
	if !validWorkflowPhase(v.Phase) {
		return fmt.Errorf("invalid workflow run phase %q", v.Phase)
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO workflow_runs(project_identity,workflow_run_id,task_id,phase,payload) VALUES(?,?,?,?,?) ON CONFLICT(project_identity,workflow_run_id) DO UPDATE SET task_id=excluded.task_id,phase=excluded.phase,payload=excluded.payload`, v.ProjectIdentity, v.WorkflowRunID, v.TaskID, v.Phase, b)
	return err
}
func (t *sqliteTx) PutExecution(v ExecutionRecord) error {
	if v.ProjectIdentity == "" || v.ExecutionID == "" || v.WorkflowRunID == "" || v.Status == "" {
		return errors.New("execution identity, workflow and status are required")
	}
	if !validExecutionStatus(v.Status) {
		return fmt.Errorf("invalid execution status %q", v.Status)
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	target, err := encodePayload(v.Target)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO executions(project_identity,execution_id,workflow_run_id,status,current_step,target_json,payload) VALUES(?,?,?,?,?,?,?) ON CONFLICT(project_identity,execution_id) DO UPDATE SET workflow_run_id=excluded.workflow_run_id,status=excluded.status,current_step=excluded.current_step,target_json=excluded.target_json,payload=excluded.payload`, v.ProjectIdentity, v.ExecutionID, v.WorkflowRunID, v.Status, v.CurrentStep, target, b)
	return err
}
func (t *sqliteTx) PutStepAttempt(v StepAttemptRecord) error {
	if v.ProjectIdentity == "" || v.AttemptID == "" || v.ExecutionID == "" || v.StepID == "" || v.AttemptNumber < 1 || v.Status == "" {
		return errors.New("step attempt identity, step, positive number and status are required")
	}
	if !validStepAttemptStatus(v.Status) {
		return fmt.Errorf("invalid step attempt status %q", v.Status)
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO step_attempts(project_identity,attempt_id,execution_id,step_id,attempt_number,operation_id,status,payload) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(project_identity,attempt_id) DO UPDATE SET execution_id=excluded.execution_id,step_id=excluded.step_id,attempt_number=excluded.attempt_number,operation_id=excluded.operation_id,status=excluded.status,payload=excluded.payload`, v.ProjectIdentity, v.AttemptID, v.ExecutionID, v.StepID, v.AttemptNumber, v.OperationID, v.Status, b)
	return err
}
func (t *sqliteTx) PutDecision(v DecisionRecord) error {
	if v.ProjectIdentity == "" || v.DecisionID == "" || v.QuestionID == "" || v.Status == "" || v.Scope == "" {
		return errors.New("decision identity, scope, question and status are required")
	}
	if !validDecisionStatus(v.Status) {
		return fmt.Errorf("invalid decision status %q", v.Status)
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO decisions(project_identity,decision_id,scope,workflow_run_id,execution_id,attempt_id,question_id,status,version,payload) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(project_identity,decision_id) DO UPDATE SET scope=excluded.scope,workflow_run_id=excluded.workflow_run_id,execution_id=excluded.execution_id,attempt_id=excluded.attempt_id,question_id=excluded.question_id,status=excluded.status,version=excluded.version,payload=excluded.payload`, v.ProjectIdentity, v.DecisionID, v.Scope, nullIfEmpty(v.WorkflowRunID), nullIfEmpty(v.ExecutionID), nullIfEmpty(v.AttemptID), v.QuestionID, v.Status, v.Version, b)
	return err
}
func (t *sqliteTx) AppendEvent(v DomainEvent) (int64, error) {
	if v.ProjectIdentity == "" || v.Type == "" {
		return 0, errors.New("event project and type are required")
	}
	if v.ReceivedAt.IsZero() {
		v.ReceivedAt = now()
	}
	if v.PayloadBytes == 0 && len(v.Payload) > 0 {
		v.PayloadBytes = len(v.Payload)
	}
	if v.PayloadDigest == "" && len(v.Payload) > 0 {
		v.PayloadDigest = operationDigest(v.Payload)
	}
	res, err := t.tx.ExecContext(t.ctx, `INSERT INTO events(project_identity,task_id,workflow_run_id,execution_id,attempt_id,external_source,external_id,type,received_at,payload_bytes,payload_digest,payload) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`, v.ProjectIdentity, v.TaskID, v.WorkflowRunID, v.ExecutionID, v.AttemptID, v.ExternalSource, v.ExternalID, v.Type, v.ReceivedAt.UTC().Format(time.RFC3339Nano), v.PayloadBytes, v.PayloadDigest, v.Payload)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 && v.ExternalSource != "" && v.ExternalID != "" {
		err = t.tx.QueryRowContext(t.ctx, `SELECT event_id FROM events WHERE project_identity=? AND external_source=? AND external_id=?`, v.ProjectIdentity, v.ExternalSource, v.ExternalID).Scan(&v.EventID)
		return v.EventID, err
	}
	id, err := res.LastInsertId()
	return id, err
}
func (t *sqliteTx) PutOperation(v OperationRecord) (OperationRecord, bool, error) {
	if v.ProjectIdentity == "" || v.OperationID == "" || v.IdempotencyKey == "" || v.Kind == "" || v.PayloadDigest == "" {
		return v, false, errors.New("operation identity, idempotency key, kind and digest are required")
	}
	if !validOperationStatus(v.Status) {
		return v, false, fmt.Errorf("invalid operation status %q", v.Status)
	}
	var oldID, oldDigest, payload string
	err := t.tx.QueryRowContext(t.ctx, `SELECT operation_id,payload_digest,payload FROM operations WHERE project_identity=? AND idempotency_key=?`, v.ProjectIdentity, v.IdempotencyKey).Scan(&oldID, &oldDigest, &payload)
	if err == nil {
		if oldDigest != v.PayloadDigest {
			return v, false, errors.New("idempotency key reused with different operation content")
		}
		var existing OperationRecord
		if err = json.Unmarshal([]byte(payload), &existing); err != nil {
			return v, false, err
		}
		if oldID == v.OperationID {
			b, marshalErr := encodePayload(v)
			if marshalErr != nil {
				return v, false, marshalErr
			}
			if _, updateErr := t.tx.ExecContext(t.ctx, `UPDATE operations SET status=?,platform_request_id=?,payload=? WHERE project_identity=? AND operation_id=? AND payload_digest=?`, v.Status, v.PlatformRequestID, b, v.ProjectIdentity, v.OperationID, v.PayloadDigest); updateErr != nil {
				return v, false, updateErr
			}
			return v, false, nil
		}
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return v, false, err
	}
	b, err := encodePayload(v)
	if err != nil {
		return v, false, err
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO operations(project_identity,operation_id,idempotency_key,kind,status,payload_digest,platform_request_id,payload) VALUES(?,?,?,?,?,?,?,?)`, v.ProjectIdentity, v.OperationID, v.IdempotencyKey, v.Kind, v.Status, v.PayloadDigest, v.PlatformRequestID, b)
	return v, err == nil, err
}
func (t *sqliteTx) UpdateOperation(v OperationRecord) error {
	if v.ProjectIdentity == "" || v.OperationID == "" || v.IdempotencyKey == "" || v.PayloadDigest == "" || v.Status == "" {
		return errors.New("operation identity, digest and status are required")
	}
	var digest string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT payload_digest FROM operations WHERE project_identity=? AND operation_id=?`, v.ProjectIdentity, v.OperationID).Scan(&digest); err != nil {
		return err
	}
	if digest != v.PayloadDigest {
		return errors.New("operation payload digest cannot change")
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	res, err := t.tx.ExecContext(t.ctx, `UPDATE operations SET status=?,platform_request_id=?,payload=? WHERE project_identity=? AND operation_id=? AND payload_digest=?`, v.Status, v.PlatformRequestID, b, v.ProjectIdentity, v.OperationID, v.PayloadDigest)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return errors.New("operation not found")
	}
	return err
}
func (t *sqliteTx) EnqueueCommand(v CommandRecord) (CommandRecord, bool, error) {
	if v.ProjectIdentity == "" || v.CommandID == "" || v.Kind == "" || v.TargetID == "" || v.IdempotencyKey == "" || v.RequestedBy == "" {
		return v, false, errors.New("command identity, kind, target, idempotency key and requester are required")
	}
	var old CommandRecord
	var payload []byte
	var created, updated string
	err := t.tx.QueryRowContext(t.ctx, `SELECT command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at FROM commands WHERE project_identity=? AND idempotency_key=?`, v.ProjectIdentity, v.IdempotencyKey).Scan(&old.CommandID, &old.Kind, &old.TargetID, &old.ExpectedVersion, &old.IdempotencyKey, &old.RequestedBy, &payload, &old.Status, &old.Result, &created, &updated)
	if err == nil {
		if old.Kind != v.Kind || old.TargetID != v.TargetID || old.ExpectedVersion != v.ExpectedVersion || old.RequestedBy != v.RequestedBy || string(payload) != string(v.Payload) {
			return v, false, errors.New("idempotency key reused with different command content")
		}
		old.ProjectIdentity = v.ProjectIdentity
		old.Payload = payload
		old.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		old.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		return old, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return v, false, err
	}
	if v.Kind == "answer_decision" {
		var version uint64
		var status string
		if err := t.tx.QueryRowContext(t.ctx, `SELECT version,status FROM decisions WHERE project_identity=? AND decision_id=?`, v.ProjectIdentity, v.TargetID).Scan(&version, &status); err != nil {
			return v, false, err
		}
		if version != v.ExpectedVersion || status != "pending" {
			return v, false, fmt.Errorf("decision version conflict: expected %d, current %d, status %s", v.ExpectedVersion, version, status)
		}
		if _, err := t.tx.ExecContext(t.ctx, `INSERT INTO decision_answers(project_identity,decision_id,decision_version,command_id,payload_digest,created_at) VALUES(?,?,?,?,?,?)`, v.ProjectIdentity, v.TargetID, v.ExpectedVersion, v.CommandID, operationDigest(v.Payload), now().Format(time.RFC3339Nano)); err != nil {
			if isConstraintError(err) {
				return v, false, errors.New("decision version already has an accepted answer command")
			}
			return v, false, err
		}
	}
	if v.Status == "" {
		v.Status = "pending"
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now()
	}
	v.UpdatedAt = v.CreatedAt
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO commands(project_identity,command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, v.ProjectIdentity, v.CommandID, v.Kind, v.TargetID, v.ExpectedVersion, v.IdempotencyKey, v.RequestedBy, v.Payload, v.Status, v.Result, v.CreatedAt.UTC().Format(time.RFC3339Nano), v.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return v, err == nil, err
}
func (t *sqliteTx) UpdateCommand(v CommandRecord) error {
	if v.Status == "" {
		return errors.New("command status is required")
	}
	v.UpdatedAt = now()
	res, err := t.tx.ExecContext(t.ctx, `UPDATE commands SET status=?,result=?,updated_at=? WHERE project_identity=? AND command_id=?`, v.Status, v.Result, v.UpdatedAt.Format(time.RFC3339Nano), v.ProjectIdentity, v.CommandID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return errors.New("command not found")
	}
	return err
}
func (t *sqliteTx) PutProjection(project string, revision uint64, payload []byte) error {
	if project == "" || len(payload) == 0 {
		return errors.New("projection identity and payload are required")
	}
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO projection(project_identity,revision,payload,updated_at) VALUES(?,?,?,?) ON CONFLICT(project_identity) DO UPDATE SET revision=excluded.revision,payload=excluded.payload,updated_at=excluded.updated_at`, project, revision, payload, now().Format(time.RFC3339Nano))
	return err
}

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func (s *SQLiteStore) LoadProjection(ctx context.Context, project string) ([]byte, uint64, error) {
	var payload []byte
	var revision uint64
	err := s.db.QueryRowContext(ctx, `SELECT payload,revision FROM projection WHERE project_identity=?`, project).Scan(&payload, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	return payload, revision, err
}

func (s *SQLiteStore) Query(ctx context.Context, project string) (DomainSnapshot, error) {
	return querySnapshot(ctx, s.db, project)
}

type queryContext interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func querySnapshot(ctx context.Context, db queryContext, project string) (DomainSnapshot, error) {
	out := DomainSnapshot{ProjectIdentity: project, Tasks: []TaskRecord{}, WorkflowRuns: []WorkflowRunRecord{}, Executions: []ExecutionRecord{}, Attempts: []StepAttemptRecord{}, Decisions: []DecisionRecord{}}
	queries := []struct {
		sql    string
		decode func([]byte) error
	}{
		{`SELECT payload FROM tasks WHERE project_identity=? ORDER BY task_id`, func(b []byte) error {
			var v TaskRecord
			if err := json.Unmarshal(b, &v); err != nil {
				return err
			}
			out.Tasks = append(out.Tasks, v)
			return nil
		}},
		{`SELECT payload FROM workflow_runs WHERE project_identity=? ORDER BY workflow_run_id`, func(b []byte) error {
			var v WorkflowRunRecord
			if err := json.Unmarshal(b, &v); err != nil {
				return err
			}
			out.WorkflowRuns = append(out.WorkflowRuns, v)
			return nil
		}},
		{`SELECT payload FROM executions WHERE project_identity=? ORDER BY execution_id`, func(b []byte) error {
			var v ExecutionRecord
			if err := json.Unmarshal(b, &v); err != nil {
				return err
			}
			out.Executions = append(out.Executions, v)
			return nil
		}},
		{`SELECT payload FROM step_attempts WHERE project_identity=? ORDER BY execution_id,step_id,attempt_number`, func(b []byte) error {
			var v StepAttemptRecord
			if err := json.Unmarshal(b, &v); err != nil {
				return err
			}
			out.Attempts = append(out.Attempts, v)
			return nil
		}},
		{`SELECT payload FROM decisions WHERE project_identity=? ORDER BY decision_id`, func(b []byte) error {
			var v DecisionRecord
			if err := json.Unmarshal(b, &v); err != nil {
				return err
			}
			out.Decisions = append(out.Decisions, v)
			return nil
		}},
	}
	for _, q := range queries {
		rows, err := db.QueryContext(ctx, q.sql, project)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var b []byte
			if err = rows.Scan(&b); err != nil {
				rows.Close()
				return out, err
			}
			if err = q.decode(b); err != nil {
				rows.Close()
				return out, err
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func (s *SQLiteStore) Events(ctx context.Context, project, execution, attempt string, after int64, limit int) ([]DomainEvent, error) {
	if limit < 1 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	query := `SELECT event_id,project_identity,task_id,workflow_run_id,execution_id,attempt_id,external_source,external_id,type,received_at,payload_bytes,payload_digest,CASE WHEN payload_bytes <= 1048576 THEN payload ELSE X'' END FROM events WHERE project_identity=? AND event_id>?`
	args := []any{project, after}
	if execution != "" {
		query += ` AND execution_id=?`
		args = append(args, execution)
	}
	if attempt != "" {
		query += ` AND attempt_id=?`
		args = append(args, attempt)
	}
	query += ` ORDER BY event_id LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DomainEvent{}
	for rows.Next() {
		var v DomainEvent
		var received string
		if err = rows.Scan(&v.EventID, &v.ProjectIdentity, &v.TaskID, &v.WorkflowRunID, &v.ExecutionID, &v.AttemptID, &v.ExternalSource, &v.ExternalID, &v.Type, &received, &v.PayloadBytes, &v.PayloadDigest, &v.Payload); err != nil {
			return nil, err
		}
		v.ReceivedAt, _ = time.Parse(time.RFC3339Nano, received)
		v.PayloadOmitted = v.PayloadBytes > len(v.Payload)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) EventPayload(ctx context.Context, project string, eventID int64, offset int64, limit int) ([]byte, int64, string, error) {
	if eventID <= 0 || offset < 0 {
		return nil, 0, "", errors.New("event body reference or offset is invalid")
	}
	if limit < 1 {
		limit = 256 << 10
	}
	if limit > 256<<10 {
		limit = 256 << 10
	}
	var total int64
	var digest string
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload_bytes,payload_digest,substr(payload,?,?) FROM events WHERE project_identity=? AND event_id=?`, offset+1, limit, project, eventID).Scan(&total, &digest, &body)
	return body, total, digest, err
}

func (s *SQLiteStore) Commands(ctx context.Context, project, status string, limit int) ([]CommandRecord, error) {
	if limit < 1 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	q := `SELECT command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at FROM commands WHERE project_identity=?`
	args := []any{project}
	if status != "" {
		q += ` AND status=?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CommandRecord{}
	for rows.Next() {
		var v CommandRecord
		var created, updated string
		if err = rows.Scan(&v.CommandID, &v.Kind, &v.TargetID, &v.ExpectedVersion, &v.IdempotencyKey, &v.RequestedBy, &v.Payload, &v.Status, &v.Result, &created, &updated); err != nil {
			return nil, err
		}
		v.ProjectIdentity = project
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SaveRuntime(ctx context.Context, runtime Runtime) (Runtime, error) {
	project := runtime.Repository.Identity
	if project == "" {
		return runtime, errors.New("runtime repository identity is required for SQLite persistence")
	}
	pendingEvents := append([]DomainEvent(nil), runtime.PendingEvents...)
	err := s.Transact(ctx, func(tx DomainTx) error {
		var revision uint64
		if err := tx.(*sqliteTx).tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision),0) FROM projection WHERE project_identity=?`, project).Scan(&revision); err != nil {
			return err
		}
		runtime.Revision = revision + 1
		if runtime.SchemaVersion != 5 {
			return fmt.Errorf("unsupported runtime schema %d; only schema 5 is accepted", runtime.SchemaVersion)
		}
		runtime.Operations = runtimeOperations(runtime)
		for _, run := range runtime.Runs {
			if run == nil {
				continue
			}
			var err error
			if run.WorkflowVersion == MinimalWorkflowVersion {
				err = persistNormalizedWorkflow(tx, project, runtime, run)
			} else {
				err = persistLegacyRun(tx, project, run)
			}
			if err != nil {
				return err
			}
		}
		for _, operation := range runtime.Operations {
			if _, _, err := tx.PutOperation(operation); err != nil {
				return err
			}
		}
		for _, event := range pendingEvents {
			if event.ProjectIdentity == "" {
				event.ProjectIdentity = project
			}
			if _, err := tx.AppendEvent(event); err != nil {
				return err
			}
		}
		snapshot, err := querySnapshot(ctx, tx.(*sqliteTx).tx, project)
		if err != nil {
			return err
		}
		runtime.Tasks = snapshot.Tasks
		runtime.WorkflowRuns = snapshot.WorkflowRuns
		runtime.Executions = snapshot.Executions
		runtime.Attempts = snapshot.Attempts
		runtime.Decisions = snapshot.Decisions
		runtime.ProjectionError = ""
		payload, err := json.Marshal(runtime)
		if err != nil {
			return err
		}
		return tx.PutProjection(project, runtime.Revision, payload)
	})
	if err == nil {
		runtime.PendingEvents = nil
	}
	return runtime, err
}

func persistNormalizedWorkflow(tx DomainTx, project string, runtime Runtime, run *Run) error {
	var task *TaskRecord
	for i := range runtime.Tasks {
		if runtime.Tasks[i].TaskID == run.DomainTaskID {
			task = &runtime.Tasks[i]
			break
		}
	}
	if task == nil {
		return fmt.Errorf("minimal-v1 run %s lacks its Task record", run.RunID)
	}
	var workflow *WorkflowRunRecord
	for i := range runtime.WorkflowRuns {
		if runtime.WorkflowRuns[i].WorkflowRunID == run.RunID {
			workflow = &runtime.WorkflowRuns[i]
			break
		}
	}
	if workflow == nil {
		return fmt.Errorf("minimal-v1 run %s lacks its WorkflowRun record", run.RunID)
	}
	var executionIDs = map[string]bool{}
	for i := range runtime.Executions {
		execution := &runtime.Executions[i]
		if execution.WorkflowRunID != run.RunID {
			continue
		}
		executionIDs[execution.ExecutionID] = true
		if run.FinishTaskStatus == "waiting_decision" {
			execution.Status = "waiting_decision"
		}
		if run.FinishTaskStatus == "waiting_pr_merge" {
			execution.Status = "succeeded"
			execution.CurrentStep = "waiting_pr_merge"
			workflow.SelectedResult = execution.ExecutionID
		}
		if run.FinishTaskStatus == "completed" {
			execution.Status = "succeeded"
			execution.CurrentStep = "completed"
			workflow.Phase = "completed"
		}
		if run.FinishTaskStatus == "blocked" {
			if run.ExecutionStatus == "unknown" {
				execution.Status = "blocked"
			} else {
				execution.Status = "failed"
			}
			workflow.Phase = "blocked"
		}
		if run.FinishTaskStatus == "waiting_decision" {
			workflow.Phase = "waiting_decision"
		}
		if run.FinishTaskStatus == "waiting_pr_merge" {
			workflow.Phase = "waiting_pr_merge"
		}
		if run.ExecutionStatus == "running" && run.FinishTaskStatus == "pending" {
			execution.Status = "active"
			workflow.Phase = "executing"
		}
		execution.UpdatedAt = now()
	}
	if len(executionIDs) == 0 {
		return fmt.Errorf("minimal-v1 workflow %s has no Execution", run.RunID)
	}
	if run.FinishTaskStatus == "completed" && task.Status != "closed" {
		if err := SetTaskStatus(task, "closed", run.PR.State == "merged"); err != nil {
			return err
		}
	}
	workflow.UpdatedAt = now()
	if err := tx.PutTask(*task); err != nil {
		return err
	}
	if err := tx.PutWorkflowRun(*workflow); err != nil {
		return err
	}
	for i := range runtime.Executions {
		execution := runtime.Executions[i]
		if executionIDs[execution.ExecutionID] {
			if err := tx.PutExecution(execution); err != nil {
				return err
			}
		}
	}
	for _, attempt := range runtime.Attempts {
		if executionIDs[attempt.ExecutionID] {
			if err := tx.PutStepAttempt(attempt); err != nil {
				return err
			}
		}
	}
	for _, decision := range runtime.Decisions {
		if decision.WorkflowRunID == run.RunID {
			if err := tx.PutDecision(decision); err != nil {
				return err
			}
		}
	}
	if run.Decision != nil {
		request, err := json.Marshal(run.Decision.Request)
		if err != nil {
			return err
		}
		var answer []byte
		if run.Decision.Status == "answer_saved" || run.Decision.Status == "applied" || run.Decision.Status == "failed" {
			value := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: run.Decision.OptionID, Note: run.Decision.Note, AnsweredBy: run.Decision.AnsweredBy}
			if run.Decision.AnsweredAt != nil {
				value.AnsweredAt = *run.Decision.AnsweredAt
			}
			answer, err = json.Marshal(value)
			if err != nil {
				return err
			}
		}
		status := run.Decision.Status
		if status == "resuming" {
			status = "answer_saved"
		}
		created := now()
		for _, old := range runtime.Decisions {
			if old.DecisionID == run.Decision.ID {
				created = old.CreatedAt
				break
			}
		}
		if err = tx.PutDecision(DecisionRecord{ProjectIdentity: project, DecisionID: run.Decision.ID, Scope: "execution", WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.Decision.AttemptID, QuestionID: run.Decision.ID, Request: request, Answer: answer, AnsweredBy: run.Decision.AnsweredBy, Status: status, Version: run.Decision.Version, CreatedAt: created, UpdatedAt: now()}); err != nil {
			return err
		}
	}
	return nil
}

func persistLegacyRun(tx DomainTx, project string, run *Run) error {
	if run.Host == "" {
		run.Host = "local"
	}
	if run.WorkflowVersion == "" {
		run.WorkflowVersion = "legacy-v1"
	}
	taskID := run.DomainTaskID
	if taskID == "" {
		taskID = legacyTaskID(project, run)
	}
	run.DomainTaskID = taskID
	n := now()
	created := n
	if run.StartedAt != nil {
		created = run.StartedAt.UTC()
	}
	taskStatus := "in_progress"
	switch run.FinishTaskStatus {
	case "completed":
		taskStatus = "closed"
	case "discarded":
		taskStatus = "discarded"
	case "pending", "blocked", "waiting_decision", "waiting_pr_merge":
		if run.BeadsStatus == "open" {
			taskStatus = "open"
		}
	}
	provider, sourceProject := "", ""
	if run.BeadsContext != nil {
		provider = "beads"
		sourceProject = run.BeadsContext.ProjectID
	}
	if err := tx.PutTask(TaskRecord{ProjectIdentity: project, TaskID: taskID, ExternalProvider: provider, ExternalProject: sourceProject, ExternalID: run.TaskID, IdentityMapDigest: run.IdentityMapDigest, Title: run.Title, Claimant: run.Claimant, Source: provider, Status: taskStatus, Version: 1, CreatedAt: created, UpdatedAt: n}); err != nil {
		return err
	}
	phase := "executing"
	switch run.FinishTaskStatus {
	case "waiting_decision":
		phase = "waiting_decision"
	case "waiting_pr_merge":
		phase = "waiting_pr_merge"
	case "completed":
		phase = "completed"
	case "blocked":
		phase = "blocked"
	}
	definition := WorkflowDefinition{ID: "legacy-v1", Version: "legacy-v1"}
	definition.ID, definition.Version = run.WorkflowVersion, run.WorkflowVersion
	wfr := WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: run.RunID, TaskID: taskID, WorkflowVersion: run.WorkflowVersion, Definition: definition, Phase: phase, Repository: run.WorktreePathValue(), Branch: run.WorktreeBranchValue(), CreatedAt: created, UpdatedAt: n}
	if run.PR.URL != nil && run.FinishTaskStatus == "completed" {
		wfr.SelectedResult = run.RunID
	}
	if err := tx.PutWorkflowRun(wfr); err != nil {
		return err
	}
	executionID := legacyExecutionID(run.RunID)
	run.ExecutionID = executionID
	status := "pending"
	switch run.ExecutionStatus {
	case "running":
		status = "active"
	case "succeeded":
		status = "succeeded"
	case "failed":
		status = "failed"
	case "unknown":
		status = "blocked"
	}
	target := run.Target
	if target.ID == "" {
		if run.Host == "orca" {
			target.ID = "local-orca"
		} else {
			target.ID = "local"
		}
	}
	if target.Branch == "" {
		target.Branch = run.WorktreeBranchValue()
	}
	if target.BaseRevision == "" {
		target.BaseRevision = run.BaseCommitValue()
	}
	if target.WorktreeID == "" {
		target.WorktreeID = run.WorktreePathValue()
	}
	run.Target = target
	if run.Host == "orca" && run.Process != nil {
		if target.WorktreeID == "" {
			target.WorktreeID = run.Process.Terminal
		}
		if target.ExternalRuntime == "" {
			target.ExternalRuntime = run.Process.RuntimeID
		}
	}
	workspace := WorkspaceEvidence{Repository: run.WorktreePathValue(), Worktree: run.WorktreePathValue(), Branch: run.WorktreeBranchValue(), Base: run.BaseCommitValue()}
	if run.WorktreePath != nil {
		workspace.VerifiedAt = n
	}
	backend := run.Backend
	if backend == "" {
		backend = run.Host
		if backend == "local" {
			backend = "local_process"
		} else if backend == "orca" {
			backend = "orca"
		}
	}
	run.Backend = backend
	protocol := run.Protocol
	if protocol == "" {
		protocol = legacyProtocol(run.Host)
	}
	run.Protocol = protocol
	exec := ExecutionRecord{ProjectIdentity: project, ExecutionID: executionID, WorkflowRunID: run.RunID, Agent: run.Agent, Backend: backend, Protocol: protocol, Process: run.Process, Target: target, Workspace: workspace, Status: status, CurrentStep: "legacy-workflow", CreatedAt: created, UpdatedAt: n}
	if err := tx.PutExecution(exec); err != nil {
		return err
	}
	attemptID := run.AttemptID
	if attemptID == "" {
		attemptID = run.RunID
	}
	run.AttemptID = attemptID
	attemptStatus := "planned"
	if status == "active" {
		attemptStatus = "running"
	} else if status == "succeeded" {
		attemptStatus = "succeeded"
	} else if status == "failed" {
		attemptStatus = "failed"
	} else if status == "blocked" {
		attemptStatus = "outcome_unknown"
	}
	attemptNumber := run.DomainAttemptNumber
	if attemptNumber < 1 {
		attemptNumber = 1
	}
	run.DomainAttemptNumber = attemptNumber
	attempt := StepAttemptRecord{ProjectIdentity: project, AttemptID: attemptID, ExecutionID: executionID, StepID: "legacy-workflow", AttemptNumber: attemptNumber, Status: attemptStatus, InputRevision: run.BaseCommitValue(), ExternalBinding: target, Legacy: true, CreatedAt: created, UpdatedAt: n}
	if run.ResultPath != nil {
		attempt.ResultID = *run.ResultPath
	}
	if err := tx.PutStepAttempt(attempt); err != nil {
		return err
	}
	if run.Decision != nil {
		d := run.Decision
		version := d.Version
		if version == 0 {
			version = 1
		}
		d.Version = version
		decision := DecisionRecord{ProjectIdentity: project, DecisionID: d.ID, Scope: "execution", WorkflowRunID: run.RunID, ExecutionID: executionID, AttemptID: attemptID, QuestionID: d.ID, Status: legacyDecisionStatus(d.Status), Version: version, UpdatedAt: n, CreatedAt: created}
		payload, _ := json.Marshal(d.Request)
		decision.Request = payload
		if d.AnswerPath != "" {
			if answer, e := os.ReadFile(d.AnswerPath); e == nil {
				decision.Answer = answer
			}
		}
		decision.AnsweredBy = d.AnsweredBy
		if err := tx.PutDecision(decision); err != nil {
			return err
		}
	}
	return nil
}

func legacyTaskID(project string, run *Run) string {
	return "legacy:" + shortHash(project+"\x00beads\x00"+func() string {
		if run.BeadsContext != nil {
			return run.BeadsContext.ProjectID
		}
		return ""
	}()+"\x00"+run.TaskID)
}
func legacyExecutionID(runID string) string { return "legacy-execution:" + runID }
func legacyProtocol(host string) string {
	if host == "orca" {
		return "orca_terminal_v2"
	}
	return "local_process"
}
func (r *Run) WorktreePathValue() string {
	if r.WorktreePath != nil {
		return *r.WorktreePath
	}
	if r.WorkspaceIntent != nil {
		return r.WorkspaceIntent.Path
	}
	return ""
}
func (r *Run) WorktreeBranchValue() string {
	if r.WorktreeBranch != nil {
		return *r.WorktreeBranch
	}
	if r.WorkspaceIntent != nil {
		return r.WorkspaceIntent.Branch
	}
	return ""
}
func (r *Run) BaseCommitValue() string {
	if r.BaseCommit != nil {
		return *r.BaseCommit
	}
	if r.WorkspaceIntent != nil {
		return r.WorkspaceIntent.BaseCommit
	}
	return ""
}
func legacyDecisionStatus(v string) string {
	switch v {
	case "pending":
		return "pending"
	case "answer_saved":
		return "answer_saved"
	case "resuming":
		return "reply_accepted"
	case "applied":
		return "applied"
	case "superseded":
		return "superseded"
	default:
		return "failed"
	}
}

func runtimeOperations(runtime Runtime) []OperationRecord {
	project := runtime.Repository.Identity
	out := append([]OperationRecord(nil), runtime.Operations...)
	indices := map[string]int{}
	for i, op := range out {
		indices[op.OperationID] = i
	}
	if op := runtime.Control.Operation; op != nil {
		status := "outcome_unknown"
		if op.Phase == "applying" {
			status = "in_flight"
		} else if op.Phase == "completed" && op.Result != nil {
			if op.Result.Status == "applied" || op.Result.Status == "accepted" {
				status = "confirmed"
			} else {
				status = "rejected"
			}
		}
		digest := op.RequestDigest
		if len(digest) != 64 {
			digest = operationDigest(op.Payload)
		}
		var receipt []byte
		if op.Result != nil {
			receipt, _ = json.Marshal(op.Result)
		}
		control := OperationRecord{ProjectIdentity: project, OperationID: op.RequestID, IdempotencyKey: op.RequestID, Kind: op.Action, Status: status, PayloadDigest: digest, Request: append([]byte(nil), op.Payload...), Receipt: receipt, CreatedAt: now(), UpdatedAt: now()}
		if index, exists := indices[op.RequestID]; exists {
			control.CreatedAt = out[index].CreatedAt
			out[index] = control
		} else {
			out = append(out, control)
		}
	}
	return out
}

func operationDigest(payload []byte) string {
	h := sha256.Sum256(payload)
	return hex.EncodeToString(h[:])
}
func isConstraintError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "constraint")
}
