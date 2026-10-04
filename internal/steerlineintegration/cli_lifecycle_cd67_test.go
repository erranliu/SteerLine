package steerlineintegration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerlineapp"
)

func TestStartServiceSerializesSameConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows service-window uniqueness is verified through service locks")
	}
	if !cliDetachedChildSupported(t) {
		t.Skip("this host keeps child processes in a Job Object and rejects breakaway")
	}
	f := newCLILifecycleFixture(t)
	config, configPath := f.c, f.path
	serviceExe := buildProductLifecycleCLI(t)
	t.Cleanup(func() {
		connection := steerline.ConnectService(context.Background(), config)
		if connection.CanStop {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_, _ = steerline.StopService(ctx, config, "test-cleanup", 12*time.Second)
		}
	})
	opts := steerline.StartServiceOptions{Executable: serviceExe, WaitTimeout: 20 * time.Second, PollInterval: 30 * time.Millisecond}
	type outcome struct {
		result steerline.ServiceStartResult
		err    error
	}
	starts := make(chan outcome, 2)
	for range 2 {
		go func() {
			result, err := steerlineapp.StartService(context.Background(), configPath, opts)
			starts <- outcome{result, err}
		}()
	}
	first, second := <-starts, <-starts
	if first.err != nil || second.err != nil || first.result.Status != "connected" || second.result.Status != "connected" {
		t.Fatalf("concurrent starts: first=%+v/%v second=%+v/%v", first.result, first.err, second.result, second.err)
	}
	firstID := first.result.Connection.Snapshot.Runtime.Service.InstanceID
	secondID := second.result.Connection.Snapshot.Runtime.Service.InstanceID
	if firstID == "" || firstID != secondID {
		t.Fatalf("same config started different instances: %q, %q", firstID, secondID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	stopped, err := steerline.StopService(ctx, config, "lifecycle-test", 12*time.Second)
	if err != nil || stopped.Status != "stopped" {
		t.Fatalf("stop concurrent winner: %+v %v", stopped, err)
	}
}

func TestStartServiceReportsPreflightFailureAndPendingHandshake(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows service-window diagnostics use foreground startup records")
	}
	if !cliDetachedChildSupported(t) {
		t.Skip("this Windows host keeps child processes in a Job Object and rejects breakaway")
	}
	t.Run("preflight failure is logged", func(t *testing.T) {
		f := newCLILifecycleFixture(t)
		f.c.PR.Repository = "wrong/repository"
		f.save(t)
		config, configPath := f.c, f.path
		result, err := steerlineapp.StartService(context.Background(), configPath, steerline.StartServiceOptions{Executable: buildProductLifecycleCLI(t), WaitTimeout: 15 * time.Second})
		if err != nil || result.Status != "failed" || !strings.Contains(result.Error, "does not match Git remote") {
			t.Fatalf("preflight result=%+v err=%v", result, err)
		}
		if result.StartupID == "" || result.LogPath == "" {
			t.Fatalf("preflight failure lacks startup identity/log: %+v", result)
		}
		record, readErr := readCLIStartupRecord(config, result.StartupID)
		if readErr != nil || record.Status != "failed" || !strings.Contains(record.Error, "does not match Git remote") {
			t.Fatalf("preflight startup record=%+v err=%v", record, readErr)
		}
		log, readErr := os.ReadFile(result.LogPath)
		require(t, readErr)
		if !strings.Contains(string(log), "does not match Git remote") {
			t.Fatalf("preflight diagnostic missing from log: %s", log)
		}
	})

	t.Run("timeout keeps startup identity and does not repeat launch", func(t *testing.T) {
		var pendingIdentity *executionbackend.ProcessIdentity
		t.Cleanup(func() {
			if pendingIdentity != nil {
				_ = terminateCLIProcess(pendingIdentity)
			}
		})
		f := newCLILifecycleFixture(t)
		t.Setenv("STEERLINE_CLI_PREFLIGHT_DELAY", "2s")
		config, configPath := f.c, f.path
		serviceExe := buildProductLifecycleCLI(t)
		result, err := steerlineapp.StartService(context.Background(), configPath, steerline.StartServiceOptions{Executable: serviceExe, WaitTimeout: 350 * time.Millisecond, PollInterval: 25 * time.Millisecond})
		if err != nil || result.Status != "pending" || result.StartupID == "" {
			t.Fatalf("pending startup result=%+v err=%v", result, err)
		}
		again, err := steerlineapp.StartService(context.Background(), configPath, steerline.StartServiceOptions{Executable: serviceExe, WaitTimeout: 350 * time.Millisecond, PollInterval: 25 * time.Millisecond})
		if err != nil || again.Status != "pending" || again.StartupID != result.StartupID {
			t.Fatalf("repeat observation changed startup attempt: first=%+v second=%+v err=%v", result, again, err)
		}
		record, err := readCLIStartupRecord(config, result.StartupID)
		require(t, err)
		pendingIdentity = record.ProcessIdentity
		if record.Status != "starting" || record.ProcessIdentity == nil || steerlineapp.NewProcessHost(time.Second).InspectProcess(record.ProcessIdentity) != "matching process still exists" {
			t.Fatalf("pending startup lost process identity: %+v", record)
		}
		if got := steerline.ReadObservationSnapshot(config, nil); got.State != steerline.SnapshotNone {
			t.Fatalf("preflight delay unexpectedly published a runtime: %+v", got)
		}
		if err := terminateCLIProcess(record.ProcessIdentity); err != nil {
			t.Fatal(err)
		}
		pendingIdentity = nil
		observed, err := observeCLIStartupExit(config, result.StartupID, 3*time.Second)
		if err != nil || observed.CanControl || observed.State != steerline.ConnectionOffline || observed.Process.State != steerline.ProcessAbsent {
			t.Fatalf("killed startup was not observed as offline: %+v %v", observed, err)
		}
	})

	t.Run("early process exit is not reported as online", func(t *testing.T) {
		t.Setenv("STEERLINE_LIFECYCLE_EXIT_EARLY", "1")
		f := newCLILifecycleFixture(t)
		config, configPath := f.c, f.path
		testExe, err := os.Executable()
		require(t, err)
		result, err := steerlineapp.StartService(context.Background(), configPath, steerline.StartServiceOptions{Executable: testExe, WaitTimeout: 2 * time.Second, PollInterval: 30 * time.Millisecond})
		if err != nil || result.Status != "failed" || result.StartupID == "" {
			t.Fatalf("early-exit result=%+v err=%v", result, err)
		}
		if result.Connection.State == steerline.ConnectionConnected || result.Connection.CanControl {
			t.Fatalf("early exit was reported as connected: %+v", result)
		}
		if record, e := readCLIStartupRecord(config, result.StartupID); e != nil || record.Status != "failed" {
			t.Fatalf("early exit startup record=%+v err=%v", record, e)
		}
	})
}

