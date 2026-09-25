package steerline

import (
	"bytes"
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

const domainSchemaVersion = 4

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
	ProjectIdentity string              `json:"project_identity"`
	Tasks           []TaskRecord        `json:"tasks"`
	WorkflowRuns    []WorkflowRunRecord `json:"workflow_runs"`
	Executions      []ExecutionRecord   `json:"executions"`
	Attempts        []StepAttemptRecord `json:"attempts"`
	Decisions       []DecisionRecord    `json:"decisions"`
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
	if _, err := os.Stat(path); err == nil {
		if err = validateExistingDatabase(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
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
	if version != 0 && version != 3 && version != domainSchemaVersion {
		return nativeErr("unsupported_schema", fmt.Sprintf("unsupported SQLite schema %d; preserve existing state", version))
	}
	if version == 0 {
		for _, stmt := range append(append(append([]string{}, domainExecutionSchema...), nativeTaskSchema...), conversationSchemaV4...) {
			if _, err = tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("create domain schema: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, domainSchemaVersion, now().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		version = domainSchemaVersion
	}
	if version == 3 {
		for _, stmt := range conversationSchemaV4 {
			if _, err = tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("create conversation schema: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, domainSchemaVersion, now().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var nativeTaskSchema = []string{
	`CREATE TABLE project_task_authorities (project_identity TEXT PRIMARY KEY, authority TEXT NOT NULL CHECK(authority='native'), authority_version INTEGER NOT NULL CHECK(authority_version>0), graph_revision INTEGER NOT NULL CHECK(graph_revision>=0), repository TEXT NOT NULL, remote TEXT NOT NULL, branch TEXT NOT NULL, dispatch_mode TEXT NOT NULL CHECK(dispatch_mode IN ('manual','auto')), dispatch_paused INTEGER NOT NULL CHECK(dispatch_paused IN (0,1)), created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
	`CREATE TABLE database_state (singleton INTEGER PRIMARY KEY CHECK(singleton=1), database_id TEXT NOT NULL CHECK(length(database_id)=32), revision INTEGER NOT NULL CHECK(revision>=0))`,
	`INSERT INTO database_state(singleton,database_id,revision) VALUES(1,lower(hex(randomblob(16))),0)`,
	`CREATE INDEX tasks_decimal_order ON tasks(project_identity,length(task_id),task_id)`,
	`CREATE TABLE task_contracts (project_identity TEXT NOT NULL, contract_id TEXT NOT NULL, task_id TEXT NOT NULL, digest TEXT NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(project_identity,contract_id), UNIQUE(project_identity,task_id), FOREIGN KEY(project_identity,task_id) REFERENCES tasks(project_identity,task_id))`,
	`CREATE TABLE task_batches (project_identity TEXT NOT NULL, batch_id TEXT NOT NULL, schema_version TEXT NOT NULL, payload_digest TEXT NOT NULL, created_at TEXT NOT NULL, command_id TEXT NOT NULL, task_ids BLOB NOT NULL, PRIMARY KEY(project_identity,batch_id), UNIQUE(project_identity,command_id))`,
	`CREATE TABLE task_dependencies (project_identity TEXT NOT NULL, predecessor_id TEXT NOT NULL, successor_id TEXT NOT NULL, PRIMARY KEY(project_identity,predecessor_id,successor_id), CHECK(predecessor_id<>successor_id), FOREIGN KEY(project_identity,predecessor_id) REFERENCES tasks(project_identity,task_id), FOREIGN KEY(project_identity,successor_id) REFERENCES tasks(project_identity,task_id))`,
	`CREATE INDEX task_dependencies_successor_idx ON task_dependencies(project_identity,successor_id,predecessor_id)`,
	`CREATE TRIGGER task_dependencies_no_cycle BEFORE INSERT ON task_dependencies BEGIN
		SELECT RAISE(ABORT,'dependency_cycle') WHERE NEW.predecessor_id=NEW.successor_id OR EXISTS (
			WITH RECURSIVE reachable(task_id) AS (
				SELECT successor_id FROM task_dependencies WHERE project_identity=NEW.project_identity AND predecessor_id=NEW.successor_id
				UNION SELECT d.successor_id FROM task_dependencies d JOIN reachable r ON d.predecessor_id=r.task_id WHERE d.project_identity=NEW.project_identity
			) SELECT 1 FROM reachable WHERE task_id=NEW.predecessor_id);
	END`,
	`CREATE TRIGGER task_dependencies_no_update BEFORE UPDATE ON task_dependencies BEGIN SELECT RAISE(ABORT,'immutable_dependency'); END`,
	`CREATE TRIGGER task_dependencies_no_delete BEFORE DELETE ON task_dependencies BEGIN SELECT RAISE(ABORT,'immutable_dependency'); END`,
	`CREATE UNIQUE INDEX workflow_run_task_identity ON workflow_runs(project_identity,workflow_run_id,task_id)`,
	`CREATE TABLE task_deliveries (project_identity TEXT NOT NULL, task_id TEXT NOT NULL, selected_workflow_run_id TEXT NOT NULL, pr_instance TEXT NOT NULL, pr_repository TEXT NOT NULL, pr_number INTEGER NOT NULL CHECK(pr_number>0), pr_url TEXT NOT NULL, merge_commit TEXT NOT NULL, merged_at TEXT NOT NULL, recorded_at TEXT NOT NULL, evidence BLOB NOT NULL, PRIMARY KEY(project_identity,task_id), UNIQUE(pr_instance,pr_repository,pr_number), FOREIGN KEY(project_identity,task_id) REFERENCES tasks(project_identity,task_id), FOREIGN KEY(project_identity,selected_workflow_run_id,task_id) REFERENCES workflow_runs(project_identity,workflow_run_id,task_id))`,
}

var domainExecutionSchema = []string{
	`CREATE TABLE execution_bindings (run_id TEXT NOT NULL, attempt_id TEXT NOT NULL, record_id TEXT NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(run_id,attempt_id,record_id))`,
	`CREATE TABLE execution_operations (run_id TEXT NOT NULL, attempt_id TEXT NOT NULL, record_id TEXT NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(run_id,attempt_id,record_id))`,
	`CREATE TABLE tasks (project_identity TEXT NOT NULL, task_id TEXT NOT NULL, external_provider TEXT NOT NULL DEFAULT '', external_project TEXT NOT NULL DEFAULT '', external_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, version INTEGER NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(project_identity, task_id))`,
	`CREATE UNIQUE INDEX task_external_identity ON tasks(project_identity,external_provider,external_project,external_id) WHERE external_id <> ''`,
	`CREATE TABLE workflow_runs (project_identity TEXT NOT NULL, workflow_run_id TEXT NOT NULL, task_id TEXT NOT NULL, phase TEXT NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(project_identity, workflow_run_id), FOREIGN KEY(project_identity, task_id) REFERENCES tasks(project_identity, task_id))`,
	`CREATE INDEX workflow_runs_task_idx ON workflow_runs(project_identity, task_id)`,
	`CREATE UNIQUE INDEX one_active_workflow_per_task ON workflow_runs(project_identity,task_id) WHERE phase <> 'completed'`,
	`CREATE TABLE executions (project_identity TEXT NOT NULL, execution_id TEXT NOT NULL, workflow_run_id TEXT NOT NULL, status TEXT NOT NULL, current_step TEXT NOT NULL DEFAULT '', target_json BLOB NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(project_identity, execution_id), FOREIGN KEY(project_identity, workflow_run_id) REFERENCES workflow_runs(project_identity, workflow_run_id))`,
	`CREATE INDEX executions_workflow_idx ON executions(project_identity, workflow_run_id)`,
	`CREATE UNIQUE INDEX one_active_execution_per_workflow ON executions(project_identity,workflow_run_id) WHERE status IN ('pending','active','waiting_decision','blocked')`,
	`CREATE TABLE step_attempts (project_identity TEXT NOT NULL, attempt_id TEXT NOT NULL, execution_id TEXT NOT NULL, step_id TEXT NOT NULL, attempt_number INTEGER NOT NULL, operation_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(project_identity, attempt_id), UNIQUE(project_identity, execution_id, step_id, attempt_number), FOREIGN KEY(project_identity, execution_id) REFERENCES executions(project_identity, execution_id))`,
	`CREATE UNIQUE INDEX one_active_attempt_per_step ON step_attempts(project_identity, execution_id, step_id) WHERE status IN ('planned','dispatching','dispatched','running','waiting_decision','stopping','outcome_unknown')`,
	`CREATE UNIQUE INDEX one_active_attempt_per_execution ON step_attempts(project_identity,execution_id) WHERE status IN ('planned','dispatching','dispatched','running','waiting_decision','stopping','outcome_unknown')`,
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

var conversationSchemaV4 = []string{
	`CREATE TABLE conversation_events (event_id INTEGER PRIMARY KEY AUTOINCREMENT, project_identity TEXT NOT NULL, task_id TEXT NOT NULL DEFAULT '', workflow_run_id TEXT NOT NULL DEFAULT '', execution_id TEXT NOT NULL, attempt_id TEXT NOT NULL, source_event_key TEXT NOT NULL, source TEXT NOT NULL, source_segment TEXT NOT NULL, session_id TEXT NOT NULL DEFAULT '', turn_id TEXT NOT NULL DEFAULT '', item_key TEXT NOT NULL DEFAULT '', event_type TEXT NOT NULL, phase TEXT NOT NULL DEFAULT 'unknown', final_availability TEXT NOT NULL DEFAULT '', received_at TEXT NOT NULL, payload_bytes INTEGER NOT NULL, payload BLOB NOT NULL, UNIQUE(project_identity,source_event_key))`,
	`CREATE INDEX conversation_execution_idx ON conversation_events(project_identity,execution_id,attempt_id,event_id)`,
	`CREATE INDEX conversation_turn_idx ON conversation_events(project_identity,execution_id,attempt_id,session_id,turn_id,event_id)`,
	`CREATE TABLE conversation_bodies (project_identity TEXT NOT NULL, execution_id TEXT NOT NULL, attempt_id TEXT NOT NULL, item_key TEXT NOT NULL, revision INTEGER NOT NULL, body BLOB NOT NULL, sha256 TEXT NOT NULL, byte_count INTEGER NOT NULL, source TEXT NOT NULL, completeness TEXT NOT NULL, created_at TEXT NOT NULL, PRIMARY KEY(project_identity,execution_id,attempt_id,item_key,revision))`,
	`CREATE TABLE conversation_current_messages (project_identity TEXT NOT NULL, execution_id TEXT NOT NULL, attempt_id TEXT NOT NULL, item_key TEXT NOT NULL, revision INTEGER NOT NULL, source_seq INTEGER NOT NULL, body BLOB NOT NULL, completeness TEXT NOT NULL, PRIMARY KEY(project_identity,execution_id,attempt_id,item_key))`,
	`CREATE TABLE conversation_raw (project_identity TEXT NOT NULL, execution_id TEXT NOT NULL, attempt_id TEXT NOT NULL, raw_id TEXT NOT NULL, source_event_key TEXT NOT NULL, payload BLOB NOT NULL, sha256 TEXT NOT NULL, byte_count INTEGER NOT NULL, created_at TEXT NOT NULL, PRIMARY KEY(project_identity,execution_id,attempt_id,raw_id), UNIQUE(project_identity,source_event_key))`,
	`CREATE TABLE conversation_source_cursors (project_identity TEXT NOT NULL, execution_id TEXT NOT NULL, attempt_id TEXT NOT NULL, source TEXT NOT NULL, source_segment TEXT NOT NULL, source_identity TEXT NOT NULL DEFAULT '', generation TEXT NOT NULL DEFAULT '', cursor TEXT NOT NULL DEFAULT '', local_seq INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL, PRIMARY KEY(project_identity,execution_id,attempt_id,source,source_segment))`,
	`CREATE TABLE conversation_capability_reports (report_id INTEGER PRIMARY KEY AUTOINCREMENT, backend TEXT NOT NULL, protocol TEXT NOT NULL, agent TEXT NOT NULL, agent_version TEXT NOT NULL DEFAULT '', platform_version TEXT NOT NULL DEFAULT '', target_id TEXT NOT NULL DEFAULT '', checked_at TEXT NOT NULL, payload BLOB NOT NULL)`,
	`CREATE INDEX conversation_capability_lookup_idx ON conversation_capability_reports(backend,protocol,agent,target_id,checked_at)`,
	`CREATE INDEX conversation_native_source_idx ON conversation_events(project_identity,execution_id,attempt_id,source,event_type)`,
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
	if err = validateNativeTaskStructure(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE database_state SET revision=revision+1 WHERE singleton=1`); err != nil {
		return err
	}
	return tx.Commit()
}

func validateNativeTaskStructure(ctx context.Context, tx *sql.Tx) error {
	var invalidTasks, invalidBatches, invalidDeliveries int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks t JOIN project_task_authorities p ON p.project_identity=t.project_identity AND p.authority='native'
		WHERE COALESCE(json_extract(t.payload,'$.source'),'')<>'native'
		OR COALESCE(json_extract(t.payload,'$.task_id'),'')<>t.task_id
		OR COALESCE(json_extract(t.payload,'$.status'),'')<>t.status
		OR t.status NOT IN ('open','in_progress','discarded','closed') OR t.version<1
		OR COALESCE(json_extract(t.payload,'$.title'),'')='' OR COALESCE(json_extract(t.payload,'$.type'),'')=''
		OR COALESCE(json_extract(t.payload,'$.priority'),0)<0 OR COALESCE(json_extract(t.payload,'$.priority'),0)>4
		OR COALESCE(json_extract(t.payload,'$.created_at'),'')=''
		OR COALESCE(json_extract(t.payload,'$.batch_id'),'')='' OR COALESCE(json_extract(t.payload,'$.contract_id'),'')=''
		OR NOT EXISTS (SELECT 1 FROM task_contracts c WHERE c.project_identity=t.project_identity AND c.task_id=t.task_id AND c.contract_id=json_extract(t.payload,'$.contract_id'))
		OR NOT EXISTS (SELECT 1 FROM task_batches b JOIN json_each(b.task_ids) m WHERE b.project_identity=t.project_identity AND b.batch_id=json_extract(t.payload,'$.batch_id') AND m.value=t.task_id)
		OR (t.status='open' AND COALESCE(json_extract(t.payload,'$.claimant'),'')<>'')
		OR (t.status IN ('in_progress','closed') AND COALESCE(json_extract(t.payload,'$.claimant'),'')='')
		OR (t.status='discarded' AND (json_extract(t.payload,'$.discarded_at') IS NULL OR trim(COALESCE(json_extract(t.payload,'$.discard_reason'),''))='' OR trim(COALESCE(json_extract(t.payload,'$.discarded_by'),''))='' OR trim(COALESCE(json_extract(t.payload,'$.discard_request_id'),''))=''))
		OR (t.status<>'discarded' AND (json_extract(t.payload,'$.discarded_at') IS NOT NULL OR COALESCE(json_extract(t.payload,'$.discard_reason'),'')<>'' OR COALESCE(json_extract(t.payload,'$.discarded_by'),'')<>'' OR COALESCE(json_extract(t.payload,'$.discard_request_id'),'')<>'' OR COALESCE(json_array_length(json_extract(t.payload,'$.cleanup_pending')),0)<>0))
		OR (t.status='closed' AND NOT EXISTS (SELECT 1 FROM task_deliveries d WHERE d.project_identity=t.project_identity AND d.task_id=t.task_id))
		OR (t.status='discarded' AND EXISTS (SELECT 1 FROM task_deliveries d WHERE d.project_identity=t.project_identity AND d.task_id=t.task_id))`).Scan(&invalidTasks)
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_batches b JOIN project_task_authorities p ON p.project_identity=b.project_identity AND p.authority='native' JOIN json_each(b.task_ids) m WHERE NOT EXISTS (SELECT 1 FROM tasks t WHERE t.project_identity=b.project_identity AND t.task_id=m.value AND json_extract(t.payload,'$.batch_id')=b.batch_id)`).Scan(&invalidBatches)
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_deliveries d JOIN project_task_authorities p ON p.project_identity=d.project_identity AND p.authority='native' JOIN tasks t ON t.project_identity=d.project_identity AND t.task_id=d.task_id WHERE t.status<>'closed' OR NOT EXISTS (SELECT 1 FROM workflow_runs w JOIN executions e ON e.project_identity=w.project_identity AND e.workflow_run_id=w.workflow_run_id WHERE w.project_identity=d.project_identity AND w.workflow_run_id=d.selected_workflow_run_id AND w.task_id=d.task_id AND w.phase='completed' AND e.execution_id=json_extract(w.payload,'$.selected_result_id') AND e.status='succeeded' AND e.current_step='completed')`).Scan(&invalidDeliveries)
	if err != nil {
		return err
	}
	if invalidTasks != 0 || invalidBatches != 0 || invalidDeliveries != 0 {
		return nativeErr("contract_mismatch", "native task, batch, contract and delivery records must commit as one consistent transaction")
	}
	return nil
}

type sqliteTx struct {
	ctx context.Context
	tx  *sql.Tx
}

func (t *sqliteTx) PutProjectTaskAuthority(v ProjectTaskAuthority) error {
	if v.ProjectIdentity == "" || v.Authority != "native" || v.AuthorityVersion == 0 || v.DispatchMode != "manual" || !v.DispatchPaused || v.Repository == "" || v.Remote == "" || v.Branch == "" {
		return errors.New("native project authority requires identity, target, version and initial manual/paused dispatch")
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now()
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = v.CreatedAt
	}
	result, err := t.tx.ExecContext(t.ctx, `INSERT INTO project_task_authorities(project_identity,authority,authority_version,graph_revision,repository,remote,branch,dispatch_mode,dispatch_paused,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(project_identity) DO UPDATE SET updated_at=excluded.updated_at WHERE project_task_authorities.authority=excluded.authority AND project_task_authorities.authority_version=excluded.authority_version AND project_task_authorities.graph_revision=excluded.graph_revision AND project_task_authorities.repository=excluded.repository AND project_task_authorities.remote=excluded.remote AND project_task_authorities.branch=excluded.branch AND project_task_authorities.dispatch_mode=excluded.dispatch_mode AND project_task_authorities.dispatch_paused=excluded.dispatch_paused`, v.ProjectIdentity, v.Authority, v.AuthorityVersion, v.GraphRevision, v.Repository, v.Remote, v.Branch, v.DispatchMode, boolInt(v.DispatchPaused), v.CreatedAt.UTC().Format(time.RFC3339Nano), v.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return requireIdentityWrite(result, err, "authority_mismatch: native project authority is immutable without a compare-and-swap")
}

func (t *sqliteTx) UpdateProjectTaskAuthorityCAS(v ProjectTaskAuthority, expectedVersion uint64) error {
	if v.ProjectIdentity == "" || expectedVersion == 0 || v.AuthorityVersion != expectedVersion+1 || v.Authority != "native" || v.DispatchMode != "manual" && v.DispatchMode != "auto" {
		return nativeErr("authority_mismatch", "project authority update requires a valid next version and dispatch mode")
	}
	var old ProjectTaskAuthority
	var paused int
	var created, updated string
	err := t.tx.QueryRowContext(t.ctx, `SELECT authority,authority_version,graph_revision,repository,remote,branch,dispatch_mode,dispatch_paused,created_at,updated_at FROM project_task_authorities WHERE project_identity=?`, v.ProjectIdentity).Scan(&old.Authority, &old.AuthorityVersion, &old.GraphRevision, &old.Repository, &old.Remote, &old.Branch, &old.DispatchMode, &paused, &created, &updated)
	if err != nil {
		return nativeErr("authority_mismatch", "native project authority was not found")
	}
	old.ProjectIdentity = v.ProjectIdentity
	old.DispatchPaused = paused != 0
	old.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	old.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if old.AuthorityVersion != expectedVersion {
		return nativeErr("stale_version", fmt.Sprintf("expected authority version %d, found %d", expectedVersion, old.AuthorityVersion))
	}
	if v.GraphRevision != old.GraphRevision || v.Repository != old.Repository || v.Remote != old.Remote || v.Branch != old.Branch || !v.CreatedAt.Equal(old.CreatedAt) {
		return nativeErr("authority_mismatch", "project identity, target or graph revision cannot change through authority CAS")
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = now()
	}
	result, err := t.tx.ExecContext(t.ctx, `UPDATE project_task_authorities SET authority_version=?,dispatch_mode=?,dispatch_paused=?,updated_at=? WHERE project_identity=? AND authority_version=? AND graph_revision=?`, v.AuthorityVersion, v.DispatchMode, boolInt(v.DispatchPaused), v.UpdatedAt.UTC().Format(time.RFC3339Nano), v.ProjectIdentity, expectedVersion, v.GraphRevision)
	return requireIdentityWrite(result, err, "stale_version: project authority changed during compare-and-swap")
}

func (t *sqliteTx) AdvanceTaskGraphRevision(project string, expectedAuthorityVersion, expectedGraphRevision uint64) (uint64, error) {
	if project == "" || expectedAuthorityVersion == 0 {
		return 0, nativeErr("authority_mismatch", "graph revision update requires a project and authority version")
	}
	result, err := t.tx.ExecContext(t.ctx, `UPDATE project_task_authorities SET graph_revision=graph_revision+1,updated_at=? WHERE project_identity=? AND authority='native' AND authority_version=? AND graph_revision=?`, now().Format(time.RFC3339Nano), project, expectedAuthorityVersion, expectedGraphRevision)
	if err != nil {
		return 0, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if changed != 1 {
		return 0, nativeErr("stale_version", "project authority or graph revision changed")
	}
	return expectedGraphRevision + 1, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (t *sqliteTx) PutTaskContract(project, taskID string, v TaskContract) error {
	if project == "" || taskID == "" || v.ContractID == "" || !validSHA256(v.Digest) {
		return errors.New("contract_mismatch: contract identity and digest are required")
	}
	if err := ValidateTaskContract(v); err != nil {
		return err
	}
	var repository, remote, branch string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT repository,remote,branch FROM project_task_authorities WHERE project_identity=? AND authority='native'`, project).Scan(&repository, &remote, &branch); err != nil {
		return nativeErr("authority_mismatch", "task contract project has no native authority")
	}
	if !sameRepositoryTarget(v.Repository, repository) || v.Remote != remote || v.Branch != branch {
		return nativeErr("contract_mismatch", "task contract target differs from persisted project authority")
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	res, err := t.tx.ExecContext(t.ctx, `INSERT INTO task_contracts(project_identity,contract_id,task_id,digest,payload) VALUES(?,?,?,?,?) ON CONFLICT(project_identity,contract_id) DO UPDATE SET digest=excluded.digest,payload=excluded.payload WHERE task_contracts.task_id=excluded.task_id AND task_contracts.digest=excluded.digest AND task_contracts.payload=excluded.payload`, project, v.ContractID, taskID, v.Digest, b)
	return requireIdentityWrite(res, err, "contract_mismatch: task contract is immutable or bound to another task")
}

func (t *sqliteTx) PutTaskBatch(v TaskBatchRecord, expectedAuthorityVersion, expectedGraphRevision uint64) error {
	if v.ProjectIdentity == "" || v.BatchID == "" || v.SchemaVersion != "task-batch/v1" || v.CommandID == "" || len(v.TaskIDs) == 0 || !validSHA256(v.PayloadDigest) {
		return errors.New("invalid task batch record")
	}
	if expectedAuthorityVersion == 0 {
		return nativeErr("authority_mismatch", "task batch requires an expected project authority version")
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now()
	}
	var authority string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT authority FROM project_task_authorities WHERE project_identity=?`, v.ProjectIdentity).Scan(&authority); err != nil || authority != "native" {
		return nativeErr("authority_mismatch", "task batch requires a registered native project")
	}
	seen := map[string]bool{}
	for localID, taskID := range v.TaskIDs {
		if strings.TrimSpace(localID) == "" || !canonicalDecimalID(taskID) || seen[taskID] {
			return nativeErr("contract_mismatch", "task batch mapping must contain unique canonical task IDs and nonempty local keys")
		}
		seen[taskID] = true
	}
	b, err := encodePayload(v.TaskIDs)
	if err != nil {
		return err
	}
	var oldSchema, oldDigest, oldCommand string
	var oldMapping []byte
	err = t.tx.QueryRowContext(t.ctx, `SELECT schema_version,payload_digest,command_id,task_ids FROM task_batches WHERE project_identity=? AND batch_id=?`, v.ProjectIdentity, v.BatchID).Scan(&oldSchema, &oldDigest, &oldCommand, &oldMapping)
	if err == nil {
		if oldSchema != v.SchemaVersion || oldDigest != v.PayloadDigest || oldCommand != v.CommandID || !bytes.Equal(oldMapping, b) {
			return nativeErr("contract_mismatch", "task batch identity is already bound to different content")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err = t.tx.ExecContext(t.ctx, `INSERT INTO task_batches(project_identity,batch_id,schema_version,payload_digest,created_at,command_id,task_ids) VALUES(?,?,?,?,?,?,?)`, v.ProjectIdentity, v.BatchID, v.SchemaVersion, v.PayloadDigest, v.CreatedAt.UTC().Format(time.RFC3339Nano), v.CommandID, b); err != nil {
		return err
	}
	_, err = t.AdvanceTaskGraphRevision(v.ProjectIdentity, expectedAuthorityVersion, expectedGraphRevision)
	return err
}

func (t *sqliteTx) PutTaskDependency(v TaskDependencyRecord) error {
	if v.ProjectIdentity == "" || v.PredecessorID == "" || v.SuccessorID == "" || v.PredecessorID == v.SuccessorID {
		return errors.New("invalid task dependency")
	}
	var authority string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT authority FROM project_task_authorities WHERE project_identity=?`, v.ProjectIdentity).Scan(&authority); err != nil || authority != "native" {
		return nativeErr("authority_mismatch", "task dependency requires a registered native project")
	}
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO task_dependencies(project_identity,predecessor_id,successor_id) VALUES(?,?,?)`, v.ProjectIdentity, v.PredecessorID, v.SuccessorID)
	return err
}

func (t *sqliteTx) PutTaskDelivery(v TaskDeliveryRecord) error {
	if v.RecordedAt.IsZero() {
		v.RecordedAt = now()
	}
	var targetBranch string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT branch FROM project_task_authorities WHERE project_identity=? AND authority='native'`, v.ProjectIdentity).Scan(&targetBranch); err != nil {
		return nativeErr("authority_mismatch", "task delivery project has no native authority")
	}
	if err := validateNativeTaskDelivery(v, targetBranch); err != nil {
		return err
	}
	var oldWorkflow, oldInstance, oldRepository, oldURL, oldMerge, oldMerged string
	var oldNumber int64
	var oldEvidence []byte
	err := t.tx.QueryRowContext(t.ctx, `SELECT selected_workflow_run_id,pr_instance,pr_repository,pr_number,pr_url,merge_commit,merged_at,evidence FROM task_deliveries WHERE project_identity=? AND task_id=?`, v.ProjectIdentity, v.TaskID).Scan(&oldWorkflow, &oldInstance, &oldRepository, &oldNumber, &oldURL, &oldMerge, &oldMerged, &oldEvidence)
	if err == nil {
		if oldWorkflow == v.SelectedWorkflowRunID && oldInstance == v.PRInstance && oldRepository == v.PRRepository && oldNumber == v.PRNumber && oldURL == v.PRURL && oldMerge == v.MergeCommit && oldMerged == v.MergedAt.UTC().Format(time.RFC3339Nano) && bytes.Equal(oldEvidence, v.Evidence) {
			return nil
		}
		return nativeErr("contract_mismatch", "task already has a different authoritative delivery")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var taskStatus string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT status FROM tasks WHERE project_identity=? AND task_id=?`, v.ProjectIdentity, v.TaskID).Scan(&taskStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nativeErr("task_not_found", "delivery requires an existing native task")
		}
		return err
	}
	if taskStatus == "discarded" {
		return nativeErr("task_discarded", "discarded task cannot receive a normal delivery")
	}
	if taskStatus != "in_progress" {
		return nativeErr("stale_version", "delivery requires the claimed task to be in progress")
	}
	var runPayload []byte
	if err := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM workflow_runs WHERE project_identity=? AND workflow_run_id=? AND task_id=?`, v.ProjectIdentity, v.SelectedWorkflowRunID, v.TaskID).Scan(&runPayload); err != nil {
		return nativeErr("contract_mismatch", "delivery must reference a workflow run belonging to the task")
	}
	var run WorkflowRunRecord
	if err := json.Unmarshal(runPayload, &run); err != nil {
		return err
	}
	if run.Phase != "waiting_pr_merge" || run.SelectedResult == "" {
		return nativeErr("stale_version", "delivery requires the selected workflow run awaiting PR merge")
	}
	var selected int
	if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM executions WHERE project_identity=? AND workflow_run_id=? AND execution_id=? AND status='succeeded' AND current_step='waiting_pr_merge'`, v.ProjectIdentity, v.SelectedWorkflowRunID, run.SelectedResult).Scan(&selected); err != nil {
		return err
	}
	if selected != 1 {
		return nativeErr("contract_mismatch", "delivery has no selected successful execution")
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO task_deliveries(project_identity,task_id,selected_workflow_run_id,pr_instance,pr_repository,pr_number,pr_url,merge_commit,merged_at,recorded_at,evidence) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, v.ProjectIdentity, v.TaskID, v.SelectedWorkflowRunID, v.PRInstance, v.PRRepository, v.PRNumber, v.PRURL, v.MergeCommit, v.MergedAt.UTC().Format(time.RFC3339Nano), v.RecordedAt.UTC().Format(time.RFC3339Nano), []byte(v.Evidence))
	return err
}

func validateNativeTaskDelivery(v TaskDeliveryRecord, targetBranch string) error {
	if v.ProjectIdentity == "" || v.TaskID == "" || v.SelectedWorkflowRunID == "" || v.PRInstance == "" || v.PRRepository == "" || v.PRNumber <= 0 || v.PRURL == "" || !validCommit(v.MergeCommit) || v.MergedAt.IsZero() || v.RecordedAt.IsZero() {
		return nativeErr("inconsistent_delivery", "native task delivery has incomplete fixed identity")
	}
	if len(v.Evidence) == 0 || !json.Valid(v.Evidence) {
		return nativeErr("inconsistent_delivery", "native task delivery has no readable PR merge evidence")
	}
	var evidence TaskDeliveryEvidence
	if err := json.Unmarshal(v.Evidence, &evidence); err != nil {
		return nativeErr("inconsistent_delivery", "native task delivery PR merge evidence has an invalid shape")
	}
	if !evidence.Merged || evidence.State != "merged" || evidence.PRInstance != v.PRInstance || evidence.URL != v.PRURL || evidence.Repository != v.PRRepository || evidence.PRNumber != v.PRNumber || evidence.MergeCommit != v.MergeCommit || !evidence.MergedAt.Equal(v.MergedAt) || evidence.BaseBranch != targetBranch {
		return nativeErr("inconsistent_delivery", "native task delivery PR evidence differs from its fixed identity")
	}
	return nil
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
	var nativeAuthority string
	authorityErr := t.tx.QueryRowContext(t.ctx, `SELECT authority FROM project_task_authorities WHERE project_identity=?`, v.ProjectIdentity).Scan(&nativeAuthority)
	if authorityErr != nil && !errors.Is(authorityErr, sql.ErrNoRows) {
		return authorityErr
	}
	if errors.Is(authorityErr, sql.ErrNoRows) && v.Source == "native" {
		return nativeErr("authority_mismatch", "native task cannot be stored without a registered native project")
	}
	if authorityErr == nil {
		if nativeAuthority != "native" || v.Source != "native" || !canonicalDecimalID(v.TaskID) || v.BatchID == "" || v.Title == "" || v.Type == "" || v.Priority < 0 || v.Priority > 4 || v.ContractID == "" || v.Version == 0 || v.CreatedAt.IsZero() || v.ExternalProvider != "" || v.ExternalProject != "" || v.ExternalID != "" {
			return nativeErr("contract_mismatch", "native Task requires its original numeric ID, batch, type, priority and fixed contract")
		}
		var repository string
		if err = t.tx.QueryRowContext(t.ctx, `SELECT repository FROM project_task_authorities WHERE project_identity=? AND authority='native'`, v.ProjectIdentity).Scan(&repository); err != nil {
			return nativeErr("authority_mismatch", "native task project binding is unavailable")
		}
		var reused int
		if err = t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM tasks old JOIN project_task_authorities p ON p.project_identity=old.project_identity WHERE p.authority='native' AND p.repository=? AND old.task_id=? AND old.project_identity<>?`, repository, v.TaskID, v.ProjectIdentity).Scan(&reused); err != nil {
			return err
		}
		if reused > 0 {
			return nativeErr("immutable_task", "task ID is already used in the same repository identity")
		}
		if v.Status == "discarded" {
			if v.DiscardedAt == nil || strings.TrimSpace(v.DiscardReason) == "" || strings.TrimSpace(v.DiscardedBy) == "" || !controlIDPattern.MatchString(v.DiscardRequestID) {
				return errors.New("discarded native task requires timestamp, reason, requester and request ID")
			}
		} else if v.DiscardedAt != nil || v.DiscardReason != "" || v.DiscardedBy != "" || v.DiscardRequestID != "" || len(v.CleanupPending) > 0 {
			return errors.New("discard audit fields are valid only for discarded native tasks")
		}
		if v.Status == "closed" {
			var deliveries int
			if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=?`, v.ProjectIdentity, v.TaskID).Scan(&deliveries); err != nil {
				return err
			}
			if deliveries != 1 {
				return errors.New("native task cannot close without its authoritative delivery evidence")
			}
		}
		var previous []byte
		previousErr := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM tasks WHERE project_identity=? AND task_id=?`, v.ProjectIdentity, v.TaskID).Scan(&previous)
		if previousErr == nil {
			if bytes.Equal(previous, b) {
				return nil
			}
			return nativeErr("stale_version", "native task identity already exists; updates require an expected-version transaction")
		}
		if !errors.Is(previousErr, sql.ErrNoRows) {
			return previousErr
		}
	}
	result, err := t.tx.ExecContext(t.ctx, `INSERT INTO tasks(project_identity,task_id,external_provider,external_project,external_id,status,version,payload) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(project_identity,task_id) DO UPDATE SET status=excluded.status,version=excluded.version,payload=excluded.payload WHERE tasks.external_provider=excluded.external_provider AND tasks.external_project=excluded.external_project AND tasks.external_id=excluded.external_id`, v.ProjectIdentity, v.TaskID, v.ExternalProvider, v.ExternalProject, v.ExternalID, v.Status, v.Version, b)
	return requireIdentityWrite(result, err, "task ID belongs to another external identity")
}

func (t *sqliteTx) UpdateNativeTaskCAS(v TaskRecord, expectedVersion uint64) error {
	if v.ProjectIdentity == "" || !canonicalDecimalID(v.TaskID) || expectedVersion == 0 {
		return nativeErr("contract_mismatch", "native task update requires project, canonical task ID and expected version")
	}
	var payload []byte
	var authority string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT authority FROM project_task_authorities WHERE project_identity=?`, v.ProjectIdentity).Scan(&authority); err != nil || authority != "native" {
		return nativeErr("authority_mismatch", "task update requires a registered native project")
	}
	if err := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM tasks WHERE project_identity=? AND task_id=?`, v.ProjectIdentity, v.TaskID).Scan(&payload); err != nil {
		return err
	}
	var old TaskRecord
	if err := json.Unmarshal(payload, &old); err != nil {
		return err
	}
	if old.Source != "native" {
		return nativeErr("authority_mismatch", "task is not owned by the native authority")
	}
	if old.Version != expectedVersion {
		return nativeErr("stale_version", fmt.Sprintf("expected version %d, found %d", expectedVersion, old.Version))
	}
	if v.ProjectIdentity != old.ProjectIdentity || v.TaskID != old.TaskID || v.BatchID != old.BatchID || v.ContractID != old.ContractID || v.Type != old.Type || v.Priority != old.Priority || v.Title != old.Title || !sameStrings(v.GoalIDs, old.GoalIDs) || v.IdentityMapDigest != old.IdentityMapDigest || v.Source != "native" || v.Version != expectedVersion+1 || !v.CreatedAt.Equal(old.CreatedAt) || v.ExternalProvider != "" || v.ExternalProject != "" || v.ExternalID != "" {
		return nativeErr("contract_mismatch", "native task update cannot change its identity, scope, contract, or fixed fields")
	}
	allowed := map[string]map[string]bool{"open": {"in_progress": true, "discarded": true}, "in_progress": {"discarded": true, "closed": true}}
	if !allowed[old.Status][v.Status] {
		return nativeErr("stale_version", "native task status transition is not permitted")
	}
	if v.Status == "in_progress" {
		if old.Status != "open" || old.Claimant != "" || v.Claimant == "" {
			return nativeErr("stale_version", "claim requires an open task and a new claimant")
		}
	} else if v.Claimant != old.Claimant {
		return nativeErr("contract_mismatch", "claimant is preserved after claim")
	}
	if v.Status == "discarded" {
		if v.DiscardedAt == nil || strings.TrimSpace(v.DiscardReason) == "" || strings.TrimSpace(v.DiscardedBy) == "" || !controlIDPattern.MatchString(v.DiscardRequestID) {
			return nativeErr("contract_mismatch", "discard requires timestamp, reason, requester and request ID")
		}
		var count int
		if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=?`, v.ProjectIdentity, v.TaskID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return nativeErr("stale_version", "task with a delivery cannot be discarded")
		}
	} else if v.DiscardedAt != nil || v.DiscardReason != "" || v.DiscardedBy != "" || v.DiscardRequestID != "" || len(v.CleanupPending) > 0 {
		return nativeErr("contract_mismatch", "discard audit fields are valid only for discarded tasks")
	}
	if v.Status == "closed" {
		if old.Claimant == "" || v.Claimant != old.Claimant {
			return nativeErr("stale_version", "unclaimed task cannot be closed")
		}
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = now()
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	res, err := t.tx.ExecContext(t.ctx, `UPDATE tasks SET status=?,version=?,payload=? WHERE project_identity=? AND task_id=? AND version=?`, v.Status, v.Version, b, v.ProjectIdentity, v.TaskID, expectedVersion)
	if err != nil {
		return err
	}
	if err = requireIdentityWrite(res, err, "stale_version: native task changed during update"); err != nil {
		return err
	}
	if v.Status == "closed" {
		if err = t.completeNativeDeliveryWorkflow(v.ProjectIdentity, v.TaskID); err != nil {
			return err
		}
	}
	if v.Status == "discarded" || v.Status == "closed" {
		var authorityVersion, graphRevision uint64
		if err = t.tx.QueryRowContext(t.ctx, `SELECT authority_version,graph_revision FROM project_task_authorities WHERE project_identity=? AND authority='native'`, v.ProjectIdentity).Scan(&authorityVersion, &graphRevision); err != nil {
			return nativeErr("authority_mismatch", "native task graph authority is unavailable")
		}
		_, err = t.AdvanceTaskGraphRevision(v.ProjectIdentity, authorityVersion, graphRevision)
		return err
	}
	return nil
}

func (t *sqliteTx) completeNativeDeliveryWorkflow(project, taskID string) error {
	var runID string
	var runPayload []byte
	err := t.tx.QueryRowContext(t.ctx, `SELECT d.selected_workflow_run_id,w.payload FROM task_deliveries d JOIN workflow_runs w ON w.project_identity=d.project_identity AND w.workflow_run_id=d.selected_workflow_run_id AND w.task_id=d.task_id WHERE d.project_identity=? AND d.task_id=?`, project, taskID).Scan(&runID, &runPayload)
	if err != nil {
		return nativeErr("contract_mismatch", "task cannot close without its authoritative selected delivery")
	}
	var run WorkflowRunRecord
	if err = json.Unmarshal(runPayload, &run); err != nil {
		return err
	}
	if run.Phase != "waiting_pr_merge" || run.SelectedResult == "" {
		return nativeErr("stale_version", "selected workflow is no longer waiting for its PR delivery")
	}
	var executionPayload []byte
	if err = t.tx.QueryRowContext(t.ctx, `SELECT payload FROM executions WHERE project_identity=? AND workflow_run_id=? AND execution_id=? AND status='succeeded' AND current_step='waiting_pr_merge'`, project, runID, run.SelectedResult).Scan(&executionPayload); err != nil {
		return nativeErr("contract_mismatch", "selected workflow has no successful PR-ready execution")
	}
	var execution ExecutionRecord
	if err = json.Unmarshal(executionPayload, &execution); err != nil {
		return err
	}
	run.Phase, run.UpdatedAt = "completed", now()
	runPayload, err = encodePayload(run)
	if err != nil {
		return err
	}
	result, err := t.tx.ExecContext(t.ctx, `UPDATE workflow_runs SET phase='completed',payload=? WHERE project_identity=? AND workflow_run_id=? AND task_id=? AND phase='waiting_pr_merge'`, runPayload, project, runID, taskID)
	if err = requireIdentityWrite(result, err, "stale_version: selected workflow changed during delivery"); err != nil {
		return err
	}
	execution.CurrentStep, execution.UpdatedAt = "completed", now()
	executionPayload, err = encodePayload(execution)
	if err != nil {
		return err
	}
	result, err = t.tx.ExecContext(t.ctx, `UPDATE executions SET current_step='completed',payload=? WHERE project_identity=? AND execution_id=? AND workflow_run_id=? AND status='succeeded' AND current_step='waiting_pr_merge'`, executionPayload, project, execution.ExecutionID, runID)
	return requireIdentityWrite(result, err, "stale_version: selected execution changed during delivery")
}

func (t *sqliteTx) PutWorkflowRun(v WorkflowRunRecord) error {
	if v.ProjectIdentity == "" || v.WorkflowRunID == "" || v.TaskID == "" || v.Phase == "" {
		return errors.New("workflow run identity, task and phase are required")
	}
	if !validWorkflowPhase(v.Phase) {
		return fmt.Errorf("invalid workflow run phase %q", v.Phase)
	}
	var authority string
	authorityErr := t.tx.QueryRowContext(t.ctx, `SELECT authority FROM project_task_authorities WHERE project_identity=?`, v.ProjectIdentity).Scan(&authority)
	if authorityErr != nil && !errors.Is(authorityErr, sql.ErrNoRows) {
		return authorityErr
	}
	if authorityErr == nil && authority == "native" {
		var previous []byte
		err := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, v.ProjectIdentity, v.WorkflowRunID).Scan(&previous)
		if err == nil {
			var old WorkflowRunRecord
			if err = json.Unmarshal(previous, &old); err != nil {
				return err
			}
			oldPin, pinErr := nativeWorkflowPin(old)
			if pinErr != nil {
				return pinErr
			}
			newPin, pinErr := nativeWorkflowPin(v)
			if pinErr != nil {
				return pinErr
			}
			if old.ProjectIdentity != v.ProjectIdentity || old.WorkflowRunID != v.WorkflowRunID || old.TaskID != v.TaskID || !old.CreatedAt.Equal(v.CreatedAt) || !bytes.Equal(oldPin, newPin) || old.SelectedResult != "" && old.SelectedResult != v.SelectedResult {
				return nativeErr("immutable_task", "native workflow identity, definition, runner and selected result are fixed")
			}
			if old.Phase == "completed" && (v.Phase != old.Phase || v.PendingDecisions != old.PendingDecisions || !v.UpdatedAt.Equal(old.UpdatedAt)) {
				return nativeErr("immutable_task", "completed native workflow history cannot be changed")
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if v.Phase == "completed" {
			var deliveries int
			if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=? AND selected_workflow_run_id=?`, v.ProjectIdentity, v.TaskID, v.WorkflowRunID).Scan(&deliveries); err != nil {
				return err
			}
			if deliveries != 1 {
				return nativeErr("inconsistent_delivery", "native workflow cannot complete without its selected delivery")
			}
		}
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	result, err := t.tx.ExecContext(t.ctx, `INSERT INTO workflow_runs(project_identity,workflow_run_id,task_id,phase,payload) VALUES(?,?,?,?,?) ON CONFLICT(project_identity,workflow_run_id) DO UPDATE SET phase=excluded.phase,payload=excluded.payload WHERE workflow_runs.task_id=excluded.task_id`, v.ProjectIdentity, v.WorkflowRunID, v.TaskID, v.Phase, b)
	return requireIdentityWrite(result, err, "workflow identity cannot be rebound to another task")
}

