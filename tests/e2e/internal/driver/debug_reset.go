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
	var locks []*host.RepoLock
	defer func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Close()
		}
	}()
	runDir := d.stateDir()
	lockDirs := []string{d.root, filepath.Join(d.repo, ".git"), runDir}
	if _, err := os.Stat(filepath.Join(d.root, "runtime")); err == nil {
		lockDirs = append(lockDirs, filepath.Join(d.root, "runtime"))
	}
	viewRoot := filepath.Join(d.home, "ui", "views")
	views, err := os.ReadDir(viewRoot)
	if err != nil {
		return err
	}
	for _, v := range views {
		if v.IsDir() {
			lockDirs = append(lockDirs, filepath.Join(viewRoot, v.Name()))
		}
	}
	for _, dir := range lockDirs {
		lock, err := host.LockRepository(dir)
		if err != nil {
			return fmt.Errorf("close this debug service, workers and consoles before reset: %w", err)
		}
		locks = append(locks, lock)
	}
	dbPath := filepath.Join(runDir, "steerline.db")
	if _, err = os.Stat(dbPath); err != nil {
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
	// Read and validate journals before mutating anything; originals live only in memory, not backups.
	type journal struct {
		path     string
		original []byte
		next     []byte
	}
	var journals []journal
	for _, v := range views {
		if !v.IsDir() {
			continue
		}
		path := filepath.Join(viewRoot, v.Name(), "view.json")
		raw, e := os.ReadFile(path)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		var value map[string]any
		if e = json.Unmarshal(raw, &value); e != nil {
			return e
		}
		projects, ok := value["projects"].(map[string]any)
		if value["version"] != float64(2) || !ok {
			return fmt.Errorf("unsupported console journal: %s", path)
		}
		changed := false
		for key, p := range projects {
			entry, ok := p.(map[string]any)
			if !ok {
				return errors.New("invalid console journal project")
			}
			config, _ := entry["config_path"].(string)
			run, _ := entry["run_dir"].(string)
			if strings.EqualFold(filepath.Clean(config), d.config) || strings.EqualFold(filepath.Clean(run), runDir) {
				delete(projects, key)
				changed = true
			}
		}
		if changed {
			value["updated_at"] = time.Now().UTC().Format(time.RFC3339Nano)
			next, e := json.MarshalIndent(value, "", "  ")
			if e != nil {
				return e
			}
			journals = append(journals, journal{path, raw, append(next, '\n')})
		}
	}
	fmt.Printf("Reset %d SteerlineTest tasks and %d console journals; preserve definitions, dependencies, Git and PR; no backup.\n", len(tasks), len(journals))
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
	changed := 0
	committed := false
	defer func() {
		if !committed {
			for i := 0; i < changed; i++ {
				_ = replaceDebugJournal(journals[i].path, journals[i].original)
			}
		}
	}()
	for _, j := range journals {
		if err = replaceDebugJournal(j.path, j.next); err != nil {
			return err
		}
		changed++
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	committed = true
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
func replaceDebugJournal(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".reset-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
