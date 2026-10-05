package host

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	execution "github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestLifecycleAuditRejectedStartupPreservesOwnerFiles(t *testing.T) {
	for _, missingLog := range []bool{false, true} {
		t.Run(fmt.Sprint("missing-log=", missingLog), func(t *testing.T) {
			f := newFixture(t)
			c, _ := lifecycleConfigFile(t, f, f.c.RunDir)
			identity, err := resourceprocess.CurrentIdentity()
			require(t, err)
			record := reviewStartup(t, c, "started", identity)
			record.InstanceID = newID()
			require(t, writeStartupRecord(c, record))
			recordPath, err := startupPath(c.RunDir, record.StartupID)
			require(t, err)
			logPath, err := startupLogPath(c.RunDir, record.StartupID)
			require(t, err)
			originalLog := []byte("original service diagnostic\n")
			require(t, os.WriteFile(logPath, originalLog, 0600))
			if missingLog {
				require(t, os.Remove(logPath))
			}
			before, err := os.ReadFile(recordPath)
			require(t, err)
			if err := testRunConfiguredService(context.Background(), c, "manual", record.StartupID, "off"); err == nil {
				t.Fatal("a completed startup ID was reused")
			}
			after, err := os.ReadFile(recordPath)
			require(t, err)
			if !bytes.Equal(before, after) {
				t.Fatalf("rejected startup overwrote the owner's record:\n%s", after)
			}
			if !missingLog {
				afterLog, err := os.ReadFile(logPath)
				require(t, err)
				if !bytes.Equal(originalLog, afterLog) {
					t.Fatalf("rejected startup wrote to the owner's log:\n%s", afterLog)
				}
			}
		})
	}
}

func TestDirectServiceAndControlEntrypointsRejectMissingNativeAuthorityBeforeEffects(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "state")
	c := execution.Config{Repository: filepath.Join(t.TempDir(), "not-a-repository"), RunDir: runDir}
	assertAuthorityError := func(name string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "task_authority=native") {
			t.Fatalf("%s did not reject missing native authority: %v", name, err)
		}
	}
	assertAuthorityError("RunConfiguredService", RunConfiguredService(context.Background(), c, "manual", "", "off", newHostFixtureService, testHostAdmission{}, nil))
	_, stopErr := StopService(context.Background(), c, "operator", time.Second)
	assertAuthorityError("StopService", stopErr)
	if _, err := ControlSnapshot(c); err == nil || !strings.Contains(err.Error(), "task_authority=native") {
		t.Fatalf("ControlSnapshot did not reject missing native authority: %v", err)
	}
	if _, err := LoadDatabaseRuntime(c); err == nil || !strings.Contains(err.Error(), "task_authority=native") {
		t.Fatalf("LoadDatabaseRuntime did not reject missing native authority: %v", err)
	}
	if _, err := execution.InspectRecovery(context.Background(), c, "run"); err == nil || !strings.Contains(err.Error(), "task_authority=native") {
		t.Fatalf("InspectRecovery did not reject missing native authority: %v", err)
	}
	assertAuthorityError("VerifyTaskContractFiles", fullFixtureTaskService(nil, c).VerifyContractFiles(context.Background(), taskmodel.TaskContract{}))
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Fatalf("rejected direct entrypoint created service state: %v", err)
	}
}

func TestRunConfiguredServiceRejectsBeadsBeforeCreatingStartupEvidence(t *testing.T) {
	f := newFixture(t)
	f.c.WorkflowProtocol = executionmodel.Version
	sentinel := filepath.Join(f.c.Repository, ".beads")
	require(t, os.Mkdir(sentinel, 0700))
	err := testRunConfiguredService(context.Background(), f.c, "manual", "", "off")
	if err == nil || !strings.Contains(err.Error(), "Beads state exists") {
		t.Fatalf("service did not refuse old repository state before startup: %v", err)
	}
	if _, err := os.Stat(f.c.RunDir); !os.IsNotExist(err) {
		t.Fatalf("refused service startup created run state: %v", err)
	}
	if info, err := os.Stat(sentinel); err != nil || !info.IsDir() {
		t.Fatalf("refused service startup changed the old state sentinel: %v", err)
	}
}

func reviewStartup(t *testing.T, c execution.Config, status string, identity *executionbackend.ProcessIdentity) StartupRecord {
	t.Helper()
	id := newID()
	_, _, err := createStartupAttempt(c, id, true)
	require(t, err)
	record := initialStartupRecord(c, id)
	record.Status, record.ProcessIdentity = status, identity
	require(t, writeStartupRecord(c, record))
	return record
}

