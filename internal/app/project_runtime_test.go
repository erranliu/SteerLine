package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/app/host"
	api "github.com/erranliu/SteerLine/internal/product/api"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpapi"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

type lifecycleLog struct {
	closed int
	err    error
}

func (*lifecycleLog) Write(p []byte) (int, error) { return len(p), nil }
func (l *lifecycleLog) Close() error              { l.closed++; return l.err }

func TestProjectRuntimeClosesLocalResourcesDespiteRunnerUncertainty(t *testing.T) {
	for _, localFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "remote_unknown", true: "local_close_failed"}[localFailure], func(t *testing.T) {
			repo, run := t.TempDir(), t.TempDir()
			repoLock, err := host.LockRepository(repo)
			require(t, err)
			runLock, err := host.LockRepository(run)
			require(t, err)
			// Failed local closure intentionally retains locks; release them after assertions.
			defer repoLock.Close()
			defer runLock.Close()
			store, err := sqlite.OpenSQLiteStore(filepath.Join(run, "steerline.db"))
			require(t, err)
			log := &lifecycleLog{}
			if localFailure {
				log.err = errors.New("log close failed")
			}
			stops := 0
			r := &ProjectRuntime{repoLock: repoLock, runLock: runLock, store: store, log: log, runner: &applicationRunner{attempted: true, stop: func(context.Context) error { stops++; return context.DeadlineExceeded }}}
			r.close()
			r.close()
			if (r.localErr != nil) != localFailure || r.runner.ResourceSettlementError() == nil || stops != 0 || log.closed != 1 {
				t.Fatal("local and Runner outcomes or ownership conflated", r.localErr, stops, log.closed)
			}
			if _, err := store.ReadRunnerSnapshot(context.Background(), repo); err == nil {
				t.Fatal("owned store left open")
			}
			for _, key := range []string{repo, run} {
				lock, err := host.LockRepository(key)
				if localFailure {
					if err == nil {
						lock.Close()
						t.Fatal("local failure released ownership")
					}
				} else {
					require(t, err)
					lock.Close()
				}
			}
		})
	}
}

func TestProjectRuntimePartialPreparationReleasesOwnership(t *testing.T) {
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	c, _ := nativeTaskFixture(t)
	require(t, os.MkdirAll(EtcDir(), 0700))
	require(t, os.WriteFile(filepath.Join(EtcDir(), "one.json"), projectConfigJSON(t, c), 0600))
	// Fail after shared Runner start, native initialization and both project locks.
	require(t, os.WriteFile(filepath.Join(HomeDir(), "logs"), []byte("blocked"), 0600))
	d := NewApplicationDirectory(HomeDir())
	defer d.Close()
	d.Start()
	switchCatalog(t, d, func(catalog contract.Catalog) bool {
		return len(catalog.Projects) == 1 && catalog.Projects[0].RuntimeState == "failed"
	})
	require(t, d.Close())
	if _, err := os.Stat(filepath.Join(d.entries[0].ref.RunDir, "steerline.db")); err != nil {
		t.Fatal("failure did not reach acquired storage", err)
	}
	if !strings.Contains(d.entries[0].view.Diagnostic, "logs") {
		t.Fatal("failure did not reach log acquisition", d.entries[0].view)
	}
	result := d.ShutdownResults()[0]
	if !result.LocalClosed || !result.LocksReleased || result.InstanceID != "" {
		t.Fatal("partial startup invented outstanding resources", result)
	}
	for _, key := range []string{d.entries[0].ref.Identity, d.entries[0].ref.RunDir} {
		lock, err := host.LockRepository(key)
		require(t, err)
		lock.Close()
	}
	if !d.runner.result().Settled() {
		t.Fatal("shared Runner failed final settlement")
	}
}

// These fixtures hold real public entry paths, including the direct task receipt
// path that bypasses product.Client's own lock. No production cleanup hooks.
type heldRuntimePort struct {
	httpapi.QueryPort
	httpapi.CommandPort
	httpapi.AuthorityPort
	httpapi.NativeSessionPort
	entered, release chan struct{}
}

func (h *heldRuntimePort) block()                  { close(h.entered); <-h.release }
func (h *heldRuntimePort) Project() api.ProjectRef { return api.ProjectRef{Identity: "original"} }
func (h *heldRuntimePort) ListTasks(context.Context, string, int) (api.TaskPage, error) {
	h.block()
	return api.TaskPage{}, nil
}
func (h *heldRuntimePort) Command(context.Context, string) (api.CommandReceipt, error) {
	h.block()
	return api.CommandReceipt{}, errors.New("held original authority read")
}

type heldTaskStore struct {
	taskStorage
	held *heldRuntimePort
}

func (h *heldTaskStore) LookupCommand(context.Context, string, string) (ports.CommandRecord, bool, error) {
	h.held.block()
	return ports.CommandRecord{}, false, nil
}

func TestProjectRuntimeDrainsEveryPublicEntryBeforeClosing(t *testing.T) {
	for _, entry := range []string{"read", "command", "task_receipt"} {
		t.Run(entry, func(t *testing.T) {
			held := &heldRuntimePort{entered: make(chan struct{}), release: make(chan struct{})}
			server, err := httpapi.NewProject(httpapi.Ports{Query: held, Commands: held, Authority: held, NativeViewer: held}, httpapi.Options{RunDir: t.TempDir(), ProjectIdentity: "original", InstanceID: "instance"})
			require(t, err)
			p := &runtimeProjectPort{Server: server, store: &heldTaskStore{held: held}}
			log := &lifecycleLog{}
			r := &ProjectRuntime{port: p, log: log}
			done := make(chan struct{})
			go func() {
				defer close(done)
				switch entry {
				case "read":
					p.Read(context.Background(), "/tasks", nil)
				case "command":
					p.Command(context.Background(), httpcontract.CommandEnvelope{RequestID: strings.Repeat("a", 32), ProjectID: httpcontract.ProjectID("original"), InstanceID: "instance", Action: "open_native_session", TargetID: "run", Payload: []byte(`{}`)})
				case "task_receipt":
					p.Task(context.Background(), "GET", "/task-batches/receipt", nil)
				}
			}()
			select {
			case <-held.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("entry did not reach real public dependency")
			}
			p.StopAccepting()
			closing := make(chan struct{})
			go func() { r.close(); close(closing) }()
			select {
			case <-closing:
				t.Fatal("cleanup overtook admitted request")
			case <-time.After(20 * time.Millisecond):
			}
			if _, err := p.Read(context.Background(), "/identity", nil); err == nil {
				t.Fatal("read admitted while stopping")
			}
			if _, err := p.Command(context.Background(), httpcontract.CommandEnvelope{}); err == nil {
				t.Fatal("command admitted while stopping")
			}
			if _, err := p.Task(context.Background(), "GET", "/task-batches/other", nil); err == nil {
				t.Fatal("task receipt admitted while stopping")
			}
			close(held.release)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("admitted request stuck")
			}
			select {
			case <-closing:
			case <-time.After(3 * time.Second):
				t.Fatal("close failed to drain")
			}
			if log.closed != 1 || r.localErr != nil {
				t.Fatal("cleanup failed", log.closed, r.localErr)
			}
		})
	}
}
