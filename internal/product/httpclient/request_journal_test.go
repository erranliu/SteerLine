package httpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
)

const journalHelperEnv = "STEERLINE_HTTP_JOURNAL_HELPER"

func TestHTTPRequestJournal(t *testing.T) {
	if os.Getenv(journalHelperEnv) != "" {
		runHTTPRequestJournalHelper(t)
		return
	}
	t.Run("two processes serialize first registration", testJournalFirstRegistrationProcesses)
	t.Run("independent instances in one process contend on the OS lock", testJournalSameProcessInstances)
	t.Run("two processes preserve terminal responses across stale writes", testJournalProcessInterleavings)
	t.Run("same terminal response keeps the original complete receipt", testJournalSameTerminalResponse)
	t.Run("opposite terminal and original identity conflicts preserve the record", testJournalConflictPreservesRecord)
	t.Run("save failure never posts", testJournalSaveFailurePreventsPost)
	t.Run("lock wait cancels and process exit releases the lock", testJournalCancelAndProcessExit)
}

func journalFixture(t *testing.T, home string) (*requestJournal, wire.CommandEnvelope) {
	t.Helper()
	if home == "" {
		home = t.TempDir()
	}
	ref := api.ProjectRef{Identity: "journal-test-repository", ApplicationHome: home}
	envelope := wire.CommandEnvelope{
		RequestID: strings.Repeat("a", 32), ProjectID: wire.ProjectID(ref.Identity), InstanceID: "original-instance",
		Action: "pause_dispatch", TargetID: "dispatch", ExpectedVersion: 7,
		ExpectedControlRevision: 7, Payload: json.RawMessage("{}"),
	}
	return newRequestJournal(ref), envelope
}

func journalResponse(envelope wire.CommandEnvelope, status, code, diagnostic string, sampled time.Time, includeReceipt bool) wire.CommandResponse {
	response := wire.CommandResponse{
		RequestID: envelope.RequestID, ProjectID: envelope.ProjectID, InstanceID: envelope.InstanceID,
		Action: envelope.Action, TargetID: envelope.TargetID, ExpectedVersion: envelope.ExpectedVersion,
		Original: &envelope, Status: status, Code: code, Diagnostic: diagnostic, SampledAt: sampled,
	}
	if includeReceipt {
		response.ControlRequest = &api.ControlRequest{RequestID: envelope.RequestID, InstanceID: envelope.InstanceID, Action: envelope.Action}
		response.ControlReceipt = &api.ControlReceipt{RequestID: envelope.RequestID, InstanceID: envelope.InstanceID, Action: envelope.Action, Status: status, Code: code, Message: diagnostic}
	}
	return response
}

