package steerline

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
)

const domainSchemaVersion = sqlite.SchemaVersion

const activeWorkflowIndexV5 = `CREATE UNIQUE INDEX one_active_workflow_per_task ON workflow_runs(project_identity,task_id) WHERE phase NOT IN ('completed','abandoned')`

func (s *SQLiteStore) ensureSchema(ctx context.Context, createNew bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var tableCount int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`).Scan(&tableCount); err != nil {
		return err
	}
	if tableCount == 1 {
		var version int
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
			return nativeErr("unsupported_schema", "SQLite schema version cannot be inspected")
		}
		if version != domainSchemaVersion {
			return nativeErr("unsupported_schema", fmt.Sprintf("unsupported SQLite schema %d; preserve existing state", version))
		}
		if err = validateDatabaseSchema(ctx, tx); err != nil {
			return err
		}
		return tx.Commit()
	}
	if !createNew {
		return nativeErr("unsupported_schema", "existing SQLite database lacks a schema version; preserve it")
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&tableCount); err != nil {
		return err
	}
	if tableCount != 0 {
		return nativeErr("unsupported_schema", "SQLite database gained tables before initialization; preserve it")
	}
	if _, err = tx.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	for _, stmt := range append(append(append([]string{}, domainExecutionSchema...), nativeTaskSchema...), conversationSchema...) {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create domain schema: %w", err)
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, domainSchemaVersion, now().Format(time.RFC3339Nano)); err != nil {
		return err
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
	`CREATE TABLE workflow_runs (project_identity TEXT NOT NULL, workflow_run_id TEXT NOT NULL, task_id TEXT NOT NULL, phase TEXT NOT NULL, control_revision INTEGER NOT NULL DEFAULT 1 CHECK(control_revision>0), payload BLOB NOT NULL, PRIMARY KEY(project_identity, workflow_run_id), FOREIGN KEY(project_identity, task_id) REFERENCES tasks(project_identity, task_id))`,
	`CREATE INDEX workflow_runs_task_idx ON workflow_runs(project_identity, task_id)`,
	activeWorkflowIndexV5,
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

var conversationSchema = []string{
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
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_deliveries d JOIN project_task_authorities p ON p.project_identity=d.project_identity AND p.authority='native' JOIN tasks t ON t.project_identity=d.project_identity AND t.task_id=d.task_id WHERE t.status<>'closed' OR
		(json_type(d.evidence,'$.existing_acceptance') IS NULL AND NOT EXISTS (SELECT 1 FROM workflow_runs w JOIN executions e ON e.project_identity=w.project_identity AND e.workflow_run_id=w.workflow_run_id WHERE w.project_identity=d.project_identity AND w.workflow_run_id=d.selected_workflow_run_id AND w.task_id=d.task_id AND w.phase='completed' AND e.execution_id=json_extract(w.payload,'$.selected_result_id') AND ((e.status='succeeded' AND e.current_step='completed') OR (json_extract(w.payload,'$.workflow_version')='worker-flow-v1' AND json_extract(w.payload,'$.pr_url')=d.pr_url AND COALESCE(json_extract(e.payload,'$.session_id'),'')<>'' AND COALESCE(json_extract(e.payload,'$.workspace.branch'),'')<>'')))) OR
		(json_type(d.evidence,'$.existing_acceptance') IS NOT NULL AND (json_type(d.evidence,'$.existing_acceptance')<>'object' OR NOT EXISTS (SELECT 1 FROM workflow_runs w JOIN executions e ON e.project_identity=w.project_identity AND e.workflow_run_id=w.workflow_run_id WHERE w.project_identity=d.project_identity AND w.workflow_run_id=d.selected_workflow_run_id AND w.task_id=d.task_id AND w.phase='blocked' AND e.status='failed' AND json_extract(e.payload,'$.process') IS NULL)))`).Scan(&invalidDeliveries)
	if err != nil {
		return err
	}
	if invalidTasks != 0 || invalidBatches != 0 || invalidDeliveries != 0 {
		return nativeErr("contract_mismatch", "native task, batch, contract and delivery records must commit as one consistent transaction")
	}
	return nil
}
