package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const domainSchemaVersion = SchemaVersion

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
	for _, stmt := range append(append([]string{}, taskServiceSchema...), nativeTaskSchema...) {
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
	`CREATE TABLE project_environments (project_identity TEXT NOT NULL, activity_id TEXT NOT NULL, revision INTEGER NOT NULL CHECK(revision>0), payload BLOB NOT NULL, PRIMARY KEY(project_identity,activity_id))`,
	`CREATE TABLE environment_requests (project_identity TEXT NOT NULL, request_id TEXT NOT NULL, activity_id TEXT NOT NULL, PRIMARY KEY(project_identity,request_id), FOREIGN KEY(project_identity,activity_id) REFERENCES project_environments(project_identity,activity_id))`,
	`CREATE TABLE maintenance_requests (project_identity TEXT NOT NULL, request_id TEXT NOT NULL, digest TEXT NOT NULL, result BLOB NOT NULL, PRIMARY KEY(project_identity,request_id))`,
	`CREATE TABLE project_materials (project_identity TEXT NOT NULL, task_id TEXT NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(project_identity,task_id))`,
	`CREATE TABLE maintenance_settings (project_identity TEXT PRIMARY KEY, profile TEXT NOT NULL, observation BLOB, generation_batch BLOB)`,
	`CREATE TABLE test_generations (project_identity TEXT NOT NULL, generation_id TEXT NOT NULL, revision INTEGER NOT NULL CHECK(revision>0), payload BLOB NOT NULL, PRIMARY KEY(project_identity,generation_id))`,
	`CREATE TABLE candidate_progress ( project_identity TEXT NOT NULL, candidate_id TEXT NOT NULL, revision INTEGER NOT NULL CHECK(revision>0), payload BLOB NOT NULL, PRIMARY KEY(project_identity,candidate_id), FOREIGN KEY(project_identity,candidate_id) REFERENCES runner_candidates(project_identity,candidate_id))`,
	`CREATE TABLE test_generation_requests (project_identity TEXT NOT NULL, request_id TEXT NOT NULL, generation_id TEXT NOT NULL, PRIMARY KEY(project_identity,request_id), FOREIGN KEY(project_identity,generation_id) REFERENCES test_generations(project_identity,generation_id))`,
	`CREATE TABLE environment_checks (project_identity TEXT NOT NULL, check_key TEXT NOT NULL, revision INTEGER NOT NULL CHECK(revision>0), payload BLOB NOT NULL, PRIMARY KEY(project_identity,check_key))`,
	`CREATE TABLE environment_check_requests (project_identity TEXT NOT NULL, request_id TEXT NOT NULL, check_key TEXT NOT NULL, PRIMARY KEY(project_identity,request_id), FOREIGN KEY(project_identity,check_key) REFERENCES environment_checks(project_identity,check_key))`,
	`CREATE TABLE runner_candidates (project_identity TEXT NOT NULL, candidate_id TEXT NOT NULL, task_id TEXT NOT NULL, revision INTEGER NOT NULL CHECK(revision>0), request_id TEXT NOT NULL, run_id TEXT NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(project_identity,candidate_id), UNIQUE(project_identity,candidate_id,task_id), UNIQUE(project_identity,request_id), FOREIGN KEY(project_identity,task_id) REFERENCES tasks(project_identity,task_id))`,
	`CREATE UNIQUE INDEX runner_candidate_run ON runner_candidates(project_identity,run_id) WHERE run_id<>''`,
	`CREATE TABLE runner_events (seq INTEGER PRIMARY KEY AUTOINCREMENT, project_identity TEXT NOT NULL, candidate_id TEXT NOT NULL, event_id TEXT NOT NULL, payload BLOB NOT NULL, UNIQUE(project_identity,candidate_id,event_id), FOREIGN KEY(project_identity,candidate_id) REFERENCES runner_candidates(project_identity,candidate_id))`,
	`CREATE TABLE task_results (project_identity TEXT NOT NULL, task_id TEXT NOT NULL, result_id TEXT NOT NULL, request_id TEXT NOT NULL, input BLOB NOT NULL, receipt BLOB NOT NULL, PRIMARY KEY(project_identity,task_id), UNIQUE(project_identity,result_id), UNIQUE(project_identity,request_id))`,
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

	`CREATE TABLE task_deliveries (project_identity TEXT NOT NULL, task_id TEXT NOT NULL, selected_workflow_run_id TEXT NOT NULL, pr_instance TEXT NOT NULL, pr_repository TEXT NOT NULL, pr_number INTEGER NOT NULL CHECK(pr_number>0), pr_url TEXT NOT NULL, merge_commit TEXT NOT NULL, merged_at TEXT NOT NULL, recorded_at TEXT NOT NULL, evidence BLOB NOT NULL, PRIMARY KEY(project_identity,task_id), UNIQUE(pr_instance,pr_repository,pr_number), FOREIGN KEY(project_identity,task_id) REFERENCES tasks(project_identity,task_id), FOREIGN KEY(project_identity,selected_workflow_run_id,task_id) REFERENCES runner_candidates(project_identity,candidate_id,task_id))`,
}

