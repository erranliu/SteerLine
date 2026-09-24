package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func readFakeEventually(path string, timeout time.Duration) (fakeDB, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(path)
		if err == nil {
			var db fakeDB
			if json.Unmarshal(b, &db) == nil {
				return db, nil
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fakeDB{}, errors.New("fake Beads snapshot did not become readable")
}

func readyControlService(t *testing.T, s *Service, mode string) {
	t.Helper()
	s.State.Service.Status = "running"
	s.State.Service.Ready = true
	s.State.Control.DispatchMode = mode
	if s.controlSeen == nil {
		s.controlSeen = map[string]string{}
	}
	require(t, s.save())
}

func controlRequestFor(s *Service, action string, payload any) ControlRequest {
	var b []byte
	if payload != nil {
		b, _ = json.Marshal(payload)
	}
	created := now()
	req := ControlRequest{ProtocolVersion: 1, RequestID: newID(), RepositoryIdentity: s.State.Repository.Identity, InstanceID: s.State.Service.InstanceID, Action: action, Payload: b, RequestedBy: "control-test", CreatedAt: created, ExpiresAt: created.Add(2 * time.Minute)}
	if action != "stop_service" {
		req.ExpectedControlRevision = s.State.Control.Revision
	}
	return req
}

func publishForService(t *testing.T, s *Service, req ControlRequest) {
	t.Helper()
	require(t, publishControlRequest(s.Config.RunDir, req))
}

func processOneControl(t *testing.T, s *Service, req ControlRequest) error {
	t.Helper()
	publishForService(t, s, req)
	return s.processNextControl(context.Background(), func() {})
}

func TestControlRequestPublicationIsImmutableAcrossClientProcesses(t *testing.T) {
	runDir := t.TempDir()
	created := now()
	req := ControlRequest{ProtocolVersion: 1, RequestID: newID(), RepositoryIdentity: filepath.Join(t.TempDir(), ".git"), InstanceID: newID(), Action: "pause_dispatch", RequestedBy: "process-race", CreatedAt: created, ExpiresAt: created.Add(time.Minute), ExpectedControlRevision: 1}
	requestFile := filepath.Join(t.TempDir(), "request.json")
	b, _ := json.Marshal(req)
	require(t, os.WriteFile(requestFile, b, 0600))
	exe, err := os.Executable()
	require(t, err)
	commands := []*exec.Cmd{exec.Command(exe, "--fake", "publish-control", runDir, requestFile), exec.Command(exe, "--fake", "publish-control", runDir, requestFile)}
	var wg sync.WaitGroup
	errs := make([]error, len(commands))
	for i, cmd := range commands {
		wg.Add(1)
		go func(i int, c *exec.Cmd) { defer wg.Done(); errs[i] = c.Run() }(i, cmd)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("publisher process %d failed: %v", i, err)
		}
	}
	path := controlRequestPath(runDir, req.RequestID)
	stored, err := readControlRequest(path)
	require(t, err)
	if requestDigest(stored) != requestDigest(req) {
		t.Fatal("published request differs from the client payload")
	}
	changed := req
	changed.Action = "resume_dispatch"
	changedFile := filepath.Join(t.TempDir(), "changed.json")
	b, _ = json.Marshal(changed)
	require(t, os.WriteFile(changedFile, b, 0600))
	if out, err := exec.Command(exe, "--fake", "publish-control", runDir, changedFile).CombinedOutput(); err == nil {
		t.Fatalf("different request replaced same ID: %s", out)
	}
	if err := publishControlRequest(runDir, req); err != nil {
		t.Fatalf("same ID and content should be idempotent: %v", err)
	}
}

func TestManualDispatchRefreshesCandidatesWithoutClaiming(t *testing.T) {
	f := newFixture(t, task("t-A"), task("t-B"))
	s := f.service(t)
	s.State.Control.DispatchMode = "manual"
	for i := 0; i < 5; i++ {
		require(t, s.dispatch(context.Background()))
	}
	db := readFake(f.db)
	if len(db.Claims) != 0 || len(db.Starts) != 0 {
		t.Fatalf("manual ticks performed task side effects: claims=%v starts=%v", db.Claims, db.Starts)
	}
	b, err := os.ReadFile(filepath.Join(f.c.RunDir, "tasks.json"))
	require(t, err)
	var snapshot TaskSnapshot
	require(t, json.Unmarshal(b, &snapshot))
	if len(snapshot.Tasks) != 2 || snapshot.Tasks[0].TaskID != "t-A" || snapshot.Tasks[1].TaskID != "t-B" {
		t.Fatalf("candidate snapshot: %+v", snapshot.Tasks)
	}
}

func TestCandidateSnapshotRetainsLastGoodTasksWhenProviderFails(t *testing.T) {
	f := newFixture(t, task("t-A"))
	s := f.service(t)
	s.State.Control.DispatchMode = "manual"
	require(t, s.dispatch(context.Background()))
	before := s.candidateSnapshot
	if before == nil || len(before.Tasks) != 1 || before.LastSuccessAt == nil {
		t.Fatalf("missing initial candidate snapshot: %+v", before)
	}
	db := readFake(f.db)
	db.Behavior = "bd-unavailable"
	writeFake(f.db, db)
	if err := s.dispatch(context.Background()); err == nil {
		t.Fatal("provider failure was not surfaced")
	}
	b, err := os.ReadFile(filepath.Join(f.c.RunDir, "tasks.json"))
	require(t, err)
	var after TaskSnapshot
	require(t, json.Unmarshal(b, &after))
	if len(after.Tasks) != 1 || after.Tasks[0].TaskID != "t-A" || after.Error == nil || after.LastSuccessAt == nil || !after.LastSuccessAt.Equal(*before.LastSuccessAt) {
		t.Fatalf("provider failure discarded the last good candidate list: %+v", after)
	}
	if len(readFake(f.db).Claims) != 0 || len(readFake(f.db).Starts) != 0 {
		t.Fatal("provider failure dispatched a candidate")
	}
}

func TestRunDefaultsToAutoDispatch(t *testing.T) {
	f := newFixture(t, task("t-A"))
	s := f.service(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		db, err := readFakeEventually(f.db, time.Second)
		if err != nil {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if db.Starts["t-A"] == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	db, err := readFakeEventually(f.db, time.Second)
	if err != nil || db.Claims["t-A"] != 1 || db.Starts["t-A"] != 1 {
		cancel()
		t.Fatalf("default run did not automatically claim and start ready work: %+v %v", db, err)
	}
	if s.State.Control.DispatchMode != "auto" {
		cancel()
		t.Fatalf("default run mode=%q", s.State.Control.DispatchMode)
	}
	cancel()
	select {
	case err := <-done:
		require(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("default run did not stop")
	}
}

func TestControlStartsOnlySelectedReadyTask(t *testing.T) {
	f := newFixture(t, task("t-A"), task("t-B"))
	s := f.service(t)
	readyControlService(t, s, "manual")
	req := controlRequestFor(s, "start_task", map[string]string{"task_id": "t-B"})
	require(t, processOneControl(t, s, req))
	db := readFake(f.db)
	startDeadline := time.Now().Add(5 * time.Second)
	for db.Starts["t-B"] == 0 && time.Now().Before(startDeadline) {
		time.Sleep(10 * time.Millisecond)
		db = readFake(f.db)
	}
	if db.Claims["t-B"] != 1 || db.Starts["t-B"] != 1 || db.Claims["t-A"] != 0 || db.Starts["t-A"] != 0 {
		t.Fatalf("selected B was not the only dispatched task: claims=%v starts=%v", db.Claims, db.Starts)
	}
	receipt, err := ReadControlReceipt(f.c, req.RequestID)
	require(t, err)
	if receipt.Status != "applied" || receipt.RunID == "" || receipt.RunID != receipt.AttemptID {
		t.Fatalf("start receipt: %+v", receipt)
	}
	if receipt.RunRevision == 0 {
		t.Fatalf("run revision was not attached to the receipt: %+v", receipt)
	}
	controlRevision := s.State.Control.Revision
	require(t, s.store.Heartbeat())
	loaded, err := LoadRuntime(filepath.Join(f.c.RunDir, f.c.RuntimeFile))
	require(t, err)
	if loaded.Control.Revision != controlRevision {
		t.Fatal("heartbeat changed control revision")
	}
	require(t, s.processNextControl(context.Background(), func() {}))
	db = readFake(f.db)
	if db.Claims["t-B"] != 1 || db.Starts["t-B"] != 1 {
		t.Fatalf("duplicate request replayed worker start: claims=%v starts=%v", db.Claims, db.Starts)
	}
	finishWorker(t, s)
}

func TestTwoDifferentStartRequestsCompeteForOneExecutionSlot(t *testing.T) {
	f := newFixture(t, task("t-A"), task("t-B"))
	s := f.service(t)
	readyControlService(t, s, "manual")
	first := controlRequestFor(s, "start_task", map[string]string{"task_id": "t-A"})
	first.RequestID = "00000000000000000000000000000001"
	second := controlRequestFor(s, "start_task", map[string]string{"task_id": "t-B"})
	second.RequestID = "ffffffffffffffffffffffffffffffff"
	publishForService(t, s, first)
	publishForService(t, s, second)
	require(t, s.processNextControl(context.Background(), func() {}))
	require(t, s.processNextControl(context.Background(), func() {}))
	db := readFake(f.db)
	totalClaims := db.Claims["t-A"] + db.Claims["t-B"]
	totalStarts := db.Starts["t-A"] + db.Starts["t-B"]
	if totalClaims != 1 || totalStarts != 1 {
		t.Fatalf("competing starts were not serialized: claims=%v starts=%v", db.Claims, db.Starts)
	}
	firstReceipt, err := ReadControlReceipt(f.c, first.RequestID)
	require(t, err)
	secondReceipt, err := ReadControlReceipt(f.c, second.RequestID)
	require(t, err)
	if firstReceipt.Status != "applied" || secondReceipt.Status != "rejected" || secondReceipt.Code != "stale_revision" {
		t.Fatalf("receipts first=%+v second=%+v", firstReceipt, secondReceipt)
	}
	finishWorker(t, s)
}

func TestControlRejectsUnavailableTargetWithoutFallback(t *testing.T) {
	f := newFixture(t, task("t-A"), task("t-B"))
	db := readFake(f.db)
	db.Tasks[1].Status = "closed"
	writeFake(f.db, db)
	s := f.service(t)
	readyControlService(t, s, "manual")
	req := controlRequestFor(s, "start_task", map[string]string{"task_id": "t-B"})
	require(t, processOneControl(t, s, req))
	got := readFake(f.db)
	if len(got.Claims) != 0 || len(got.Starts) != 0 {
		t.Fatalf("unavailable B fell back to another task: %+v", got)
	}
	receipt, err := ReadControlReceipt(f.c, req.RequestID)
	require(t, err)
	if receipt.Status != "rejected" || receipt.Code != "task_not_ready" {
		t.Fatalf("receipt=%+v", receipt)
	}
}

func TestControlModePauseResumeAndRevision(t *testing.T) {
	f := newFixture(t, task("t-A"))
	s := f.service(t)
	readyControlService(t, s, "auto")
	initial := s.State.Control.Revision
	mode := controlRequestFor(s, "set_dispatch_mode", map[string]string{"mode": "manual"})
	require(t, processOneControl(t, s, mode))
	if s.State.Control.DispatchMode != "manual" || s.State.Control.Revision <= initial {
		t.Fatalf("mode/revision not updated: %+v", s.State.Control)
	}
	pause := controlRequestFor(s, "pause_dispatch", nil)
	require(t, processOneControl(t, s, pause))
	if !s.State.Control.DispatchPaused {
		t.Fatal("pause was not applied")
	}
	rev := s.State.Control.Revision
	for i := 0; i < 3; i++ {
		require(t, s.dispatch(context.Background()))
	}
	if s.State.Control.Revision != rev {
		t.Fatal("candidate refresh/ticks invalidated the control revision")
	}
	resume := controlRequestFor(s, "resume_dispatch", nil)
	require(t, processOneControl(t, s, resume))
	if s.State.Control.DispatchPaused {
		t.Fatal("resume was not applied")
	}
	db := readFake(f.db)
	if len(db.Claims) != 0 || len(db.Starts) != 0 {
		t.Fatalf("manual/pause path dispatched: %+v", db)
	}
}

func TestPauseLeavesCurrentWorkerRunningAndBlocksNewDispatch(t *testing.T) {
	f := newFixture(t, task("t-A"), task("t-B"))
	s := f.service(t)
	readyControlService(t, s, "manual")
	start := controlRequestFor(s, "start_task", map[string]string{"task_id": "t-A"})
	require(t, processOneControl(t, s, start))
	worker := s.worker
	pause := controlRequestFor(s, "pause_dispatch", nil)
	require(t, processOneControl(t, s, pause))
	if s.worker != worker || s.worker == nil {
		t.Fatal("pause interrupted the active worker")
	}
	for i := 0; i < 3; i++ {
		require(t, s.dispatch(context.Background()))
	}
	db := readFake(f.db)
	if db.Claims["t-A"] != 1 || db.Starts["t-A"] != 1 || db.Claims["t-B"] != 0 || db.Starts["t-B"] != 0 {
		t.Fatalf("pause changed current work or dispatched another task: claims=%v starts=%v", db.Claims, db.Starts)
	}
	finishWorker(t, s)
}

func TestControlRejectsStaleInstanceExpiredRevisionAndWrongRepository(t *testing.T) {
	f := newFixture(t)
	s := f.service(t)
	readyControlService(t, s, "manual")
	cases := []struct {
		name, code string
		mutate     func(*ControlRequest)
	}{
		{"stale_instance", "stale_instance", func(r *ControlRequest) { r.InstanceID = newID() }},
		{"expired", "expired", func(r *ControlRequest) { r.ExpiresAt = now().Add(-time.Second) }},
		{"stale_revision", "stale_revision", func(r *ControlRequest) { r.ExpectedControlRevision++ }},
		{"wrong_repository", "invalid_request", func(r *ControlRequest) { r.RepositoryIdentity = filepath.Join(t.TempDir(), "other.git") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := controlRequestFor(s, "pause_dispatch", nil)
			tc.mutate(&req)
			require(t, processOneControl(t, s, req))
			receipt, err := ReadControlReceipt(f.c, req.RequestID)
			require(t, err)
			if receipt.Status != "rejected" || receipt.Code != tc.code {
				t.Fatalf("receipt=%+v", receipt)
			}
		})
	}
	db := readFake(f.db)
	if len(db.Claims) != 0 || len(db.Starts) != 0 {
		t.Fatalf("invalid requests had task side effects: %+v", db)
	}
}

func TestApplyingReceiptFailurePreventsDispatchAndRecoveryDoesNotReplay(t *testing.T) {
	f := newFixture(t, task("t-A"))
	s := f.service(t)
	readyControlService(t, s, "manual")
	req := controlRequestFor(s, "start_task", map[string]string{"task_id": "t-A"})
	publishForService(t, s, req)
	path := controlReceiptPath(f.c.RunDir, req.RequestID)
	require(t, os.MkdirAll(filepath.Dir(path), 0700))
	s.controlFault = func(point string) {
		if point == "after_checkpoint" {
			if err := os.Mkdir(path, 0700); err != nil {
				panic(err)
			}
		}
	}
	processErr := s.processNextControl(context.Background(), func() {})
	if processErr == nil {
		t.Fatal("receipt publication failure was ignored")
	}
	if s.State.Control.Operation == nil {
		t.Fatalf("control operation was not checkpointed: %v", processErr)
	}
	if rt, loadErr := LoadRuntime(filepath.Join(f.c.RunDir, f.c.RuntimeFile)); loadErr != nil || rt == nil || rt.Control.Operation == nil {
		t.Fatalf("durable checkpoint missing: runtime=%+v err=%v process=%v", rt, loadErr, processErr)
	}
	db := readFake(f.db)
	if len(db.Claims) != 0 || len(db.Starts) != 0 {
		t.Fatalf("dispatch continued after applying receipt failure: %+v", db)
	}
	require(t, os.Remove(path))
	restarted := f.service(t)
	if restarted.State.Control.Operation == nil {
		t.Fatal("restart lost the applying checkpoint")
	}
	require(t, restarted.recoverControlOperation())
	db = readFake(f.db)
	if len(db.Claims) != 0 || len(db.Starts) != 0 {
		t.Fatalf("recovery replayed task side effects: %+v", db)
	}
	if len(restarted.State.Control.Blockers) != 0 {
		t.Fatalf("no-side-effect recovery created an unnecessary blocker: %+v", restarted.State.Control.Blockers)
	}
	receipt, err := ReadControlReceipt(f.c, req.RequestID)
	require(t, err)
	if receipt.Status != "failed" || receipt.Code != "persistence_failed" {
		t.Fatalf("receipt=%+v", receipt)
	}
}

func TestControlCrashBoundariesNeverReplayClaimWorktreeOrWorker(t *testing.T) {
	cases := []struct {
		point                     string
		wantClaim, wantStart      int
		wantWorktree, wantBlocker bool
	}{
		{"after_checkpoint", 0, 0, false, false}, {"after_applying_receipt", 0, 0, false, true}, {"after_claim", 1, 0, false, true}, {"after_worktree", 1, 0, true, true}, {"after_worker_start", 1, 1, true, true}, {"worker_persisted", 1, 1, true, true}, {"terminal_checkpoint", 1, 1, true, false}, {"after_terminal_receipt", 1, 1, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.point, func(t *testing.T) {
			f := newFixture(t, task("t-A"))
			s := f.service(t)
			readyControlService(t, s, "manual")
			req := controlRequestFor(s, "start_task", map[string]string{"task_id": "t-A"})
			publishForService(t, s, req)
			s.controlFault = func(point string) {
				if point == tc.point {
					panic("injected crash at " + point)
				}
			}
			func() {
				defer func() {
					if recover() == nil {
						t.Fatalf("fault point %s did not fire", tc.point)
					}
				}()
				_ = s.processNextControl(context.Background(), func() {})
			}()
			if tc.wantStart > 0 {
				deadline := time.Now().Add(15 * time.Second)
				for time.Now().Before(deadline) {
					db, err := readFakeEventually(f.db, time.Second)
					if err == nil && db.Starts["t-A"] == tc.wantStart {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
			if s.worker != nil {
				s.worker.Kill()
				select {
				case <-s.worker.Done:
				case <-time.After(3 * time.Second):
					t.Fatal("worker cleanup timed out")
				}
			}
			db := readFake(f.db)
			if db.Claims["t-A"] != tc.wantClaim || db.Starts["t-A"] != tc.wantStart {
				runID := ""
				if s.State.Control.Operation != nil {
					runID = s.State.Control.Operation.RunID
				}
				stderr, _ := os.ReadFile(filepath.Join(f.c.RunDir, "runs", runID, "stderr.log"))
				t.Fatalf("before restart counts claims=%v starts=%v stderr=%s", db.Claims, db.Starts, stderr)
			}
			runID := ""
			if s.State.Control.Operation != nil {
				runID = s.State.Control.Operation.RunID
			}
			if tc.wantWorktree {
				if _, err := os.Stat(filepath.Join(f.c.RunDir, "worktrees", runID)); err != nil {
					t.Fatalf("expected existing worktree at crash boundary: %v", err)
				}
			}
			restarted := f.service(t)
			require(t, restarted.recoverControlOperation())
			db = readFake(f.db)
			if db.Claims["t-A"] != tc.wantClaim || db.Starts["t-A"] != tc.wantStart {
				t.Fatalf("restart replayed an external action: claims=%v starts=%v", db.Claims, db.Starts)
			}
			if got := len(restarted.State.Control.Blockers) > 0; got != tc.wantBlocker {
				t.Fatalf("blocker present=%v want=%v", got, tc.wantBlocker)
			}
		})
	}
}

func TestTerminalReceiptWriteFailureBlocksNewWorkAndRecoversCheckpoint(t *testing.T) {
	f := newFixture(t, task("t-A"), task("t-B"))
	s := f.service(t)
	readyControlService(t, s, "manual")
	req := controlRequestFor(s, "start_task", map[string]string{"task_id": "t-A"})
	publishForService(t, s, req)
	path := controlReceiptPath(f.c.RunDir, req.RequestID)
	s.controlFault = func(point string) {
		if point == "terminal_checkpoint" {
			if err := os.Remove(path); err != nil {
				panic(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				panic(err)
			}
		}
	}
	if err := s.processNextControl(context.Background(), func() {}); err == nil {
		t.Fatal("terminal receipt write failure was ignored")
	}
	if s.State.Control.Operation == nil || s.State.Control.Operation.Phase != "completed" {
		t.Fatalf("terminal checkpoint was not retained: %+v", s.State.Control.Operation)
	}
	if s.slotReason("") == "" {
		t.Fatal("unpublished terminal receipt did not block the next operation")
	}
	db := readFake(f.db)
	if db.Claims["t-A"] != 1 || db.Starts["t-A"] != 1 || db.Claims["t-B"] != 0 || db.Starts["t-B"] != 0 {
		t.Fatalf("unexpected task side effects: claims=%v starts=%v", db.Claims, db.Starts)
	}
	if s.worker != nil {
		s.worker.Kill()
		select {
		case <-s.worker.Done:
		case <-time.After(3 * time.Second):
			t.Fatal("worker cleanup timed out")
		}
	}
	require(t, os.Remove(path))
	restarted := f.service(t)
	require(t, restarted.recoverControlOperation())
	receipt, err := ReadControlReceipt(f.c, req.RequestID)
	require(t, err)
	if receipt.Status != "applied" {
		t.Fatalf("receipt recovered as %+v", receipt)
	}
	db = readFake(f.db)
	if db.Claims["t-A"] != 1 || db.Starts["t-A"] != 1 {
		t.Fatalf("receipt recovery replayed the task: claims=%v starts=%v", db.Claims, db.Starts)
	}
}

func TestControlCompletedCheckpointRebuildsReceiptWithoutReapplying(t *testing.T) {
	for _, point := range []string{"terminal_checkpoint", "after_terminal_receipt"} {
		t.Run(point, func(t *testing.T) {
			f := newFixture(t)
			s := f.service(t)
			readyControlService(t, s, "manual")
			req := controlRequestFor(s, "pause_dispatch", nil)
			publishForService(t, s, req)
			s.controlFault = func(got string) {
				if got == point {
					panic("injected " + point)
				}
			}
			func() {
				defer func() {
					if recover() == nil {
						t.Fatalf("fault point %s did not fire", point)
					}
				}()
				_ = s.processNextControl(context.Background(), func() {})
			}()
			if !s.State.Control.DispatchPaused {
				t.Fatal("pause action state was not checkpointed")
			}
			restarted := f.service(t)
			require(t, restarted.recoverControlOperation())
			receipt, err := ReadControlReceipt(f.c, req.RequestID)
			require(t, err)
			if receipt.Status != "applied" || !restarted.State.Control.DispatchPaused {
				t.Fatalf("recovered state/receipt: %+v %+v", restarted.State.Control, receipt)
			}
		})
	}
}

func TestCrashBeforeModeTransitionDoesNotReportItApplied(t *testing.T) {
	f := newFixture(t)
	s := f.service(t)
	readyControlService(t, s, "manual")
	req := controlRequestFor(s, "pause_dispatch", nil)
	publishForService(t, s, req)
	s.controlFault = func(point string) {
		if point == "after_applying_receipt" {
			panic("crash before state mutation")
		}
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("fault point did not fire")
			}
		}()
		_ = s.processNextControl(context.Background(), func() {})
	}()
	restarted := f.service(t)
	require(t, restarted.recoverControlOperation())
	receipt, err := ReadControlReceipt(f.c, req.RequestID)
	require(t, err)
	if receipt.Status != "failed" || restarted.State.Control.DispatchPaused {
		t.Fatalf("transition recovery state=%+v receipt=%+v", restarted.State.Control, receipt)
	}
}

func TestControlStopCancelsLongPreparationAndPublishesOnlyAfterStopped(t *testing.T) {
	f := newFixture(t, task("t-A"))
	db := readFake(f.db)
	db.Behavior = "slow-context"
	writeFake(f.db, db)
	s := f.service(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.RunWithDispatch(ctx, "manual") }()
	var startReq ControlRequest
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := ControlSnapshot(f.c); err == nil {
			startReq, err = SubmitControl(f.c, "start_task", map[string]string{"task_id": "t-A"}, "test")
			require(t, err)
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("control service did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	deadline = time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(f.db + ".context-started"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("control start did not reach the cancellable preparation command")
		}
		time.Sleep(10 * time.Millisecond)
	}
	startID := startReq.RequestID
	stopReq, err := SubmitControl(f.c, "stop_service", nil, "test")
	require(t, err)
	select {
	case err = <-done:
		require(t, err)
	case <-time.After(4 * time.Second):
		t.Fatal("stop did not cancel the long preparation command")
	}
	stopReceipt, err := ReadControlReceipt(f.c, stopReq.RequestID)
	require(t, err)
	if stopReceipt.Status != "applied" || s.State.Service.Status != "stopped" {
		t.Fatalf("stop receipt/state: %+v status=%s", stopReceipt, s.State.Service.Status)
	}
	startReceipt, err := ReadControlReceipt(f.c, startID)
	require(t, err)
	if startReceipt.Status == "applying" {
		t.Fatalf("interrupted operation was left unsettled: %+v", startReceipt)
	}
	db = readFake(f.db)
	if len(db.Claims) != 0 || len(db.Starts) != 0 {
		t.Fatalf("cancelled preparation claimed or started a task: %+v", db)
	}
}

func TestRuntimeV1ToV4RejectedWithoutMigration(t *testing.T) {
	for _, version := range []int{1, 2, 3, 4} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			f := newFixture(t)
			legacy := map[string]any{"schema_version": version, "repository": map[string]any{"path": f.c.Repository, "remote": f.c.Remote, "branch": f.c.Branch, "identity": f.c.identity}, "service": map[string]any{"status": "stopped", "claimant": f.c.Claimant}, "runs": []any{map[string]any{"task_id": "old-task", "title": "old task", "run_id": "0123456789abcdef0123456789abcdef", "claimant": f.c.Claimant, "beads_status": "in_progress", "execution_status": "succeeded", "finish_task_status": "waiting_pr_merge", "pr": map[string]any{"url": "https://gitea.example/owner/repo/pulls/42", "state": "open"}}}}
			b, err := json.Marshal(legacy)
			require(t, err)
			path := filepath.Join(f.c.RunDir, f.c.RuntimeFile)
			require(t, os.WriteFile(path, b, 0600))
			if _, err := LoadRuntime(path); err == nil || !strings.Contains(err.Error(), "unsupported runtime schema") {
				t.Fatalf("old runtime was accepted: %v", err)
			}
			onDisk, err := os.ReadFile(path)
			require(t, err)
			var disk Runtime
			require(t, json.Unmarshal(onDisk, &disk))
			if disk.SchemaVersion != version {
				t.Fatal("read-only load migrated the file")
			}
			if _, err := NewService(f.c); err == nil || !strings.Contains(err.Error(), "unsupported runtime schema") {
				t.Fatalf("service accepted old runtime: %v", err)
			}
			after, err := os.ReadFile(path)
			require(t, err)
			if !bytes.Equal(b, after) {
				t.Fatal("old runtime changed during rejection")
			}
		})
	}
}

func TestControlValidationRejectsUnknownPayloadFields(t *testing.T) {
	req := ControlRequest{Action: "set_dispatch_mode", Payload: json.RawMessage(`{"mode":"manual","surprise":true}`)}
	if _, err := decodeControlPayload(req); err == nil {
		t.Fatal("unknown payload field was accepted")
	}
	if _, err := decodeControlPayload(ControlRequest{Action: "pause_dispatch", Payload: json.RawMessage(`null`)}); err == nil {
		t.Fatal("null action payload was accepted")
	}
	dir := t.TempDir()
	instanceID := newID()
	repository := filepath.Join(dir, "repo.git")
	created := now()
	badStop := ControlRequest{ProtocolVersion: 1, RequestID: newID(), RepositoryIdentity: repository, InstanceID: instanceID, Action: "stop_service", Payload: json.RawMessage(`{"task_id":"unexpected"}`), RequestedBy: "test", CreatedAt: created, ExpiresAt: created.Add(time.Minute)}
	require(t, publishControlRequest(dir, badStop))
	if req, _ := findStopRequest(Config{RunDir: dir, identity: repository}, instanceID); req != nil {
		t.Fatal("invalid stop payload bypassed protocol validation")
	}
}