func TestRunCommandRejectsLegacyTaskAuthorityBeforeWritingState(t *testing.T) {
	fixture := newLifecycleProjectBase(t, "local", false)
	f := &cliLifecycleFixture{c: fixture.config, path: fixture.configPath}
	config := f.c
	config.TaskAuthority = "beads"
	data, err := json.Marshal(config)
	require(t, err)
	configPath := filepath.Join(t.TempDir(), "legacy-config.json")
	require(t, os.WriteFile(configPath, data, 0600))
	output, runErr := exec.Command(buildProductLifecycleCLI(t), "run", "-config", configPath, "-dispatch", "manual", "-console", "off").CombinedOutput()
	if runErr == nil || !strings.Contains(string(output), "task_authority") {
		t.Fatalf("legacy task authority was accepted: err=%v output=%s", runErr, output)
	}
	if _, err := os.Stat(filepath.Join(f.c.RunDir, "steerline.db")); !os.IsNotExist(err) {
		t.Fatalf("rejected legacy config initialized a native database: %v", err)
	}
}

type cliLifecycleFixture struct {
	c    execution.Config
	path string
}

func newCLILifecycleFixture(t *testing.T) *cliLifecycleFixture {
	t.Helper()
	f := newLifecycleProject(t, "local")
	return &cliLifecycleFixture{c: f.config, path: f.configPath}
}

func (f *cliLifecycleFixture) save(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(f.path)
	require(t, err)
	var fields map[string]json.RawMessage
	require(t, json.Unmarshal(raw, &fields))
	fields["pr"] = mustJSON(f.c.PR)
	require(t, os.WriteFile(f.path, mustJSON(fields), 0600))
}

var cliBuildOnce sync.Once

var cliBuildPath string

var cliBuildErr error

func buildProductLifecycleCLI(t *testing.T) string {
	t.Helper()
	cliBuildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "sl-product-cli-")
		if err != nil {
			cliBuildErr = err
			return
		}
		cliBuildPath = filepath.Join(dir, "steerline")
		if runtime.GOOS == "windows" {
			cliBuildPath += ".exe"
		}
		out, err := exec.Command("go", "build", "-o", cliBuildPath, "../../cmd/steerline").CombinedOutput()
		if err != nil {
			cliBuildErr = fmt.Errorf("%w: %s", err, out)
		}
	})
	require(t, cliBuildErr)
	return cliBuildPath
}

func cliDetachedChildSupported(t *testing.T) bool { t.Helper(); return runtime.GOOS != "windows" }

func readCLIStartupRecord(c execution.Config, id string) (steerline.StartupRecord, error) {
	var r steerline.StartupRecord
	raw, err := os.ReadFile(filepath.Join(c.RunDir, "service", "startups", id, "startup.json"))
	if err == nil {
		err = json.Unmarshal(raw, &r)
	}
	return r, err
}

func terminateCLIProcess(identity *executionbackend.ProcessIdentity) error {
	if identity == nil || steerlineapp.NewProcessHost(time.Second).InspectProcess(identity) != "matching process still exists" {
		return fmt.Errorf("original test process identity differs")
	}
	p, err := os.FindProcess(identity.PID)
	if err != nil {
		return err
	}
	return p.Kill()
}

func observeCLIStartupExit(c execution.Config, id string, budget time.Duration) (steerline.ServiceConnection, error) {
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		connection := steerline.ConnectService(context.Background(), c)
		if connection.StartupID == id && connection.Process.State == steerline.ProcessAbsent {
			return connection, nil
		}
		time.Sleep(30 * time.Millisecond)
	}
	return steerline.ServiceConnection{}, fmt.Errorf("original pending process exit was not proven")
}
