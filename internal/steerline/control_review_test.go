package steerline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func controlReviewService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	c := Config{RunDir: dir, RuntimeFile: "runtime.json", Repository: dir, Remote: "origin", Branch: "main", Claimant: "review", Heartbeat: "5s", identity: filepath.Join(dir, ".git")}
	s := &Service{Config: c, store: &Store{Path: filepath.Join(dir, c.RuntimeFile)}, activities: map[string]*ActivityWriter{}, State: Runtime{
		SchemaVersion: 4, Repository: Repository{Identity: c.identity, Remote: c.Remote, Branch: c.Branch},
		Service: ServiceState{Status: "running", Ready: true, InstanceID: newID(), Claimant: c.Claimant, ControlProtocol: 1},
		Control: ControlState{DispatchMode: "manual", Revision: 1, Blockers: []ControlBlocker{}}, Runs: []*Run{},
	}}
	require(t, s.save())
	return s
}

func TestControlReviewMissingRequiredPayload(t *testing.T) {
	for _, action := range []string{"start_task", "set_dispatch_mode"} {
		if _, err := decodeControlPayload(ControlRequest{Action: action}); err == nil {
			t.Errorf("%s accepted an absent required payload", action)
		}
	}
}

func TestControlReviewMismatchedIDCannotSelectReceiptPath(t *testing.T) {
	s := controlReviewService(t)
	id := newID()
	sentinel := filepath.Join(s.Config.RunDir, "sentinel.json")
	require(t, os.WriteFile(sentinel, []byte("preserve"), 0600))
	req := controlRequestFor(s, "pause_dispatch", nil)
	req.RequestID = "../../sentinel"
	path := controlRequestPath(s.Config.RunDir, id)
	require(t, os.MkdirAll(filepath.Dir(path), 0700))
	b, err := json.Marshal(req)
	require(t, err)
	require(t, os.WriteFile(path, b, 0600))
	require(t, s.processNextControl(context.Background(), func() {}))
	got, err := os.ReadFile(sentinel)
	require(t, err)
	if string(got) != "preserve" {
		t.Fatal("request body selected a receipt path outside the receipt directory")
	}
	r, err := ReadControlReceipt(s.Config, id)
	require(t, err)
	if r.Status != "rejected" || r.Code != "invalid_request" || s.State.Control.DispatchPaused {
		t.Fatalf("mismatched request was not safely rejected: %+v", r)
	}
}

func TestControlReviewEmptyObjectStopCancels(t *testing.T) {
	s := controlReviewService(t)
	req := controlRequestFor(s, "stop_service", map[string]string{})
	publishForService(t, s, req)
	if found, _ := findStopRequest(s.Config, s.State.Service.InstanceID); found == nil || found.RequestID != req.RequestID {
		t.Fatal("valid empty stop payload is invisible to the cancellation watcher")
	}
}

func TestControlReviewCanceledContextDoesNotApplyMode(t *testing.T) {
	s := controlReviewService(t)
	req := controlRequestFor(s, "pause_dispatch", nil)
	publishForService(t, s, req)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = s.processNextControl(ctx, func() {})
	if s.State.Control.DispatchPaused || s.State.Control.Operation != nil {
		t.Fatal("a new control action ran after service cancellation")
	}
}

func TestControlReviewDegradedReadyServiceCanPause(t *testing.T) {
	s := controlReviewService(t)
	s.State.Service.Status = "error"
	require(t, s.save())
	req := controlRequestFor(s, "pause_dispatch", nil)
	require(t, processOneControl(t, s, req))
	r, err := ReadControlReceipt(s.Config, req.RequestID)
	require(t, err)
	if r.Status != "applied" || !s.State.Control.DispatchPaused {
		t.Fatalf("an unrelated health error prevented pause: %+v", r)
	}
}

func TestControlReviewCanStopInitializingService(t *testing.T) {
	s := controlReviewService(t)
	s.State.Service.Ready = false
	require(t, s.save())
	if _, err := SubmitControl(s.Config, "stop_service", nil, "review"); err != nil {
		t.Fatalf("cannot stop an instance still initializing: %v", err)
	}
}

