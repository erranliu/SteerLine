package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	app "github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

// Opt-in process integration; preparation is always the repository's single generator.
func TestDebugReleaseSharedRunnerService(t *testing.T) {
	root := os.Getenv("STEERLINE_DEBUG_SECOND_TEST_ROOT")
	if root == "" {
		t.Skip("requires a fresh dual-project package from new-debug-release.ps1")
	}
	d, err := openDebugRelease(root)
	if err != nil {
		t.Fatal(err)
	}
	if d.webProjects != 2 {
		t.Fatal("requires two projects")
	}
	if _, err := os.Stat(filepath.Join(root, "scheme.json")); !os.IsNotExist(err) {
		t.Fatal("requires a fresh unfrozen package")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	log, err := os.Create(filepath.Join(root, "evidence", "shared-runner-service.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	var owners []*testprocess.Process
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 130*time.Second)
		defer done()
		if err := runCoordinator(cleanup, d.command(d.binary, "stop")); err != nil {
			t.Errorf("owned products cleanup: %v", err)
		}
		for _, owner := range owners {
			if err := owner.Stop(cleanup); err != nil {
				t.Errorf("owned launcher cleanup: %v", err)
			}
		}
	}()
	var observations []any
	var cli runnerclient.Client
	start := func() runnerclient.ServiceInfo {
		command := d.command(d.helper, "debug", "run", "--no-open")
		command.Stdout, command.Stderr = log, log
		owner, err := testprocess.Start(command)
		if err != nil {
			t.Fatal(err)
		}
		owners = append(owners, owner)
		for {
			raw, err := d.command(d.binary, "status", "--json").Output()
			var status app.ApplicationStatus
			if err == nil && json.Unmarshal(raw, &status) == nil && len(status.Projects) == 2 && status.Projects[0].Online && status.Projects[1].Online && debugRunReady(root) {
				break
			}
			select {
			case <-owner.Done:
				t.Fatal("launcher exited before readiness", log.Name())
			case <-ctx.Done():
				t.Fatal("real service readiness", ctx.Err(), log.Name())
			case <-time.After(100 * time.Millisecond):
			}
		}
		raw, err := os.ReadFile(filepath.Join(d.home, "execution.json"))
		if err != nil {
			t.Fatal(err)
		}
		var config app.RunnerExecutionConfig
		if err = json.Unmarshal(raw, &config); err != nil {
			t.Fatal(err)
		}
		cli, _, err = config.Runner.Open(ctx, runnerstatepath.Directory(d.home), 20*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		info, err := cli.ServiceStatus(ctx)
		if err != nil || info.Status != "ready" || info.PID == 0 || info.InstanceID == "" {
			t.Fatal("real service identity", info, err)
		}
		for _, scope := range []*debugRelease{d, d.secondary()} {
			common, err := scope.git("rev-parse", "--path-format=absolute", "--git-common-dir")
			if err != nil {
				t.Fatal(err)
			}
			snap, err := ReadApplicationProjectSnapshot(ctx, filepath.Join(scope.stateDir(), "steerline.db"), strings.ToLower(filepath.Clean(common)), "", 100)
			if err != nil || snap.RunnerState.Checkpoint == nil {
				t.Fatal("project binding snapshot", err)
			}
			pin := snap.RunnerState.Checkpoint.Service.RunnerBuild
			if pin == nil || pin.StateDir != cli.StateDir || pin.SHA256 != info.ExecutableSHA256 || len(snap.RunnerState.Candidates) != 0 {
				t.Fatal("project uses another Runner or dispatched a task", pin)
			}
		}
		observations = append(observations, info)
		return info
	}
	first := start()
	if err := runCoordinator(ctx, d.command(d.binary, "start", "--no-open")); err != nil {
		t.Fatal(err)
	}
	repeated, err := cli.ServiceStatus(ctx)
	if err != nil || repeated.InstanceID != first.InstanceID || repeated.PID != first.PID {
		t.Fatal("duplicate Runner", repeated, err)
	}
	if d.reset(true) == nil {
		t.Fatal("reset accepted an active shared service")
	}
	stop := func() {
		if err := runCoordinator(ctx, d.command(d.helper, "debug", "stop")); err != nil {
			t.Fatal(err)
		}
		if err := owners[len(owners)-1].Wait(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := cli.VerifyStopped(ctx); err != nil {
			t.Fatal("shared final proof", err)
		}
	}
	stop()
	pinPath := filepath.Join(d.home, "application-api", "runner-build.json")
	pinBefore, err := os.ReadFile(pinPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = d.reset(false); err != nil {
		t.Fatal(err)
	}
	pinAfter, err := os.ReadFile(pinPath)
	if err != nil || !bytes.Equal(pinBefore, pinAfter) {
		t.Fatal("reset changed build pin", err)
	}
	if _, err = os.Stat(cli.StateDir); err != nil {
		t.Fatal("reset deleted Runner history", err)
	}
	second := start()
	if first.InstanceID == second.InstanceID {
		t.Fatal("restart retained old Runner instance")
	}
	stop()
	if err = controlled.Write(filepath.Join(root, "evidence", "shared-runner-service.json"), map[string]any{"passed": true, "instances": observations, "reset_preserved_build_pin": true, "reset_preserved_runner_directory": true, "business_runs": 0, "cleanup": "confirmed"}); err != nil {
		t.Fatal(err)
	}
}
