package driver

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	host "github.com/erranliu/SteerLine/internal/app/host"
	reconstruction "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

func TestDebugReleaseWebProjects(t *testing.T) {
	root := os.Getenv("STEERLINE_DEBUG_SECOND_TEST_ROOT")
	if root == "" {
		t.Skip("requires the second fresh package from scripts/test-debug-release.ps1")
	}
	d, err := openDebugRelease(root)
	if err != nil {
		t.Fatal(err)
	}
	if d.webProjects != 2 {
		t.Fatal("requires two independent projects")
	}
	if _, err := d.command(d.helper, "debug", "console", "--simulate").CombinedOutput(); err == nil {
		t.Fatal("retired debug console accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "scheme.json")); !os.IsNotExist(err) {
		t.Fatal("retired operation froze the execution scheme")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if out, err := d.command(d.helper, "debug", "prepare", "--simulate").CombinedOutput(); err != nil {
		t.Fatalf("prepare: %v %s", err, out)
	}
	var webConfig struct {
		Address string `json:"address"`
	}
	raw, err := os.ReadFile(filepath.Join(d.home, "web.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &webConfig); err != nil {
		t.Fatal(err)
	}
	occupied, err := net.Listen("tcp", webConfig.Address)
	if err != nil {
		t.Fatal(err)
	}
	conflict := d.command(d.helper, "debug", "run", "--simulate")
	conflictProcess, err := testprocess.Start(conflict)
	if err != nil {
		occupied.Close()
		t.Fatal(err)
	}
	conflictCtx, conflictDone := context.WithTimeout(ctx, 30*time.Second)
	conflictErr := conflictProcess.Wait(conflictCtx)
	conflictDone()
	cleanup, cleanupDone := context.WithTimeout(context.Background(), 10*time.Second)
	conflictProcess.Stop(cleanup)
	cleanupDone()
	occupied.Close()
	if conflictErr == nil || conflictCtx.Err() == context.DeadlineExceeded {
		t.Fatalf("Web port conflict did not fail promptly: %v", conflictErr)
	}
	if err := d.reset(true); err != nil {
		t.Fatalf("failed Web startup leaked package activity: %v", err)
	}
	log, err := os.Create(filepath.Join(root, "evidence", "two-project-runtime.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	command := d.command(d.helper, "debug", "run", "--simulate")
	command.Stdout, command.Stderr = log, log
	owner, err := testprocess.Start(command)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		owner.Stop(cleanup)
	}()
	scopes := []*debugRelease{d, d.secondary()}
	identities := map[string]string{}
	for _, scope := range scopes {
		common, err := scope.git("rev-parse", "--path-format=absolute", "--git-common-dir")
		if err != nil {
			t.Fatal(err)
		}
		runDir := scope.stateDir()
		ready := false
		for ctx.Err() == nil {
			snapshot, err := ReadHTTPProject(ctx, runDir, strings.ToLower(filepath.Clean(common)), "")
			if err == nil && snapshot.NativeTasks != nil && snapshot.Connection.Snapshot.Page != nil && snapshot.Connection.Snapshot.Page.Service.Ready {
				if len(snapshot.NativeTasks.Tasks) != 2 {
					t.Fatal("dual-project package lost task definitions")
				}
				identities[snapshot.NativeTasks.ProjectID] = snapshot.NativeTasks.DatabaseID
				ready = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !ready {
			t.Fatal("dual-project HTTP readiness timed out")
		}
	}
	if len(identities) != 2 {
		t.Fatal("projects share identity")
	}
	if out, err := d.command(filepath.Join(root, "bin", "steerline-web.exe"), "open", "-print-url").CombinedOutput(); err != nil {
		t.Fatalf("Web not available: %v %s", err, out)
	}
	if err := d.reset(false); err == nil {
		t.Fatal("reset accepted active dual-project package")
	}
	for _, entry := range []struct {
		exe  string
		args []string
	}{
		{filepath.Join(root, "bin", "steerline-web.exe"), []string{"stop"}},
		{d.binary, []string{"stop-app"}},
		{d.binary, []string{"stop-service", "-config", d.config}},
		{d.binary, []string{"stop-service", "-config", d.secondary().config}},
	} {
		if out, err := d.command(entry.exe, entry.args...).CombinedOutput(); err != nil {
			t.Fatalf("normal stop: %v %s", err, out)
		}
	}
	if err := owner.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err := d.reset(true); err != nil {
		t.Fatal(err)
	}
	if err := d.reset(false); err != nil {
		t.Fatal(err)
	}
	if err := controlled.Write(filepath.Join(root, "evidence", "two-project-reset.json"), map[string]any{"passed": true, "projects": identities, "normal_exit": true}); err != nil {
		t.Fatal(err)
	}
}

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
	fixture, err := controlled.Read(d.provider)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Recovery = &controlled.RecoveryFixture{HoldStep: "implement.execute", HoldStream: true}
	if err := controlled.Write(d.provider, fixture); err != nil {
		t.Fatal(err)
	}
	c := d.client()
	serviceCmd := d.command(d.helper, "debug", "run", "--simulate", "--no-open")
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
		stopCtx, cancel := context.WithTimeout(context.Background(), 130*time.Second)
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
		if _, err := os.Stat(filepath.Join(root, "runtime-launcher", "steerline.lock")); err != nil {
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
		if err != nil || snap.ExecutionState.Checkpoint == nil {
			return false
		}
		runtime, err := reconstruction.BuildFromStorage(snap.ExecutionState, *snap.ExecutionState.Checkpoint)
		return err == nil && runtime.Service.Ready
	})
	page, err := c.Tasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tasks) != 2 {
		t.Fatalf("seed task count: %d", len(page.Tasks))
	}
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
	wait("original active worker", func() bool {
		state, err := controlled.Read(d.provider)
		return err == nil && len(state.Turns) == 1 && state.Turns[0]["status"] == "inProgress"
	})
	if err := d.reset(false); err == nil {
		t.Fatal("reset accepted active worker")
	}
	if out, err := d.command(d.helper, "release-recovery", "--e2e-state", d.provider).CombinedOutput(); err != nil {
		t.Fatalf("release: %v %s", err, out)
	}
	wait("published candidate", func() bool {
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
		return err == nil && len(snap.WorkflowRuns) == 1 && snap.WorkflowRuns[0].StepID == "await_merge"
	})
	before, err := ReadWorkflow(ctx, db, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.WorkflowRuns) != 1 || before.WorkflowRuns[0].StepID != "await_merge" {
		t.Fatal("not waiting for actual PR merge")
	}
	cmd := d.command(d.helper, "debug", "merge", "--simulate")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("merge: %v %s", err, out)
	}
	wait("delivery", func() bool {
		snap, err := ReadWorkflow(ctx, db, project)
		return err == nil && len(snap.Deliveries) == 1
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
	stop := d.command(d.helper, "debug", "stop", "--simulate")
	out, err = stop.CombinedOutput()
	if err != nil {
		t.Fatalf("stop: %v %s", err, out)
	}
	if err = d.reset(false); err == nil {
		t.Fatal("reset accepted active application/Web after project stop")
	}
	for _, entry := range []struct {
		exe  string
		args []string
	}{{filepath.Join(root, "bin", "steerline-web.exe"), []string{"stop"}}, {d.binary, []string{"stop-app"}}} {
		out, err := d.command(entry.exe, entry.args...).CombinedOutput()
		if err != nil {
			t.Fatalf("normal stop: %v %s", err, out)
		}
		if entry.args[0] == "stop" {
			if err := d.reset(false); err == nil {
				t.Fatal("reset accepted active application")
			}
		}
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
	if err = d.reset(true); err != nil {
		t.Fatal(err)
	}
	unchanged, err := ReadWorkflow(ctx, db, project)
	if err != nil || len(unchanged.Deliveries) != 1 {
		t.Fatal("check modified data", err)
	}
	if err = d.reset(false); err != nil {
		t.Fatal(err)
	}
	after, err := ReadWorkflow(ctx, db, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.WorkflowRuns) != 0 || len(after.Deliveries) != 0 {
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
	raw, err := os.ReadFile(sentinel)
	if err != nil || string(raw) != "unchanged" {
		t.Fatal("external sentinel changed", err)
	}
}