func TestControlReviewCompletedHistoryIsNotReparsedOrOverwritten(t *testing.T) {
	s := controlReviewService(t)
	req := controlRequestFor(s, "pause_dispatch", nil)
	require(t, processOneControl(t, s, req))
	require(t, os.WriteFile(controlRequestPath(s.Config.RunDir, req.RequestID), []byte("unreadable old archive"), 0600))
	require(t, s.processNextControl(context.Background(), func() {}))
	r, err := ReadControlReceipt(s.Config, req.RequestID)
	require(t, err)
	if r.Status != "applied" {
		t.Fatalf("rescanning an immutable historical request overwrote its receipt: %+v", r)
	}
}

func TestControlReviewSubmittedAnswerWaitsForUnknownExecution(t *testing.T) {
	s := controlReviewService(t)
	r := decisionRun(t)
	decision, err := WriteDecisionRequest(r, DecisionRequest{Question: "choose", Options: []DecisionOption{{ID: "a", Label: "A"}}})
	require(t, err)
	r.Decision = decision
	r.ExecutionStatus = "succeeded"
	require(t, WriteAnswer(r, DecisionAnswer{OptionID: "a", AnsweredBy: "review"}))
	s.State.Runs = []*Run{r, {TaskID: "other", RunID: newID(), ExecutionStatus: "unknown", FinishTaskStatus: "blocked"}}
	_ = s.applyDecisions(context.Background())
	if r.Decision.Status != "pending" || r.FinishTaskStatus != "waiting_decision" {
		t.Fatal("answer was consumed despite another unresolved execution")
	}
}

func TestControlReviewPublishedSlotReflectsWaitingDecision(t *testing.T) {
	s := controlReviewService(t)
	s.State.Runs = []*Run{{TaskID: "waiting", RunID: newID(), ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_decision"}}
	require(t, s.save())
	rt, err := LoadRuntime(s.store.Path)
	require(t, err)
	if !rt.SlotOccupied {
		t.Fatal("published runtime reports a free slot reserved for a decision")
	}
}

func TestControlReviewPrepareFailureIsNotClaimConflict(t *testing.T) {
	f := newFixture(t, task("target"))
	s := f.service(t)
	readyControlService(t, s, "manual")
	s.Config.Codex.Executable = filepath.Join(f.c.RunDir, "missing-worker.exe")
	req := controlRequestFor(s, "start_task", map[string]string{"task_id": "target"})
	require(t, processOneControl(t, s, req))
	r, err := ReadControlReceipt(f.c, req.RequestID)
	require(t, err)
	if r.Status != "failed" || r.Code == "claim_conflict" || readFake(f.db).Claims["target"] != 1 {
		t.Fatalf("post-claim startup failure was mislabeled: %+v", r)
	}
}

func TestControlReviewAutoDispatchCannotIgnorePendingReceipt(t *testing.T) {
	f := newFixture(t, task("target"))
	s := f.service(t)
	readyControlService(t, s, "auto")
	s.State.Control.Operation = &ControlOperation{RequestID: newID(), Action: "pause_dispatch", Phase: "completed", RequestDigest: strings.Repeat("a", 64), Result: &ControlReceipt{Version: 1, Status: "applied"}}
	require(t, s.save())
	_ = s.dispatch(context.Background())
	if s.worker != nil {
		finishWorker(t, s)
	}
	if len(readFake(f.db).Claims) != 0 {
		t.Fatal("automatic dispatch bypassed an unpublished terminal checkpoint")
	}
}

func TestControlReviewRetryHonorsPersistentBlocker(t *testing.T) {
	f, s, r := failedStartup(t)
	s.State.Control.Blockers = []ControlBlocker{{RequestID: newID(), Reason: "unresolved prior execution"}}
	oldLog := r.LogPath
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := s.RetryOne(ctx, r.RunID)
	if err == nil || readFake(f.db).Starts[r.TaskID] != 1 || r.LogPath != oldLog {
		t.Fatalf("retry bypassed the persistent execution blocker: err=%v", err)
	}
}

func TestControlReviewMissingReceiptDoesNotEraseClaimEvidence(t *testing.T) {
	f := newFixture(t, task("target"))
	s := f.service(t)
	readyControlService(t, s, "manual")
	req := controlRequestFor(s, "start_task", map[string]string{"task_id": "target"})
	publishForService(t, s, req)
	s.controlFault = func(point string) {
		if point == "after_claim" {
			panic("crash after external claim")
		}
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("claim fault did not fire")
			}
		}()
		_ = s.processNextControl(context.Background(), func() {})
	}()
	require(t, os.Remove(controlReceiptPath(f.c.RunDir, req.RequestID)))
	restarted := f.service(t)
	require(t, restarted.recoverControlOperation())
	r, err := ReadControlReceipt(f.c, req.RequestID)
	require(t, err)
	if r.Status != "indeterminate" || len(restarted.State.Control.Blockers) == 0 || readFake(f.db).Claims["target"] != 1 {
		t.Fatalf("missing receipt was mistaken for proof of no claim: %+v", r)
	}
}

