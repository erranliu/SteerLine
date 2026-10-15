package driver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	host "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/taskrunner"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

// Explicit opt-in: this test consumes only a newly generated package, never an installed home.
func TestDebugReleaseRuntime(t *testing.T) {
	root := os.Getenv("STEERLINE_DEBUG_TEST_ROOT")
	if root == "" {
		t.Skip("run scripts/test-debug-release.ps1 to create an independent debug package")
	}
	d, err := openDebugRelease(root)
	if err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(filepath.Join(root, "scheme.json")); err == nil {
		t.Fatalf("requires a fresh debug package, found %s", raw)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Run preparation in a child so this test never changes its own CODEX_HOME/Git/auth environment.
	prepare := d.command(d.helper, "debug", "prepare", "--simulate")
	out, err := prepare.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare: %v %s", err, out)
	}
	c := d.client()
	page, err := c.Tasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tasks) != 2 {
		t.Fatalf("seed task count: %d", len(page.Tasks))
	}
	serviceCmd := d.command(d.helper, "debug", "run", "--simulate")
	log, err := os.Create(filepath.Join(root, "evidence", "service-test.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	serviceCmd.Stdout, serviceCmd.Stderr = log, log
	service, err := testprocess.Start(serviceCmd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		service.Stop(stopCtx)
	}()
	wait := func(label string, predicate func() bool) {
		t.Helper()
		for ctx.Err() == nil {
			if predicate() {
				return
			}
			select {
			case <-service.Done:
				t.Fatalf("%s: service exited; inspect %s", label, log.Name())
			default:
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal(label, " timeout")
	}
	wait("service lock", func() bool {
		if _, err := os.Stat(filepath.Join(root, "runtime", "steerline.lock")); err != nil {
			return false
		}
		lock, err := host.LockRepository(filepath.Join(root, "runtime"))
		if err != nil {
			return true
		}
		lock.Close()
		return false
	})
	if err = d.reset(false); err == nil {
		t.Fatal("reset accepted active service")
	}
	readyCommon, err := d.git("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		t.Fatal(err)
	}
	wait("service snapshot", func() bool {
		snap, err := ReadApplicationProjectSnapshot(ctx, filepath.Join(d.stateDir(), "steerline.db"), strings.ToLower(filepath.Clean(readyCommon)), "", 100)
		if err != nil || snap.RunnerError != nil || snap.RunnerState.Checkpoint == nil {
			return false
		}
		runtime := taskrunner.Project(snap.RunnerState, *snap.RunnerState.Checkpoint)
		return runtime.Service.Ready
	})
	for _, action := range []string{"resume_dispatch", "start_task"} {
		args := []string{"debug", "control", "--simulate", "-config", d.config, "-action", action, "-profile", "simulate"}
		if action == "start_task" {
			args = append(args, "-task-id", "1")
		}
		cmd := d.command(d.helper, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v %s", action, err, out)
		}
	}
	wait("published candidate", func() bool {
		if common, err := d.git("rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
			if snap, err := ReadWorkflow(ctx, filepath.Join(d.stateDir(), "steerline.db"), strings.ToLower(filepath.Clean(common))); err == nil {
				for _, candidate := range snap.Candidates {
					if candidate.SubmitState == "rejected" {
						t.Fatal("Runner rejected original submission: ", candidate.Diagnostic)
					}
					if candidate.Projection != nil && candidate.Projection.ResumeRequired {
						t.Fatal("Runner blocked: ", candidate.Projection.Diagnostic)
					}
				}
			}
		}
		state, err := controlled.Read(d.provider)
		return err == nil && state.PR.State == "open" && len(state.Turns) == 3
	})
	state, err := controlled.Read(d.provider)
	if err != nil {
		t.Fatal(err)
	}
	if state.PR.Repository != debugRepository || state.PR.BaseBranch != d.base {
		t.Fatal("simulation escaped SteerlineTest/base")
	}
	common, err := d.git("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		t.Fatal(err)
	}
	project := strings.ToLower(filepath.Clean(common))
	db := filepath.Join(d.stateDir(), "steerline.db")
	wait("await merge checkpoint", func() bool {
		snap, err := ReadWorkflow(ctx, db, project)
		return err == nil && len(snap.Candidates) == 1 && snap.Candidates[0].Projection != nil && snap.Candidates[0].Projection.Status == "pr_ready"
	})
	before, err := ReadWorkflow(ctx, db, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Candidates) != 1 || before.Candidates[0].Projection == nil || before.Candidates[0].Projection.Status != "pr_ready" {
		t.Fatal("not waiting for actual PR merge")
	}
	cmd := d.command(d.helper, "debug", "merge", "--simulate")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("merge: %v %s", err, out)
	}
	wait("delivery", func() bool {
		snap, err := ReadWorkflow(ctx, db, project)
		return err == nil && len(snap.Graph.Deliveries) == 1
	})
	snap, err := ReadApplicationProjectSnapshot(ctx, db, project, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	proof := filepath.Join(root, "evidence", "controlled-delivery.json")
	if err = controlled.Write(proof, snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.ProjectView.Ready.Tasks) != 1 {
		t.Fatalf("expected one dependency-ready successor: %+v", snap.ProjectView.Ready)
	}
	wait("original Runner stopped", func() bool {
		state, err := ReadWorkflow(ctx, db, project)
		if err != nil || len(state.Candidates) != 1 {
			return false
		}
		c := state.Candidates[0]
		return c.InputsRevoked && c.Projection != nil && c.Projection.StopStatus == "stopped"
	})
	stop := d.command(d.binary, "stop-service", "-config", d.config)
	out, err = stop.CombinedOutput()
	if err != nil {
		t.Fatalf("stop: %v %s", err, out)
	}
	if err = service.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	head, err := d.git("rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	providerBefore, err := os.ReadFile(d.provider)
	if err != nil {
		t.Fatal(err)
	}
	viewDir := filepath.Join(d.home, "ui", "views", "reset-test")
	if err = os.MkdirAll(viewDir, 0700); err != nil {
		t.Fatal(err)
	}
	journal := map[string]any{"version": 2, "projects": map[string]any{"target": map[string]string{"config_path": d.config, "run_dir": filepath.Dir(db)}, "unrelated": map[string]string{"config_path": "other"}}}
	if err = controlled.Write(filepath.Join(viewDir, "view.json"), journal); err != nil {
		t.Fatal(err)
	}
	viewLock, err := host.LockRepository(viewDir)
	if err != nil {
		t.Fatal(err)
	}
	if err = d.reset(false); err == nil {
		viewLock.Close()
		t.Fatal("reset accepted active console lock")
	}
	viewLock.Close()
	if err = d.reset(true); err != nil {
		t.Fatal(err)
	}
	unchanged, err := ReadWorkflow(ctx, db, project)
	if err != nil || len(unchanged.Graph.Deliveries) != 1 {
		t.Fatal("check modified data", err)
	}
	if err = d.reset(false); err != nil {
		t.Fatal(err)
	}
	after, err := ReadWorkflow(ctx, db, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Candidates) != 0 || len(after.Graph.Deliveries) != 0 {
		t.Fatal("execution state remained")
	}
	page, err = c.Tasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tasks) != 2 {
		t.Fatal("tasks lost")
	}
	for _, task := range page.Tasks {
		if task.Status != "open" {
			t.Fatal("task not open")
		}
	}
	newHead, err := d.git("rev-parse", "HEAD")
	if err != nil || newHead != head {
		t.Fatal("reset changed Git", err)
	}
	providerAfter, err := os.ReadFile(d.provider)
	if err != nil || string(providerBefore) != string(providerAfter) {
		t.Fatal("reset changed PR facts", err)
	}
	raw, err := os.ReadFile(filepath.Join(viewDir, "view.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &journal); err != nil {
		t.Fatal(err)
	}
	projects := journal["projects"].(map[string]any)
	if len(projects) != 1 || projects["unrelated"] == nil {
		t.Fatal("console reset scope incorrect")
	}
	if _, err = os.Stat(filepath.Join(root, "backup")); !os.IsNotExist(err) {
		t.Fatal("unexpected backup")
	}
	cmd = d.command(d.helper, "debug", "prepare")
	if _, err = cmd.CombinedOutput(); err == nil {
		t.Fatal("scheme switched from simulate to live")
	}
	cmd = d.command(d.helper, "debug", "control", "--simulate", "-config", `C:\SteerlineRuntime\etc\SteerlineTest.json`, "-action", "start_task", "-profile", "simulate", "-task-id", "1")
	if _, err = cmd.CombinedOutput(); err == nil {
		t.Fatal("control accepted installed config")
	}
	linked := filepath.Join(root, "logs", "external-hardlink.txt")
	sentinel := filepath.Join(filepath.Dir(root), "sentinel.txt")
	if err = os.WriteFile(sentinel, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Link(sentinel, linked); err != nil {
		t.Fatal(err)
	}
	if _, err = openDebugRelease(root); err == nil {
		t.Fatal("hard link accepted")
	}
	if err = os.Remove(linked); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(sentinel)
	if err != nil || string(raw) != "unchanged" {
		t.Fatal("external sentinel changed", err)
	}
}