func nativeWorkflowPin(v WorkflowRunRecord) ([]byte, error) {
	return encodePayload(struct {
		WorkflowVersion string
		ConfigDigest    string
		Definition      WorkflowDefinition
		ProjectSteps    ProjectSteps
		Runner          StepRunnerSettings
		Repository      string
		Branch          string
	}{v.WorkflowVersion, v.ConfigDigest, v.Definition, v.ProjectSteps, v.Runner, v.Repository, v.Branch})
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
	var previous []byte
	err = t.tx.QueryRowContext(t.ctx, `SELECT payload FROM executions WHERE project_identity=? AND execution_id=?`, v.ProjectIdentity, v.ExecutionID).Scan(&previous)
	if err == nil && !bytes.Equal(previous, b) {
		delivered, e := t.selectedNativeDeliveryExecution(v.ProjectIdentity, v.ExecutionID)
		if e != nil {
			return e
		}
		if delivered {
			return nativeErr("immutable_task", "selected delivered execution history cannot be changed")
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	target, err := encodePayload(v.Target)
	if err != nil {
		return err
	}
	result, err := t.tx.ExecContext(t.ctx, `INSERT INTO executions(project_identity,execution_id,workflow_run_id,status,current_step,target_json,payload) VALUES(?,?,?,?,?,?,?) ON CONFLICT(project_identity,execution_id) DO UPDATE SET status=excluded.status,current_step=excluded.current_step,target_json=excluded.target_json,payload=excluded.payload WHERE executions.workflow_run_id=excluded.workflow_run_id`, v.ProjectIdentity, v.ExecutionID, v.WorkflowRunID, v.Status, v.CurrentStep, target, b)
	return requireIdentityWrite(result, err, "execution identity cannot be rebound to another workflow")
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
	var previous []byte
	err = t.tx.QueryRowContext(t.ctx, `SELECT payload FROM step_attempts WHERE project_identity=? AND attempt_id=?`, v.ProjectIdentity, v.AttemptID).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && !bytes.Equal(previous, b) {
		delivered, e := t.selectedNativeDeliveryExecution(v.ProjectIdentity, v.ExecutionID)
		if e != nil {
			return e
		}
		if delivered {
			return nativeErr("immutable_task", "selected delivered attempt history cannot be changed")
		}
	}
	result, err := t.tx.ExecContext(t.ctx, `INSERT INTO step_attempts(project_identity,attempt_id,execution_id,step_id,attempt_number,operation_id,status,payload) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(project_identity,attempt_id) DO UPDATE SET operation_id=excluded.operation_id,status=excluded.status,payload=excluded.payload WHERE step_attempts.execution_id=excluded.execution_id AND step_attempts.step_id=excluded.step_id AND step_attempts.attempt_number=excluded.attempt_number`, v.ProjectIdentity, v.AttemptID, v.ExecutionID, v.StepID, v.AttemptNumber, v.OperationID, v.Status, b)
	return requireIdentityWrite(result, err, "attempt identity cannot be rebound to another execution or step")
}

func (t *sqliteTx) selectedNativeDeliveryExecution(project, executionID string) (bool, error) {
	var runID string
	err := t.tx.QueryRowContext(t.ctx, `SELECT workflow_run_id FROM executions WHERE project_identity=? AND execution_id=?`, project, executionID).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var taskID string
	var body []byte
	if err = t.tx.QueryRowContext(t.ctx, `SELECT task_id,payload FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, project, runID).Scan(&taskID, &body); err != nil {
		return false, nativeErr("inconsistent_delivery", "selected execution has no matching workflow")
	}
	var workflow WorkflowRunRecord
	if err = json.Unmarshal(body, &workflow); err != nil {
		return false, err
	}
	if workflow.ProjectIdentity != project || workflow.WorkflowRunID != runID || workflow.TaskID != taskID {
		return false, nativeErr("inconsistent_delivery", "selected workflow payload differs from its stored identity")
	}
	if workflow.SelectedResult != executionID {
		return false, nil
	}
	var deliveries int
	if err = t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=? AND selected_workflow_run_id=?`, project, taskID, runID).Scan(&deliveries); err != nil {
		return false, err
	}
	return deliveries == 1, nil
}

func requireIdentityWrite(result sql.Result, err error, message string) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New(message)
	}
	return nil
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
		if existing.Kind != v.Kind || !sameOperationRequest(existing.Request, v.Request) {
			return v, false, errors.New("operation request content cannot change")
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
		if old.Kind != v.Kind || old.TargetID != v.TargetID {
			return v, false, errors.New("idempotency key reused with different command content")
		}
		if old.Kind == "answer_decision" {
			var oldAnswer, newAnswer DecisionAnswer
			if decode(payload, &oldAnswer) != nil || decode(v.Payload, &newAnswer) != nil || !sameDecisionAnswer(oldAnswer, newAnswer) {
				return v, false, errors.New("idempotency key reused with a different decision answer")
			}
		} else if old.ExpectedVersion != v.ExpectedVersion || old.RequestedBy != v.RequestedBy || string(payload) != string(v.Payload) {
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
		var recordBytes []byte
		if err := t.tx.QueryRowContext(t.ctx, `SELECT version,status,payload FROM decisions WHERE project_identity=? AND decision_id=?`, v.ProjectIdentity, v.TargetID).Scan(&version, &status, &recordBytes); err != nil {
			return v, false, err
		}
		var record DecisionRecord
		var answer DecisionAnswer
		var request DecisionRequest
		if err := decode(recordBytes, &record); err != nil {
			return v, false, err
		}
		if err := decode(v.Payload, &answer); err != nil {
			return v, false, err
		}
		if err := decode(record.Request, &request); err != nil {
			return v, false, err
		}
		if answer.DecisionID != v.TargetID || answer.RunID != record.WorkflowRunID || answer.AnsweredBy != v.RequestedBy {
			return v, false, errors.New("answer target or requester mismatch")
		}
		if err := validateDecisionAnswer(&request, &answer); err != nil {
			return v, false, err
		}
		var acceptedCommandID string
		acceptedErr := t.tx.QueryRowContext(t.ctx, `SELECT command_id FROM decision_answers WHERE project_identity=? AND decision_id=? ORDER BY decision_version LIMIT 1`, v.ProjectIdentity, v.TargetID).Scan(&acceptedCommandID)
		if acceptedErr == nil {
			var first CommandRecord
			var firstPayload, firstResult []byte
			var firstCreated, firstUpdated string
			if err := t.tx.QueryRowContext(t.ctx, `SELECT command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at FROM commands WHERE project_identity=? AND command_id=?`, v.ProjectIdentity, acceptedCommandID).Scan(&first.CommandID, &first.Kind, &first.TargetID, &first.ExpectedVersion, &first.IdempotencyKey, &first.RequestedBy, &firstPayload, &first.Status, &firstResult, &firstCreated, &firstUpdated); err != nil {
				return v, false, err
			}
			var firstAnswer DecisionAnswer
			if err := decode(firstPayload, &firstAnswer); err != nil {
				return v, false, err
			}
			if !sameDecisionAnswer(firstAnswer, answer) {
				return v, false, ErrAnswerConflict
			}
			first.ProjectIdentity, first.Payload, first.Result = v.ProjectIdentity, firstPayload, firstResult
			first.CreatedAt, _ = time.Parse(time.RFC3339Nano, firstCreated)
			first.UpdatedAt, _ = time.Parse(time.RFC3339Nano, firstUpdated)
			return first, false, nil
		}
		if !errors.Is(acceptedErr, sql.ErrNoRows) {
			return v, false, acceptedErr
		}
		if version != v.ExpectedVersion && !(status == "answer_saved" && version == v.ExpectedVersion+1) {
			return v, false, fmt.Errorf("decision version conflict: expected %d, current %d, status %s", v.ExpectedVersion, version, status)
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

func (t *sqliteTx) AcceptDecisionAnswer(command CommandRecord, answer DecisionAnswer) (DecisionRecord, bool, error) {
	var record DecisionRecord
	var version uint64
	var status string
	var recordBytes []byte
	if err := t.tx.QueryRowContext(t.ctx, `SELECT version,status,payload FROM decisions WHERE project_identity=? AND decision_id=?`, command.ProjectIdentity, command.TargetID).Scan(&version, &status, &recordBytes); err != nil {
		return record, false, err
	}
	if err := decode(recordBytes, &record); err != nil {
		return record, false, err
	}
	if answer.DecisionID != command.TargetID || answer.RunID == "" || answer.AnsweredBy != command.RequestedBy || answer.RunID != record.WorkflowRunID || answer.ExpectedDecisionVersion != 0 && answer.ExpectedDecisionVersion != command.ExpectedVersion {
		return record, false, errors.New("answer target or requester mismatch")
	}
	var previousCommandID string
	var previousPayload []byte
	err := t.tx.QueryRowContext(t.ctx, `SELECT da.command_id,c.payload FROM decision_answers da JOIN commands c ON c.project_identity=da.project_identity AND c.command_id=da.command_id WHERE da.project_identity=? AND da.decision_id=? ORDER BY da.decision_version LIMIT 1`, command.ProjectIdentity, command.TargetID).Scan(&previousCommandID, &previousPayload)
	if err == nil {
		if status == "pending" {
			return record, false, errors.New("unsupported prior answer reservation exists while this decision remains pending; preserve the state without importing it")
		}
		var previous DecisionAnswer
		if err = decode(previousPayload, &previous); err != nil {
			return record, false, err
		}
		if !sameDecisionAnswer(previous, answer) {
			return record, false, ErrAnswerConflict
		}
		result := mustJSON(map[string]any{"decision_id": command.TargetID, "status": record.Status, "version": record.Version, "original_command_id": previousCommandID, "idempotent": true})
		command.Status, command.Result = "accepted", result
		if err = t.UpdateCommand(command); err != nil {
			return record, false, err
		}
		return record, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return record, false, err
	}
	if version != command.ExpectedVersion || status != "pending" {
		return record, false, fmt.Errorf("decision version conflict: expected %d, current %d, status %s", command.ExpectedVersion, version, status)
	}
	if answer.ExpectedRunRevision == 0 || answer.ServiceInstanceID == "" {
		return record, false, errors.New("decision answer lacks its displayed service instance and run revision")
	}
	var projectionBytes []byte
	if err = t.tx.QueryRowContext(t.ctx, `SELECT payload FROM projection WHERE project_identity=?`, command.ProjectIdentity).Scan(&projectionBytes); err != nil {
		return record, false, fmt.Errorf("load current service projection: %w", err)
	}
	var projection Runtime
	if err = decode(projectionBytes, &projection); err != nil {
		return record, false, fmt.Errorf("decode current service projection: %w", err)
	}
	if projection.Service.InstanceID != answer.ServiceInstanceID || projection.Service.Status != "running" || !projection.Service.Ready {
		return record, false, errors.New("stale_instance: answer targets a service instance that is no longer ready")
	}
	var target *Run
	for _, run := range projection.Runs {
		if run != nil && run.RunID == answer.RunID {
			target = run
			break
		}
	}
	if target == nil || target.ControlRevision != answer.ExpectedRunRevision || target.FinishTaskStatus != "waiting_decision" || target.Decision == nil || target.Decision.ID != command.TargetID || target.Decision.Status != "pending" || target.Decision.Version != command.ExpectedVersion || target.Decision.AttemptID != record.AttemptID || target.ExecutionID != record.ExecutionID {
		return record, false, errors.New("stale_revision: decision or run identity changed; refresh before answering")
	}
	var request DecisionRequest
	if err = decode(record.Request, &request); err != nil {
		return record, false, err
	}
	if err = validateDecisionAnswer(&request, &answer); err != nil {
		return record, false, err
	}
	answer.AnsweredAt = now()
	record.Answer, err = json.Marshal(answer)
	if err != nil {
		return record, false, err
	}
	record.AnsweredBy = answer.AnsweredBy
	record.Status = "answer_saved"
	record.Version++
	record.UpdatedAt = now()
	if err = t.PutDecision(record); err != nil {
		return record, false, err
	}
	if _, err = t.tx.ExecContext(t.ctx, `INSERT INTO decision_answers(project_identity,decision_id,decision_version,command_id,payload_digest,created_at) VALUES(?,?,?,?,?,?)`, command.ProjectIdentity, command.TargetID, command.ExpectedVersion, command.CommandID, operationDigest(mustJSON(struct{ OptionID, Note string }{answer.OptionID, answer.Note})), now().Format(time.RFC3339Nano)); err != nil {
		return record, false, err
	}
	command.Status = "accepted"
	command.Result = mustJSON(map[string]any{"decision_id": command.TargetID, "status": record.Status, "version": record.Version, "idempotent": false})
	if err = t.UpdateCommand(command); err != nil {
		return record, false, err
	}
	return record, false, nil
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
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return DomainSnapshot{}, err
	}
	defer tx.Rollback()
	snapshot, err := querySnapshot(ctx, tx, project)
	if err != nil {
		return snapshot, err
	}
	return snapshot, tx.Commit()
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

func (s *SQLiteStore) Command(ctx context.Context, project, commandID string) (CommandRecord, error) {
	var command CommandRecord
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT project_identity,command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at FROM commands WHERE project_identity=? AND command_id=?`, project, commandID).Scan(&command.ProjectIdentity, &command.CommandID, &command.Kind, &command.TargetID, &command.ExpectedVersion, &command.IdempotencyKey, &command.RequestedBy, &command.Payload, &command.Status, &command.Result, &created, &updated)
	if err != nil {
		return command, err
	}
	command.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return command, err
	}
	command.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return command, err
}

func (s *SQLiteStore) CommandByIdempotency(ctx context.Context, project, idempotencyKey string) (CommandRecord, error) {
	var command CommandRecord
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT project_identity,command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at FROM commands WHERE project_identity=? AND idempotency_key=?`, project, idempotencyKey).Scan(&command.ProjectIdentity, &command.CommandID, &command.Kind, &command.TargetID, &command.ExpectedVersion, &command.IdempotencyKey, &command.RequestedBy, &command.Payload, &command.Status, &command.Result, &created, &updated)
	if err != nil {
		return command, err
	}
	command.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return command, err
	}
	command.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return command, err
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
			persisted, _, err := tx.PutOperation(operation)
			if err != nil {
				return err
			}
			if persisted.OperationID != operation.OperationID {
				return errors.New("runtime contains a second identity for an existing operation intent")
			}
		}
		for _, runID := range runtime.RejectedClaimRuns {
			confirmed := false
			for _, operation := range runtime.Operations {
				var claim struct {
					RunID string `json:"run_id"`
				}
				if operation.Kind == "claim_task" && operation.Status == "rejected" && json.Unmarshal(operation.Request, &claim) == nil && claim.RunID == runID {
					confirmed = true
					break
				}
			}
			if !confirmed {
				return errors.New("cannot discard workflow without a rejected claim receipt")
			}
			if err := tx.(*sqliteTx).discardUnclaimedWorkflow(project, runID); err != nil {
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
		runtime.RejectedClaimRuns = nil
	}
	return runtime, err
}

func (t *sqliteTx) discardUnclaimedWorkflow(project, runID string) error {
	var taskID string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT task_id FROM workflow_runs WHERE project_identity=? AND workflow_run_id=? AND phase IN ('queued','executing')`, project, runID).Scan(&taskID); err != nil {
		return err
	}
	var effects int
	if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM executions e LEFT JOIN step_attempts a ON a.project_identity=e.project_identity AND a.execution_id=e.execution_id WHERE e.project_identity=? AND e.workflow_run_id=? AND (e.status <> 'pending' OR a.status <> 'planned')`, project, runID).Scan(&effects); err != nil {
		return err
	}
	if effects != 0 {
		return errors.New("cannot discard a workflow whose execution may have started")
	}
	if _, err := t.tx.ExecContext(t.ctx, `DELETE FROM step_attempts WHERE project_identity=? AND execution_id IN (SELECT execution_id FROM executions WHERE project_identity=? AND workflow_run_id=?)`, project, project, runID); err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(t.ctx, `DELETE FROM executions WHERE project_identity=? AND workflow_run_id=?`, project, runID); err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(t.ctx, `DELETE FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, project, runID); err != nil {
		return err
	}
	_, err := t.tx.ExecContext(t.ctx, `DELETE FROM tasks WHERE project_identity=? AND task_id=? AND NOT EXISTS (SELECT 1 FROM workflow_runs WHERE project_identity=? AND task_id=?)`, project, taskID, project, taskID)
	return err
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
		if run.Decision.Status == "answer_saved" || run.Decision.Status == "reply_accepted" || run.Decision.Status == "applied" || run.Decision.Status == "failed" {
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
		target.ID = targetFor(Config{Host: run.Host}).ID
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
		if d.Status != "pending" {
			if answer, e := ReadDecisionAnswer(run); e == nil {
				decision.Answer = mustJSON(answer)
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
	case "reply_accepted":
		return "reply_accepted"
	case "resuming":
		return "resuming"
	case "applied":
		return "applied"
	case "failed":
		return "failed"
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

func sameOperationRequest(a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}
	var compactA, compactB bytes.Buffer
	if json.Compact(&compactA, a) != nil || json.Compact(&compactB, b) != nil {
		return false
	}
	return bytes.Equal(compactA.Bytes(), compactB.Bytes())
}
func isConstraintError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "constraint")
}