func TestLifecycleReviewMultipleLiveAttemptsRefuseLaunch(t *testing.T) {
	s := controlReviewService(t)
	id, err := resourceprocess.CurrentIdentity()
	require(t, err)
	reviewStartup(t, s.Config, "starting", id)
	reviewStartup(t, s.Config, "starting", id)
	_, _, _, err = findLiveStartupAttempt(s.Config)
	if err == nil {
		t.Fatal("ambiguous live attempts must stop the caller from launching a third process")
	}
}

func TestLifecycleReviewStartedProcessEarlyExit(t *testing.T) {
	s := controlReviewService(t)
	removeRuntimeProjection(t, s)
	id := &executionbackend.ProcessIdentity{Kind: "os", PID: 2_000_000_000, Created: "absent", Executable: "absent"}
	record := reviewStartup(t, s.Config, "started", id)
	result, err := waitForStartup(context.Background(), s.Config, record.StartupID, "", StartServiceOptions{WaitTimeout: 30 * time.Millisecond, PollInterval: time.Millisecond})
	require(t, err)
	if result.Status != "failed" {
		t.Fatalf("known exited startup reported %s", result.Status)
	}
}

func removeRuntimeProjection(t *testing.T, s *execution.Service) {
	t.Helper()
	_, err := fixtureStoreFor(t, s).db.Exec(`DELETE FROM projection WHERE project_identity=?`, s.Config.Identity())
	require(t, err)
}

func TestLifecycleReviewFailedRecordMustBindProcess(t *testing.T) {
	s := controlReviewService(t)
	record := reviewStartup(t, s.Config, "failed", nil)
	s.State.Service.StartupID = &record.StartupID
	require(t, publishFixtureCheckpoint(s))
	connection := ConnectService(context.Background(), s.Config)
	if connection.CanStop || connection.IdentityMatched {
		t.Fatalf("unbound failed record authorizes a process: %+v", connection)
	}
}

func TestLifecycleReviewExistingServiceDoesNotNeedInstalledBinary(t *testing.T) {
	f := newFixture(t)
	initialized := f.service(t)
	require(t, initialized.Close())
	c, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
	s, err := newTestService(c)
	require(t, err)
	t.Cleanup(func() { _ = s.Close() })
	identity, err := resourceprocess.CurrentIdentity()
	require(t, err)
	s.State.Service.Ready = true
	s.State.Service.Status = "running"
	startupID := newID()
	_, _, err = createStartupAttempt(s.Config, startupID, true)
	require(t, err)
	record := initialStartupRecord(s.Config, startupID)
	record.Status, record.InstanceID, record.ProcessIdentity = "started", s.State.Service.InstanceID, identity
	require(t, writeStartupRecord(s.Config, record))
	s.State.Service.StartupID, s.State.Service.PID, s.State.Service.ProcessIdentity = &startupID, identity.PID, identity
	require(t, publishFixtureCheckpoint(s))
	result, err := testStartService(context.Background(), configPath, StartServiceOptions{Executable: filepath.Join(t.TempDir(), "missing.exe")})
	if err != nil || result.Status != "connected" {
		t.Fatalf("existing compatible service needs no launch executable: %+v %v", result, err)
	}
}

func TestLifecycleReviewCancelledLaunchDoesNotWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := filepath.Join(t.TempDir(), "cancelled.lock")
	lock, err := acquireLaunchLock(ctx, path)
	if lock != nil {
		lock.Close()
	}
	if err == nil {
		t.Error("cancelled client acquired a launch lock")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("cancelled client created a lock file")
	}
}

func TestLifecycleReviewWaitTimeoutDiagnosticUsesActualDuration(t *testing.T) {
	s := controlReviewService(t)
	removeRuntimeProjection(t, s)
	id, err := resourceprocess.CurrentIdentity()
	require(t, err)
	record := reviewStartup(t, s.Config, "starting", id)
	result, err := waitForStartup(context.Background(), s.Config, record.StartupID, "", StartServiceOptions{WaitTimeout: 25 * time.Millisecond, PollInterval: time.Millisecond})
	require(t, err)
	if strings.Contains(result.Error, "30 seconds") {
		t.Fatal("custom wait timeout is reported as 30 seconds")
	}
}

