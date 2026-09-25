package steerline

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
)

func TestLifecycleAuditRejectedStartupPreservesOwnerFiles(t *testing.T) {
	for _, missingLog := range []bool{false, true} {
		t.Run(fmt.Sprint("missing-log=", missingLog), func(t *testing.T) {
			f := newFixture(t)
			c, _ := lifecycleConfigFile(t, f, f.c.RunDir)
			identity, err := currentProcessIdentity()
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
			if err := RunConfiguredService(context.Background(), c, "manual", record.StartupID, "off"); err == nil {
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

func reviewStartup(t *testing.T, c Config, status string, identity *ProcessIdentity) StartupRecord {
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
	id, err := currentProcessIdentity()
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
	require(t, os.Remove(s.store.Path))
	id := &ProcessIdentity{Kind: "os", PID: 2_000_000_000, Created: "absent", Executable: "absent"}
	record := reviewStartup(t, s.Config, "started", id)
	result, err := waitForStartup(context.Background(), s.Config, record.StartupID, "", StartServiceOptions{WaitTimeout: 30 * time.Millisecond, PollInterval: time.Millisecond})
	require(t, err)
	if result.Status != "failed" {
		t.Fatalf("known exited startup reported %s", result.Status)
	}
}

func TestLifecycleReviewFailedRecordMustBindProcess(t *testing.T) {
	s := controlReviewService(t)
	record := reviewStartup(t, s.Config, "failed", nil)
	s.State.Service.StartupID = &record.StartupID
	require(t, s.save())
	connection := ConnectService(context.Background(), s.Config)
	if connection.CanStop || connection.IdentityMatched {
		t.Fatalf("unbound failed record authorizes a process: %+v", connection)
	}
}

func TestLifecycleReviewExistingServiceDoesNotNeedInstalledBinary(t *testing.T) {
	f := newFixture(t)
	c, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
	s, err := NewService(c)
	require(t, err)
	t.Cleanup(func() { _ = s.Close() })
	s.State.Service.Ready = true
	s.State.Service.Status = "running"
	require(t, s.save())
	result, err := StartService(context.Background(), configPath, StartServiceOptions{Executable: filepath.Join(t.TempDir(), "missing.exe")})
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
	require(t, os.Remove(s.store.Path))
	id, err := currentProcessIdentity()
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
	id, err := currentProcessIdentity()
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
	require(t, s.save())
	req, err := submitControlSnapshot(s.Config, &observed, "stop_service", nil, "review")
	require(t, err)
	if req.InstanceID != observed.Service.InstanceID {
		t.Fatal("stop was retargeted to replacement service")
	}
}

func TestLifecycleReviewOfflineWithHeldLockCannotRestart(t *testing.T) {
	s := controlReviewService(t)
	s.State.Service.ProcessIdentity = &ProcessIdentity{Kind: "os", PID: 2_000_000_000, Created: "absent", Executable: "absent"}
	s.State.Service.PID = s.State.Service.ProcessIdentity.PID
	require(t, s.save())
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
	db := readFake(f.db)
	db.Behavior = "slow-context"
	writeFake(f.db, db)
	record := reviewStartup(t, c, "starting", nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- RunConfiguredService(ctx, c, "manual", record.StartupID, "off") }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, err := readStartupRecord(c, record.StartupID)
		if err == nil && current.ProcessIdentity != nil && current.ProcessIdentity.PID == os.Getpid() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("slow preflight left startup with no process identity")
}

func TestLifecycleReviewStopReportsExitWithoutReceipt(t *testing.T) {
	s := controlReviewService(t)
	exe, err := os.Executable()
	require(t, err)
	cmd := exec.Command(exe, "--fake", "sleep")
	process, err := startProcess(cmd)
	require(t, err)
	defer process.Close()
	defer func() { process.Kill(); _ = cmd.Wait() }()
	identity := process.Identity()
	s.State.Service.PID, s.State.Service.ProcessIdentity = identity.PID, &identity
	require(t, s.save())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan ServiceStopResult, 1)
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