func TestControlReviewPRCompletionDoesNotInvalidateDispatchRevision(t *testing.T) {
	s := controlReviewService(t)
	r := &Run{RunID: newID(), TaskID: "delivered", ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_pr_merge"}
	s.State.Runs = []*Run{r}
	require(t, s.save())
	before, runBefore := s.State.Control.Revision, r.ControlRevision
	r.FinishTaskStatus = "completed"
	require(t, s.save())
	if s.State.Control.Revision != before || r.ControlRevision != runBefore+1 {
		t.Fatalf("unrelated delivery changed dispatch eligibility version: control=%d/%d run=%d/%d", before, s.State.Control.Revision, runBefore, r.ControlRevision)
	}
}

func TestControlReviewRunRevisionTracksWorkspaceEvidence(t *testing.T) {
	s := controlReviewService(t)
	r := &Run{RunID: newID(), TaskID: "recover", ExecutionStatus: "failed", FinishTaskStatus: "blocked"}
	s.State.Runs = []*Run{r}
	require(t, s.save())
	before := r.ControlRevision
	r.WorktreePath = ptr(filepath.Join(s.Config.RunDir, "worktree"))
	require(t, s.save())
	if r.ControlRevision != before+1 {
		t.Fatal("changed recovery evidence retained a stale run revision")
	}
}

func TestControlReviewNewInstanceStartsControlRevisionAtOne(t *testing.T) {
	f := newFixture(t)
	s := f.service(t)
	s.State.Control.Revision = 40
	require(t, s.save())
	restarted := f.service(t)
	if restarted.State.Control.Revision != 1 {
		t.Fatalf("new instance retained old control revision %d", restarted.State.Control.Revision)
	}
}

func TestControlReviewStopAcceptanceSurvivesExpiryDuringCleanup(t *testing.T) {
	f := newFixture(t, task("target"))
	db := readFake(f.db)
	db.Behavior = "slow-context"
	writeFake(f.db, db)
	s := f.service(t)
	expiry := make(chan time.Time, 1)
	s.controlFault = func(point string) {
		if point == "terminal_checkpoint" {
			until := <-expiry
			time.Sleep(time.Until(until) + 20*time.Millisecond)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.RunWithDispatch(ctx, "manual") }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := ControlSnapshot(f.c); err == nil {
			_, err = SubmitControl(f.c, "start_task", map[string]string{"task_id": "target"}, "review")
			require(t, err)
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("service not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for {
		if _, err := os.Stat(f.db + ".context-started"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("start did not enter long preparation")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Construct from the published snapshot; do not read the live Service state.
	rt, err := LoadRuntime(filepath.Join(f.c.RunDir, f.c.RuntimeFile))
	require(t, err)
	created := now()
	req := ControlRequest{ProtocolVersion: 1, RequestID: newID(), RepositoryIdentity: rt.Repository.Identity, InstanceID: rt.Service.InstanceID, Action: "stop_service", RequestedBy: "review", CreatedAt: created, ExpiresAt: created.Add(time.Second)}
	expiry <- req.ExpiresAt
	require(t, publishControlRequest(f.c.RunDir, req))
	select {
	case err = <-done:
		require(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("stop cleanup timed out")
	}
	receipt, err := ReadControlReceipt(f.c, req.RequestID)
	require(t, err)
	if receipt.Status != "applied" {
		t.Fatalf("accepted stop expired during cleanup: %+v", receipt)
	}
	if receipt.AcceptedAt == nil || receipt.AcceptedAt.After(req.ExpiresAt) {
		t.Fatalf("receipt lost the actual pre-expiry acceptance time: %+v", receipt)
	}
}

func TestControlReviewTerminalReceiptFailureKeepsServiceControllable(t *testing.T) {
	f := newFixture(t)
	s := f.service(t)
	blockedPath := make(chan string, 1)
	s.controlFault = func(point string) {
		if point == "terminal_checkpoint" {
			path := controlReceiptPath(f.c.RunDir, s.State.Control.Operation.RequestID)
			if err := os.Remove(path); err != nil {
				panic(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				panic(err)
			}
			blockedPath <- path
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.RunWithDispatch(ctx, "manual") }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := ControlSnapshot(f.c); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("service not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	req, err := SubmitControl(f.c, "pause_dispatch", nil, "review")
	require(t, err)
	var path string
	select {
	case path = <-blockedPath:
	case <-time.After(5 * time.Second):
		t.Fatal("receipt failure was not injected")
	}
	select {
	case err := <-done:
		t.Fatalf("receipt publication failure killed the control service: %v", err)
	case <-time.After(2 * time.Second):
	}
	require(t, os.Remove(path))
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer waitCancel()
	receipt, err := WaitControlReceipt(waitCtx, f.c, req.RequestID, 10*time.Millisecond)
	require(t, err)
	if receipt.Status != "applied" {
		t.Fatalf("terminal checkpoint did not republish after repair: %+v", receipt)
	}
	require(t, RequestStop(f.c))
	select {
	case err := <-done:
		require(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("service stopped responding after receipt recovery")
	}
}

func TestControlReviewInterruptedDecisionRetainsUnknownReservation(t *testing.T) {
	f := newFixture(t)
	s := f.service(t)
	s.State.Runs = []*Run{{RunID: newID(), TaskID: "waiting", Claimant: f.c.Claimant, ExecutionStatus: "succeeded", FinishTaskStatus: "waiting_decision", Decision: &DecisionState{ID: newID(), Status: "resuming"}}}
	require(t, s.save())
	restarted := f.service(t)
	require(t, restarted.save())
	if restarted.State.Runs[0].ExecutionStatus != "unknown" || !restarted.State.SlotOccupied {
		t.Fatal("interrupted terminal continuation released the execution reservation")
	}
}

func TestControlReviewReadyWaitsForPublishedCandidates(t *testing.T) {
	f := newFixture(t)
	require(t, os.Mkdir(filepath.Join(f.c.RunDir, "tasks.json"), 0700))
	s := f.service(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.RunWithDispatch(ctx, "manual") }()
	readyWithoutCandidates := false
	for ctx.Err() == nil {
		if rt, err := LoadRuntime(filepath.Join(f.c.RunDir, f.c.RuntimeFile)); err == nil && rt != nil && rt.Service.Ready {
			readyWithoutCandidates = true
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("service cleanup timed out")
	}
	if readyWithoutCandidates {
		t.Fatal("service advertised ready despite candidate publication failure")
	}
}

func TestControlReviewRequestExpiresBeforeAcceptance(t *testing.T) {
	f := newFixture(t, task("target"))
	db := readFake(f.db)
	db.Behavior = "slow-ready-control"
	writeFake(f.db, db)
	s := f.service(t)
	readyControlService(t, s, "manual")
	req := controlRequestFor(s, "start_task", map[string]string{"task_id": "target"})
	req.ExpiresAt = req.CreatedAt.Add(500 * time.Millisecond)
	require(t, processOneControl(t, s, req))
	if s.worker != nil {
		finishWorker(t, s)
	}
	r, err := ReadControlReceipt(f.c, req.RequestID)
	require(t, err)
	if r.Status != "rejected" || r.Code != "expired" || len(readFake(f.db).Claims) != 0 {
		t.Fatalf("task was accepted after slow eligibility checks exceeded expiry: %+v", r)
	}
}
