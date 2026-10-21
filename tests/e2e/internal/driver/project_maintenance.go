package driver

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"os"
	"path/filepath"
	"time"
)

// projectMaintenance reuses the single package launcher and its isolated Git
// transport. Browser actions use the authenticated public Go/Web protocol.
func (d *debugRelease) projectMaintenance(source string) (result error) {
	if !filepath.IsAbs(source) {
		return errors.New("maintenance source must be absolute")
	}
	out := filepath.Join(d.root, "evidence", "project-maintenance")
	if e := os.MkdirAll(out, 0700); e != nil {
		return e
	}
	log, e := os.Create(filepath.Join(out, "runtime.log"))
	if e != nil {
		return e
	}
	defer log.Close()
	cmd := d.command(d.helper, "debug", "run", "--simulate", "--no-open")
	cmd.Stdout = log
	cmd.Stderr = log
	process, e := testprocess.Start(cmd)
	if e != nil {
		return e
	}
	stopped := false
	defer func() {
		stopErr := d.stop()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		exitErr := process.Wait(ctx)
		stopped = stopErr == nil && exitErr == nil
		settled := false
		if stopped {
			_, verifyErr := d.tmCommand(out, "verify-stop", d.binary, "verify-stop", "--runner-exe", d.helper)
			settled = verifyErr == nil
			result = errors.Join(result, verifyErr)
		}
		if result == nil && stopped && settled {
			result = d.reset(false)
			if result == nil {
				result = d.verifyMaintenanceReset(out)
			}
		}
		if !stopped {
			result = errors.Join(result, fmt.Errorf("maintenance closure: stop=%v process=%v", stopErr, exitErr))
			_ = process.Stop(ctx)
		}
		result = errors.Join(result, controlled.Write(filepath.Join(out, "proof.json"), map[string]any{"passed": result == nil, "stopped": stopped, "resources_settled": settled, "package": d.root, "source": source, "environment": "controlled"}))
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if e = tmPoll(ctx, func() (bool, error) {
		select {
		case <-process.Done:
			return false, errors.New("debug launcher exited before readiness; inspect runtime.log")
		default:
		}
		_, e := os.Stat(filepath.Join(d.root, "runtime-launcher", "ready.json"))
		return e == nil, nil
	}); e != nil {
		return e
	}
	browser := d.command("npm.cmd", "exec", "--", "playwright", "test", "tests/project-maintenance.spec.ts")
	browser.Dir = filepath.Join(source, "web")
	browser.Env = append(browser.Env, "STEERLINE_WEB_CONFIG="+d.config, "STEERLINE_WEB_EVIDENCE="+filepath.Join(out, "browser"))
	var buffer bytes.Buffer
	browser.Stdout = &buffer
	browser.Stderr = &buffer
	browserContext, browserCancel := context.WithTimeout(context.Background(), 330*time.Second)
	e = testprocess.Run(browserContext, browser)
	browserCancel()
	b := buffer.Bytes()
	_ = os.WriteFile(filepath.Join(out, "browser.log"), b, 0600)
	if e != nil {
		return fmt.Errorf("maintenance browser: %w: %s", e, b)
	}
	return nil
}

func (d *debugRelease) verifyMaintenanceReset(out string) error {
	scopes := []*debugRelease{d}
	if d.webProjects == 2 {
		scopes = append(scopes, d.secondary())
	}
	facts := map[string]int{}
	for _, scope := range scopes {
		db, e := sql.Open("sqlite", filepath.Join(scope.stateDir(), "steerline.db"))
		if e != nil {
			return e
		}
		for _, table := range []string{"task_execution_snapshots", "tasks", "task_contracts"} {
			var count int
			e = db.QueryRow("SELECT count(*) FROM " + table).Scan(&count)
			if e != nil {
				db.Close()
				return e
			}
			facts[filepath.Base(scope.config)+":"+table] = count
			if table == "task_execution_snapshots" && count != 0 || table != "task_execution_snapshots" && count != 2 {
				db.Close()
				return errors.New("reset lost task definitions or retained execution snapshot")
			}
		}
		db.Close()
	}
	return controlled.Write(filepath.Join(out, "reset.json"), facts)
}
