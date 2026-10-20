package driver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	host "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	storesqlite "github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

var debugHistory = []string{"environment_check_requests", "environment_checks", "test_generation_requests", "test_generations", "test_evidence", "task_deliveries", "task_results", "runner_events", "runner_candidates", "task_audits", "events", "commands", "projection"}

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
		dirs = append(dirs, filepath.Join(scope.repo, ".git"))
		if _, err := os.Stat(scope.stateDir()); err == nil {
			dirs = append(dirs, scope.stateDir())
		} else if !os.IsNotExist(err) {
			return err
		}
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
	proof, err := d.verifySharedRunnerReset()
	if err != nil {
		return err
	}
	for _, scope := range scopes {
		if err := scope.resetState(true, proof); err != nil {
			return err
		}
	}
	if check {
		return nil
	}
	for _, scope := range scopes {
		if err := scope.resetState(false, proof); err != nil {
			return err
		}
	}
	return nil
}
func (d *debugRelease) resetState(check bool, proof runnerclient.StoppedVerification) error {
	runDir := d.stateDir()
	if err := runnerstatepath.Validate(d.home); err != nil {
		return err
	}
	dbPath := filepath.Join(runDir, "steerline.db")
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		} // Fresh packages are initialized by the host, never reset into existence.
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
	stateDir := runnerstatepath.Directory(d.home)
	byRun := map[string]runnerclient.StoppedRun{}
	for _, r := range proof.Runs {
		byRun[r.RunID] = r
	}
	byCheck := map[string]runnerclient.StoppedCheck{}
	for _, c := range proof.Checks {
		byCheck[c.CheckID] = c
	}
	generations, err := tx.QueryContext(ctx, "SELECT payload FROM test_generations WHERE project_identity=?", project)
	if err != nil {
		return err
	}
	for generations.Next() {
		var raw []byte
		var g testmanagement.Generation
		if err = generations.Scan(&raw); err == nil {
			err = json.Unmarshal(raw, &g)
		}
		if err == nil {
			err = generationResetSafety(g, project, stateDir, byRun)
		}
		if err != nil {
			break
		}
	}
	if err == nil {
		err = generations.Err()
	}
	generations.Close()
	if err != nil {
		return err
	}
	checks, err := tx.QueryContext(ctx, "SELECT payload FROM environment_checks WHERE project_identity=?", project)
	if err != nil {
		return err
	}
	for checks.Next() {
		var raw []byte
		var record runnerstate.EnvironmentCheckRecord
		if err = checks.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal(raw, &record); err != nil {
			break
		}
		err = environmentCheckResetSafety(record, stateDir, byCheck)
		if err != nil {
			break
		}
	}
	if err == nil {
		err = checks.Err()
	}
	checks.Close()
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT payload FROM runner_candidates WHERE project_identity=?", project)
	if err != nil {
		return err
	}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			break
		}
		var c runnerbridge.Candidate
		if err = json.Unmarshal(raw, &c); err != nil {
			break
		}
		if !strings.EqualFold(filepath.Clean(c.StateDir), filepath.Clean(stateDir)) {
			err = errors.New("candidate Runner directory differs from this debug package")
			break
		}
		if c.RunID == "" && c.SubmitState != "rejected" && c.SubmitState != "cancelled" || c.RunID != "" && (byRun[c.RunID].RunID == "" || c.InputDigest == "" || byRun[c.RunID].InputDigest != c.InputDigest) {
			err = errors.New("original Runner resource is not proven stopped; preserve its recovery state")
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
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
	for _, name := range []string{"control", "service"} {
		target := filepath.Join(runDir, name)
		if !pathWithin(d.root, target) {
			return errors.New("reset target escaped debug root")
		}
		if err := filepath.WalkDir(target, func(path string, entry fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("linked reset path: %s", path)
			}
			if !entry.IsDir() {
				return debugPlainFile(path)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	// The short Runner namespace is outside the package, but bound to this
	// package's complete run directory. Validate it before changing the DB.
	if check {
		return nil
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
	for _, name := range []string{"control", "service", "application-commands"} {
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

func generationResetSafety(g testmanagement.Generation, project, stateDir string, runs map[string]runnerclient.StoppedRun) error {
	if g.Project != project || !strings.EqualFold(filepath.Clean(g.Runner.StateDir), filepath.Clean(stateDir)) {
		return errors.New("test generation belongs to another project/Runner directory")
	}
	if g.State == "rejected" && g.RunID == "" {
		return nil
	}
	r := runs[g.RunID]
	if g.RunID == "" || g.InputDigest == "" || r.RunID != g.RunID || r.InputDigest != g.InputDigest || !r.ResourcesSettled {
		return errors.New("original test generation is not proven stopped; preserve recovery state")
	}
	return nil
}

func environmentCheckResetSafety(record runnerstate.EnvironmentCheckRecord, stateDir string, byCheck map[string]runnerclient.StoppedCheck) error {
	if !strings.EqualFold(filepath.Clean(record.StateDir), filepath.Clean(stateDir)) {
		return errors.New("environment check belongs to another Runner directory")
	}
	if len(record.Operations) == 0 {
		return errors.New("environment check intent is incomplete")
	}
	if record.CheckID == "" && record.Operations[0].Status != "rejected" {
		return errors.New("environment check submission is unverified; preserve original request")
	}
	if record.CheckID != "" {
		v, ok := byCheck[record.CheckID]
		if !ok || !v.ResourcesSettled || record.Report == nil || record.Report.InputDigest == "" || record.Report.InputDigest != v.InputDigest {
			return errors.New("original environment check resource proof is absent or differs")
		}
	}
	for _, op := range record.Operations {
		if op.Status == "outcome_unknown" {
			return errors.New("environment check operation is unverified; preserve original request")
		}
	}
	return nil
}

// Verify the whole service before any project transaction, even if no task DB exists.
func (d *debugRelease) verifySharedRunnerReset() (runnerclient.StoppedVerification, error) {
	var proof runnerclient.StoppedVerification
	raw, err := os.ReadFile(filepath.Join(d.home, "execution.json"))
	if err != nil {
		return proof, err
	}
	var selected struct {
		Runner runnerclient.Selection `json:"runner"`
	}
	if err = json.Unmarshal(raw, &selected); err != nil {
		return proof, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cli, _, err := selected.Runner.Open(ctx, runnerstatepath.Directory(d.home), 20*time.Second)
	if err != nil {
		return proof, err
	}
	info, err := cli.ServiceStatus(ctx)
	if err != nil {
		return proof, err
	}
	if info.Status != "not_running" {
		return proof, errors.New("shared Runner must be stopped before resetting any project")
	}
	if _, err := os.Stat(cli.StateDir); os.IsNotExist(err) {
		return runnerclient.StoppedVerification{StateDir: cli.StateDir, ResourcesSettled: true, Runs: []runnerclient.StoppedRun{}, Checks: []runnerclient.StoppedCheck{}}, nil
	}
	return cli.VerifyStopped(ctx)
}
