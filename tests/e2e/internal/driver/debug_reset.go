package driver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	host "github.com/erranliu/SteerLine/internal/app/host"
	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	processfacts "github.com/erranliu/SteerLine/internal/execution/process"
	storesqlite "github.com/erranliu/SteerLine/internal/storage/sqlite"
)

var debugHistory = []string{"decision_answers", "decisions", "task_deliveries", "task_results", "conversation_events", "conversation_bodies", "conversation_source_cursors", "step_attempts", "executions", "workflow_runs", "events", "operations", "commands", "projection"}

func (d *debugRelease) reset(check bool) error {
	scopes := []*debugRelease{d}
	if d.webProjects == 2 {
		scopes = append(scopes, d.secondary())
	}
	dirs := []string{d.root}
	if _, err := os.Stat(filepath.Join(d.root, "runtime-launcher")); err == nil {
		dirs = append(dirs, filepath.Join(d.root, "runtime-launcher"))
	}
	for _, scope := range scopes {
		dirs = append(dirs, filepath.Join(scope.repo, ".git"), scope.stateDir())
		runtimeDir := filepath.Join(d.root, "runtime"+scope.suffix())
		if _, err := os.Stat(runtimeDir); err == nil {
			dirs = append(dirs, runtimeDir)
		}
	}
	for _, dir := range []string{filepath.Join(d.home, "application-api"), filepath.Join(d.home, "ui", "web")} {
		if _, err := os.Stat(dir); err == nil {
			dirs = append(dirs, dir)
		}
	}
	var locks []*host.RepoLock
	defer func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Close()
		}
	}()
	for _, dir := range dirs {
		lock, err := host.LockRepository(dir)
		if err != nil {
			return fmt.Errorf("close this debug service, Web, application gateway, workers before reset: %w", err)
		}
		locks = append(locks, lock)
	}
	for _, scope := range scopes {
		if err := scope.resetState(true); err != nil {
			return err
		}
	}
	if check {
		return nil
	}
	for _, scope := range scopes {
		if err := scope.resetState(false); err != nil {
			return err
		}
	}
	return nil
}
func (d *debugRelease) resetState(check bool) error {
	runDir := d.stateDir()
	dbPath := filepath.Join(runDir, "steerline.db")
	if _, err := os.Stat(dbPath); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err = db.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var schema int
	if err = tx.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&schema); err != nil {
		return err
	}
	if schema != storesqlite.SchemaVersion {
		return fmt.Errorf("reset requires current schema %d, found %d", storesqlite.SchemaVersion, schema)
	}
	var project string
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(MIN(project_identity),'') FROM project_task_authorities").Scan(&count, &project); err != nil {
		return err
	}
	if count != 1 || !strings.EqualFold(filepath.Clean(project), filepath.Join(d.repo, ".git")) {
		return errors.New("reset project does not belong to this debug package")
	}
	rows, err := tx.QueryContext(ctx, "SELECT payload FROM executions WHERE project_identity=?", project)
	if err != nil {
		return err
	}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var value struct {
			Process *backend.ProcessIdentity `json:"process"`
		}
		if err = json.Unmarshal(raw, &value); err != nil {
			rows.Close()
			return err
		}
		if value.Process != nil {
			facts := processfacts.ServiceFacts{}.Probe(value.Process)
			if facts.State != "absent" {
				rows.Close()
				return fmt.Errorf("worker is not proven absent: %s", facts.State)
			}
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	type task struct {
		id      string
		version uint64
		value   map[string]any
	}
	var tasks []task
	rows, err = tx.QueryContext(ctx, "SELECT task_id,version,payload FROM tasks WHERE project_identity=? ORDER BY task_id", project)
	if err != nil {
		return err
	}
	for rows.Next() {
		var t task
		var raw []byte
		if err = rows.Scan(&t.id, &t.version, &raw); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(raw, &t.value); err != nil {
			rows.Close()
			return err
		}
		tasks = append(tasks, t)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	fmt.Printf("Reset %d SteerlineTest tasks; preserve definitions, dependencies, Git and PR; no backup.\n", len(tasks))
	if check {
		return nil
	}
	for _, table := range []string{"execution_bindings", "execution_operations"} {
		if _, err = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE run_id IN (SELECT workflow_run_id FROM workflow_runs WHERE project_identity=?)", project); err != nil {
			return err
		}
	}
	for _, table := range debugHistory {
		if _, err = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE project_identity=?", project); err != nil {
			return err
		}
	}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	for _, t := range tasks {
		for _, key := range []string{"claimant", "discarded_at", "discard_reason", "discarded_by", "discard_request_id", "stop_pending"} {
			delete(t.value, key)
		}
		t.value["status"] = "open"
		t.value["version"] = t.version + 1
		t.value["updated_at"] = stamp
		raw, e := json.Marshal(t.value)
		if e != nil {
			return e
		}
		if _, err = tx.ExecContext(ctx, "UPDATE tasks SET status='open',version=?,payload=? WHERE project_identity=? AND task_id=?", t.version+1, raw, project, t.id); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE project_task_authorities SET graph_revision=graph_revision+1,dispatch_mode='manual',dispatch_paused=0,updated_at=? WHERE project_identity=?", stamp, project); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE database_state SET revision=revision+1 WHERE singleton=1"); err != nil {
		return err
	}
	rows, err = tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	invalid := rows.Next()
	rows.Close()
	if invalid {
		return errors.New("reset foreign key validation failed")
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	// These are local control/runtime journals, not Git, provider facts or authentication.
	for _, name := range []string{"control", "service", "runs"} {
		path := filepath.Join(runDir, name)
		if !pathWithin(d.root, path) {
			return errors.New("reset path escaped root")
		}
		resolved, e := filepath.EvalSymlinks(path)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if !pathWithin(d.root, resolved) {
			return errors.New("resolved reset target escaped debug root")
		}
		if err = os.RemoveAll(path); err != nil {
			return err
		}
	}
	fmt.Println("DONE: SteerlineTest state reset without backup")
	return nil
}