var taskServiceSchema = []string{
	`CREATE TABLE tasks (project_identity TEXT NOT NULL, task_id TEXT NOT NULL, external_provider TEXT NOT NULL DEFAULT '', external_project TEXT NOT NULL DEFAULT '', external_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, version INTEGER NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(project_identity, task_id))`,
	`CREATE UNIQUE INDEX task_external_identity ON tasks(project_identity,external_provider,external_project,external_id) WHERE external_id <> ''`,
	`CREATE TABLE events (event_id INTEGER PRIMARY KEY AUTOINCREMENT, project_identity TEXT NOT NULL, task_id TEXT NOT NULL DEFAULT '', workflow_run_id TEXT NOT NULL DEFAULT '', execution_id TEXT NOT NULL DEFAULT '', attempt_id TEXT NOT NULL DEFAULT '', external_source TEXT NOT NULL DEFAULT '', external_id TEXT NOT NULL DEFAULT '', type TEXT NOT NULL, received_at TEXT NOT NULL, payload_bytes INTEGER NOT NULL, payload_digest TEXT NOT NULL, payload BLOB NOT NULL)`,
	`CREATE UNIQUE INDEX events_source_identity_idx ON events(project_identity, external_source, external_id) WHERE external_source <> '' AND external_id <> ''`,
	`CREATE INDEX events_execution_idx ON events(project_identity, execution_id, attempt_id, event_id)`,
	`CREATE TABLE commands (project_identity TEXT NOT NULL, command_id TEXT NOT NULL, kind TEXT NOT NULL, target_id TEXT NOT NULL, expected_version INTEGER NOT NULL, idempotency_key TEXT NOT NULL, requested_by TEXT NOT NULL, payload BLOB NOT NULL, status TEXT NOT NULL, result BLOB, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(project_identity, command_id), UNIQUE(project_identity, idempotency_key))`,
	`CREATE INDEX commands_status_idx ON commands(project_identity, status, created_at)`,
	`CREATE TABLE projection (project_identity TEXT PRIMARY KEY, revision INTEGER NOT NULL, payload BLOB NOT NULL, updated_at TEXT NOT NULL)`,
	`CREATE TABLE task_audits (project_identity TEXT NOT NULL, command_id TEXT NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(project_identity,command_id), FOREIGN KEY(project_identity,command_id) REFERENCES commands(project_identity,command_id))`,
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
		OR (t.status<>'discarded' AND (json_extract(t.payload,'$.discarded_at') IS NOT NULL OR COALESCE(json_extract(t.payload,'$.discard_reason'),'')<>'' OR COALESCE(json_extract(t.payload,'$.discarded_by'),'')<>'' OR COALESCE(json_extract(t.payload,'$.discard_request_id'),'')<>'' OR COALESCE(json_array_length(json_extract(t.payload,'$.stop_pending')),0)<>0))
		OR (t.status='closed' AND NOT EXISTS (SELECT 1 FROM task_deliveries d WHERE d.project_identity=t.project_identity AND d.task_id=t.task_id))
		OR (t.status='discarded' AND EXISTS (SELECT 1 FROM task_deliveries d WHERE d.project_identity=t.project_identity AND d.task_id=t.task_id))`).Scan(&invalidTasks)
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_batches b JOIN project_task_authorities p ON p.project_identity=b.project_identity AND p.authority='native' JOIN json_each(b.task_ids) m WHERE NOT EXISTS (SELECT 1 FROM tasks t WHERE t.project_identity=b.project_identity AND t.task_id=m.value AND json_extract(t.payload,'$.batch_id')=b.batch_id)`).Scan(&invalidBatches)
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_deliveries d JOIN project_task_authorities p ON p.project_identity=d.project_identity AND p.authority='native' JOIN tasks t ON t.project_identity=d.project_identity AND t.task_id=d.task_id
		WHERE t.status<>'closed' OR NOT EXISTS (SELECT 1 FROM runner_candidates c WHERE c.project_identity=d.project_identity AND c.candidate_id=d.selected_workflow_run_id AND c.task_id=d.task_id
		AND json_extract(c.payload,'$.participation')='completed' AND coalesce(c.run_id,'')<>'' AND json_extract(c.payload,'$.pr_url')=d.pr_url AND json_extract(c.payload,'$.stop_requested')=1
		AND coalesce(json_extract(c.payload,'$.input_digest'),'')<>'' AND coalesce(json_extract(c.payload,'$.branch'),'')<>'' )`).Scan(&invalidDeliveries)
	if err != nil {
		return err
	}
	if invalidTasks != 0 || invalidBatches != 0 || invalidDeliveries != 0 {
		return nativeErr("contract_mismatch", "native task, batch, contract and delivery records must commit as one consistent transaction")
	}
	return nil
}