func TestLifecycleReviewLauncherCannotRollbackChildHandshake(t *testing.T) {
	s := controlReviewService(t)
	id, err := resourceprocess.CurrentIdentity()
	require(t, err)
	record := reviewStartup(t, s.Config, "started", id)
	record.InstanceID = s.State.Service.InstanceID
	require(t, writeStartupRecord(s.Config, record))
	path, err := startupPath(s.Config.RunDir, record.StartupID)
	require(t, err)
	before, err := os.ReadFile(path)
	require(t, err)
	require(t, recordStartupProcess(s.Config, record.StartupID, *id))
	after, err := os.ReadFile(path)
	require(t, err)
	if string(before) != string(after) {
		t.Fatal("launcher modified child's completed handshake")
	}
}

func TestLifecycleReviewStopPublicationKeepsObservedInstance(t *testing.T) {
	s := controlReviewService(t)
	observed := s.State
	s.State.Service.InstanceID = newID()
	require(t, publishFixtureCheckpoint(s))
	req, err := submitControlSnapshot(s.Config, &observed, "stop_service", nil, "review")
	require(t, err)
	if req.InstanceID != observed.Service.InstanceID {
		t.Fatal("stop was retargeted to replacement service")
	}
}

func TestLifecycleReviewOfflineWithHeldLockCannotRestart(t *testing.T) {
	s := controlReviewService(t)
	absent := &executionbackend.ProcessIdentity{Kind: "os", PID: 2_000_000_000, Created: "absent", Executable: "absent"}
	s.State.Service.ProcessIdentity = absent
	s.State.Service.PID = s.State.Service.ProcessIdentity.PID
	startup, err := readStartupRecord(s.Config, *s.State.Service.StartupID)
	require(t, err)
	startup.ProcessIdentity = absent
	require(t, writeStartupRecord(s.Config, *startup))
	require(t, publishFixtureCheckpoint(s))
	lock, err := LockRepository(s.Config.RunDir)
	require(t, err)
	defer lock.Close()
	connection := ConnectService(context.Background(), s.Config)
	if connection.State != ConnectionOffline || connection.CanStart {
		t.Fatalf("restart enabled before lock release: %+v", connection)
	}
}

func TestLifecycleReviewCorruptStartupCannotBeIgnored(t *testing.T) {
	s := controlReviewService(t)
	id := newID()
	path, _, err := createStartupAttempt(s.Config, id, true)
	require(t, err)
	require(t, os.WriteFile(path, []byte("{"), 0600))
	if _, _, _, err := findLiveStartupAttempt(s.Config); err == nil {
		t.Fatal("corrupt startup evidence was ignored")
	}
}

func TestLifecycleReviewChildPublishesIdentityBeforePreflight(t *testing.T) {
	f := newFixture(t)
	c, _ := lifecycleConfigFile(t, f, f.c.RunDir)
	t.Setenv("STEERLINE_FAKE_PR_DELAY", "15s")
	record := reviewStartup(t, c, "starting", nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- testRunConfiguredService(ctx, c, "manual", record.StartupID, "off") }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		current, err := readStartupRecord(c, record.StartupID)
		if err == nil && current.ProcessIdentity != nil && current.ProcessIdentity.PID == os.Getpid() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("slow preflight left startup with no process identity")
}

func TestTM08Shutdown(t *testing.T) {
	s := controlReviewService(t)
	exe, err := os.Executable()
	require(t, err)
	cmd := exec.Command(exe, "--fake", "sleep")
	process, err := resourceprocess.StartManaged(cmd)
	require(t, err)
	defer process.Close()
	defer func() { process.Kill(); _ = cmd.Wait() }()
	identity := process.Identity()
	s.State.Service.PID, s.State.Service.ProcessIdentity = identity.PID, &identity
	startup, err := readStartupRecord(s.Config, *s.State.Service.StartupID)
	require(t, err)
	startup.ProcessIdentity = &identity
	require(t, writeStartupRecord(s.Config, *startup))
	require(t, publishFixtureCheckpoint(s))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan productapi.ServiceStopResult, 1)
	errors := make(chan error, 1)
	go func() {
		result, err := StopService(ctx, s.Config, "review", time.Second)
		done <- result
		errors <- err
	}()
	defer func() { cancel() }()
	deadline := time.Now().Add(4 * time.Second)
	for {
		entries, _ := os.ReadDir(filepath.Join(s.Config.RunDir, "control", "requests"))
		if len(entries) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stop request was not published")
		}
		time.Sleep(10 * time.Millisecond)
	}
	process.Kill()
	_ = cmd.Wait()
	result := <-done
	require(t, <-errors)
	if result.Status != "pending" || !result.ProcessExited || result.Receipt != nil || result.RequestID == "" || result.InstanceID != s.State.Service.InstanceID {
		t.Fatalf("unconfirmed stop lost request identity or exit evidence: %+v", result)
	}
}