func testJournalFirstRegistrationProcesses(t *testing.T) {
	journal, envelope := journalFixture(t, "")
	root := t.TempDir()
	first := startJournalHelper(t, root, "register", journal.home, envelope.RequestID, "first")
	second := startJournalHelper(t, root, "register", journal.home, envelope.RequestID, "second")
	waitJournalSignal(t, first.ready)
	waitJournalSignal(t, second.ready)
	writeJournalSignal(t, first.gate)
	writeJournalSignal(t, second.gate)
	waitJournalHelper(t, first)
	waitJournalHelper(t, second)
	got, err := journal.read(envelope.RequestID)
	if err != nil || !reflect.DeepEqual(got.Envelope, envelope) || got.Response != nil {
		t.Fatalf("first request registration: %+v %v", got, err)
	}
	lockPath, err := journal.lockPath(envelope.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(lockPath); err != nil {
		t.Fatalf("stable request lock file was not retained: %v", err)
	}
}

func testJournalSameProcessInstances(t *testing.T) {
	first, envelope := journalFixture(t, "")
	second := newRequestJournal(api.ProjectRef{Identity: first.identity, ApplicationHome: first.home})
	lockPath, err := first.lockPath(envelope.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	locked := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- withRequestJournalLock(context.Background(), lockPath, func() error {
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	contended := make(chan struct{})
	acquired := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondLockPath, lockErr := second.lockPath(envelope.RequestID)
		if lockErr != nil {
			secondDone <- lockErr
			return
		}
		secondDone <- withRequestJournalLockObserved(context.Background(), secondLockPath, func() {
			select {
			case <-contended:
			default:
				close(contended)
			}
		}, func() error {
			close(acquired)
			return nil
		})
	}()
	select {
	case <-contended:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("second journal instance did not see the held OS lock")
	}
	select {
	case <-acquired:
		t.Fatal("second journal instance acquired a lock held by the first")
	default:
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-acquired:
	default:
		t.Fatal("second journal instance did not acquire after release")
	}
}

func testJournalProcessInterleavings(t *testing.T) {
	for _, test := range []struct {
		name          string
		terminalFirst bool
		terminal      string
	}{
		{name: "applied before stale submitted", terminalFirst: true, terminal: "applied"},
		{name: "stale submitted before applied", terminal: "applied"},
		{name: "rejected before stale submitted", terminalFirst: true, terminal: "rejected"},
		{name: "stale submitted before rejected", terminal: "rejected"},
	} {
		t.Run(test.name, func(t *testing.T) {
			journal, envelope := journalFixture(t, "")
			if _, err := journal.save(context.Background(), outgoing{Version: 1, Envelope: envelope}); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			var terminal, stale *journalHelperProcess
			if test.terminalFirst {
				terminal = startJournalHelper(t, root, "locked-write", journal.home, envelope.RequestID, "terminal", test.terminal)
				waitJournalSignal(t, terminal.ready)
				stale = startJournalHelper(t, root, "wait-write", journal.home, envelope.RequestID, "stale")
				waitJournalSignal(t, stale.waiting)
				writeJournalSignal(t, terminal.gate)
				waitJournalHelper(t, terminal)
				waitJournalHelper(t, stale)
			} else {
				terminal = startJournalHelper(t, root, "write", journal.home, envelope.RequestID, "terminal", test.terminal)
				stale = startJournalHelper(t, root, "write", journal.home, envelope.RequestID, "stale")
				waitJournalSignal(t, terminal.ready)
				waitJournalSignal(t, stale.ready)
				writeJournalSignal(t, stale.gate)
				waitJournalHelper(t, stale)
				writeJournalSignal(t, terminal.gate)
				waitJournalHelper(t, terminal)
			}
			got, err := journal.read(envelope.RequestID)
			if err != nil || got.Response == nil || got.Response.Status != test.terminal {
				t.Fatalf("terminal was lost: %+v %v", got, err)
			}
			staleResult := readJournalHelperResult(t, stale.result)
			if test.terminalFirst && (staleResult.Response == nil || staleResult.Response.Status != test.terminal) {
				t.Fatalf("stale process did not receive retained terminal result: %+v", staleResult)
			}
			if !test.terminalFirst && staleResult.Response != nil {
				t.Fatalf("stale-first process unexpectedly observed a future terminal result: %+v", staleResult)
			}
		})
	}
}

func testJournalSameTerminalResponse(t *testing.T) {
	for _, status := range []string{"applied", "rejected"} {
		t.Run(status, func(t *testing.T) {
			journal, envelope := journalFixture(t, "")
			if _, err := journal.save(context.Background(), outgoing{Version: 1, Envelope: envelope}); err != nil {
				t.Fatal(err)
			}
			first := journalResponse(envelope, status, "original-code", "original diagnostic", time.Date(2026, 10, 14, 1, 2, 3, 4, time.UTC), true)
			stored, err := journal.save(context.Background(), outgoing{Version: 1, Envelope: envelope, Response: &first})
			if err != nil {
				t.Fatal(err)
			}
			later := journalResponse(envelope, status, "later-code", "different GET diagnostic", time.Date(2026, 10, 14, 2, 3, 4, 5, time.UTC), false)
			retained, err := journal.save(context.Background(), outgoing{Version: 1, Envelope: envelope, Response: &later})
			if err != nil {
				t.Fatal(err)
			}
			if retained.Response == nil || !reflect.DeepEqual(*retained.Response, first) {
				t.Fatalf("same terminal replaced the original full receipt: %+v", retained.Response)
			}
			onDisk, err := journal.read(envelope.RequestID)
			if err != nil || onDisk.Response == nil || !reflect.DeepEqual(*onDisk.Response, first) {
				t.Fatalf("original terminal response changed on disk: %+v %v", onDisk, err)
			}
			if stored.Response == nil || !reflect.DeepEqual(*stored.Response, first) {
				t.Fatal("initial terminal receipt was not retained")
			}
		})
	}
}

func testJournalConflictPreservesRecord(t *testing.T) {
	t.Run("opposite terminal", func(t *testing.T) {
		journal, envelope := journalFixture(t, "")
		first := journalResponse(envelope, "applied", "applied", "first", time.Now().UTC(), true)
		if _, err := journal.save(context.Background(), outgoing{Version: 1, Envelope: envelope, Response: &first}); err != nil {
			t.Fatal(err)
		}
		other := journalResponse(envelope, "rejected", "stale_target", "opposite", time.Now().UTC(), false)
		retained, err := journal.save(context.Background(), outgoing{Version: 1, Envelope: envelope, Response: &other})
		if err == nil || retained.Response == nil || !reflect.DeepEqual(*retained.Response, first) {
			t.Fatalf("opposite terminal was not rejected with the retained receipt: %+v %v", retained, err)
		}
		onDisk, readErr := journal.read(envelope.RequestID)
		if readErr != nil || onDisk.Response == nil || !reflect.DeepEqual(*onDisk.Response, first) {
			t.Fatalf("opposite terminal replaced saved outcome: %+v %v", onDisk, readErr)
		}
	})
	t.Run("target, version and stop identity", func(t *testing.T) {
		for _, change := range []string{"target", "version", "stop_identity"} {
			t.Run(change, func(t *testing.T) {
				journal, envelope := journalFixture(t, "")
				base := outgoing{Version: 1, Envelope: envelope}
				if change == "stop_identity" {
					base.Envelope.Action = "stop_service"
					base.Envelope.TargetID = base.Envelope.InstanceID
					identity := wire.Identity{ProjectID: base.Envelope.ProjectID, RepositoryIdentity: journal.identity, InstanceID: base.Envelope.InstanceID, Process: wire.ProcessIdentity{Kind: "os", PID: 22, Created: "generation-a"}}
					base.StopIdentity = &identity
				}
				if _, err := journal.save(context.Background(), base); err != nil {
					t.Fatal(err)
				}
				changed := base
				switch change {
				case "target":
					changed.Envelope.TargetID = "different-target"
				case "version":
					changed.Envelope.ExpectedVersion++
				case "stop_identity":
					copy := *base.StopIdentity
					copy.Process.Created = "generation-b"
					changed.StopIdentity = &copy
				}
				if _, err := journal.save(context.Background(), changed); err == nil {
					t.Fatal("changed original identity was accepted")
				}
				got, err := journal.read(envelope.RequestID)
				if err != nil || !reflect.DeepEqual(got, base) {
					t.Fatalf("conflict changed original record: %+v %v", got, err)
				}
			})
		}
	})
}

func testJournalSaveFailurePreventsPost(t *testing.T) {
	ref := api.ProjectRef{Identity: "journal-test-repository", ApplicationHome: t.TempDir()}
	identity := wire.Identity{ProjectID: wire.ProjectID(ref.Identity), RepositoryIdentity: ref.Identity, InstanceID: "original-instance"}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(wire.ErrorResponse{Code: "command_not_found"})
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL, project: identity.ProjectID, identity: identity, http: server.Client()}
	c := newTestConsole(ref, client, nil)
	lockRoot := filepath.Join(ref.ApplicationHome, "ui", "http-request-locks")
	if err := os.MkdirAll(filepath.Dir(lockRoot), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockRoot, []byte("block lock directory creation"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := c.SubmitControl(context.Background(), api.ControlInput{Action: api.PauseDispatch, RequestID: strings.Repeat("b", 32), InstanceID: identity.InstanceID})
	if err == nil || requests != 0 {
		t.Fatalf("save failure allowed HTTP work: err=%v requests=%d", err, requests)
	}
}

func testJournalCancelAndProcessExit(t *testing.T) {
	journal, envelope := journalFixture(t, "")
	root := t.TempDir()
	holder := startJournalHelper(t, root, "hold", journal.home, envelope.RequestID, "holder")
	waitJournalSignal(t, holder.ready)
	lockPath, err := journal.lockPath(envelope.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	probe, err := openPrivateRequestLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	_, acquired, err := tryRequestJournalLock(probe)
	_ = probe.Close()
	if err != nil || acquired {
		t.Fatalf("independent process did not hold the kernel lock: acquired=%t err=%v", acquired, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	contended := make(chan struct{})
	var once sync.Once
	waitResult := make(chan error, 1)
	go func() {
		waitResult <- withRequestJournalLockObserved(ctx, lockPath, func() { once.Do(func() { close(contended) }) }, func() error { return nil })
	}()
	select {
	case <-contended:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("lock waiter did not report kernel contention")
	}
	cancel()
	if err := <-waitResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("lock wait cancellation: %v", err)
	}
	waiter := startJournalHelper(t, root, "wait", journal.home, envelope.RequestID, "waiter")
	waitJournalSignal(t, waiter.waiting)
	if err := holder.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = holder.cmd.Wait()
	waitJournalSignal(t, waiter.ready)
	writeJournalSignal(t, waiter.gate)
	waitJournalHelper(t, waiter)
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file was deleted after release: %v", err)
	}
	var reacquired bool
	if err := withRequestJournalLock(context.Background(), lockPath, func() error { reacquired = true; return nil }); err != nil || !reacquired {
		t.Fatalf("released lock could not be reacquired: %v", err)
	}
}

type journalHelperProcess struct {
	cmd                  *exec.Cmd
	ready, gate, waiting string
	result               string
}

func startJournalHelper(t *testing.T, root, operation, home, id, label string, status ...string) *journalHelperProcess {
	t.Helper()
	paths := journalHelperProcess{ready: filepath.Join(root, label+".ready"), gate: filepath.Join(root, label+".go"), waiting: filepath.Join(root, label+".waiting"), result: filepath.Join(root, label+".json")}
	cmd := exec.Command(os.Args[0], "-test.run=^TestHTTPRequestJournal$")
	values := []string{
		journalHelperEnv + "=" + operation,
		"STEERLINE_HTTP_JOURNAL_HOME=" + home,
		"STEERLINE_HTTP_JOURNAL_ID=" + id,
		"STEERLINE_HTTP_JOURNAL_READY=" + paths.ready,
		"STEERLINE_HTTP_JOURNAL_GATE=" + paths.gate,
		"STEERLINE_HTTP_JOURNAL_WAITING=" + paths.waiting,
		"STEERLINE_HTTP_JOURNAL_RESULT=" + paths.result,
	}
	if len(status) > 0 {
		values = append(values, "STEERLINE_HTTP_JOURNAL_STATUS="+status[0])
	}
	cmd.Env = append(os.Environ(), values...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	paths.cmd = cmd
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	return &paths
}

func waitJournalHelper(t *testing.T, helper *journalHelperProcess) {
	t.Helper()
	waitJournalSignal(t, helper.result)
	if err := helper.cmd.Wait(); err != nil {
		t.Fatalf("journal helper failed: %v", err)
	}
}

func waitJournalSignal(t *testing.T, path string) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !os.IsNotExist(err) {
			t.Fatalf("wait for synchronization marker %s: %v", path, err)
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for synchronization marker %s", path)
		case <-ticker.C:
		}
	}
}

func writeJournalSignal(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("go"), 0600); err != nil {
		t.Fatal(err)
	}
}

func readJournalHelperResult(t *testing.T, path string) outgoing {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result outgoing
	if err = json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func runHTTPRequestJournalHelper(t *testing.T) {
	get := func(key string) string { return os.Getenv("STEERLINE_HTTP_JOURNAL_" + key) }
	journal, envelope := journalFixture(t, get("HOME"))
	id := get("ID")
	envelope.RequestID = id
	operation := os.Getenv(journalHelperEnv)
	switch operation {
	case "register":
		if err := waitAndSignalJournalHelper(get("READY"), get("GATE")); err != nil {
			t.Fatal(err)
		}
		retained, err := journal.save(context.Background(), outgoing{Version: 1, Envelope: envelope})
		if err != nil {
			t.Fatal(err)
		}
		writeJournalHelperResult(t, get("RESULT"), retained)
	case "write":
		previous, err := journal.read(id)
		if err != nil {
			t.Fatal(err)
		}
		if get("STATUS") != "" {
			response := journalResponse(previous.Envelope, get("STATUS"), get("STATUS"), "helper response", time.Date(2026, 10, 14, 3, 4, 5, 6, time.UTC), true)
			previous.Response = &response
		}
		if err = waitAndSignalJournalHelper(get("READY"), get("GATE")); err != nil {
			t.Fatal(err)
		}
		retained, err := journal.save(context.Background(), previous)
		if err != nil {
			t.Fatal(err)
		}
		writeJournalHelperResult(t, get("RESULT"), retained)
	case "locked-write":
		previous, err := journal.read(id)
		if err != nil {
			t.Fatal(err)
		}
		response := journalResponse(previous.Envelope, get("STATUS"), get("STATUS"), "helper response", time.Date(2026, 10, 14, 3, 4, 5, 6, time.UTC), true)
		previous.Response = &response
		path, err := journal.lockPath(id)
		if err != nil {
			t.Fatal(err)
		}
		err = withRequestJournalLockObserved(context.Background(), path, nil, func() error {
			if err := writeJournalHelperSignal(get("READY")); err != nil {
				return err
			}
			if err := waitJournalHelperFile(get("GATE")); err != nil {
				return err
			}
			retained, err := journal.saveLocked(context.Background(), previous)
			if err != nil {
				return err
			}
			writeJournalHelperResult(t, get("RESULT"), retained)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	case "wait-write":
		previous, err := journal.read(id)
		if err != nil {
			t.Fatal(err)
		}
		path, err := journal.lockPath(id)
		if err != nil {
			t.Fatal(err)
		}
		var once sync.Once
		err = withRequestJournalLockObserved(context.Background(), path, func() {
			once.Do(func() { _ = writeJournalHelperSignal(get("WAITING")) })
		}, func() error {
			retained, err := journal.saveLocked(context.Background(), previous)
			if err != nil {
				return err
			}
			writeJournalHelperResult(t, get("RESULT"), retained)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	case "hold":
		path, err := journal.lockPath(id)
		if err != nil {
			t.Fatal(err)
		}
		err = withRequestJournalLockObserved(context.Background(), path, nil, func() error {
			if err := writeJournalHelperSignal(get("READY")); err != nil {
				return err
			}
			return waitJournalHelperFile(get("GATE"))
		})
		if err != nil {
			t.Fatal(err)
		}
	case "wait":
		path, err := journal.lockPath(id)
		if err != nil {
			t.Fatal(err)
		}
		var once sync.Once
		err = withRequestJournalLockObserved(context.Background(), path, func() {
			once.Do(func() { _ = writeJournalHelperSignal(get("WAITING")) })
		}, func() error {
			if err := writeJournalHelperSignal(get("READY")); err != nil {
				return err
			}
			return waitJournalHelperFile(get("GATE"))
		})
		if err != nil {
			t.Fatal(err)
		}
		writeJournalHelperResult(t, get("RESULT"), outgoing{Version: 1, Envelope: envelope})
	default:
		t.Fatalf("unknown HTTP journal helper operation %q", operation)
	}
}

func waitAndSignalJournalHelper(ready, gate string) error {
	if err := writeJournalHelperSignal(ready); err != nil {
		return err
	}
	return waitJournalHelperFile(gate)
}

func writeJournalHelperSignal(path string) error { return os.WriteFile(path, []byte("ready"), 0600) }

func waitJournalHelperFile(path string) error {
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		select {
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for journal helper gate %s", path)
		case <-ticker.C:
		}
	}
}

func writeJournalHelperResult(t *testing.T, path string, value outgoing) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
