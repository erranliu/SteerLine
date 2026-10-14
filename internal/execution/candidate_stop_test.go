package execution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
	m "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func reserveStopTestCandidates(t *testing.T, s *Service, count int) []string {
	t.Helper()
	ctx := context.Background()
	var ids []string
	for i := 0; i < count; i++ {
		graph, err := s.commandStore.TaskGraphSnapshot(ctx, s.Config.identity)
		require(t, err)
		config, profile, err := s.runtime.Profiles.ResolveExecutionProfile("first", "")
		require(t, err)
		id := newID()
		_, err = s.commandService(s.Config.identity).ClaimNativeTask(ctx, config, TaskClaimOptions{
			TaskID: "101", RequestID: id, WorkflowRunID: id, ExpectedTaskVersion: graph.ByID["101"].Version,
			ExpectedAuthorityVersion: graph.Authority.AuthorityVersion, ExpectedGraphRevision: graph.Authority.GraphRevision,
			RequestedBy: "operator", Explicit: true, ExecutionProfile: &profile, ServiceInstanceID: s.State.Service.InstanceID,
			reserveOnly: true, startCandidate: i != 0 || graph.ByID["101"].Status == "in_progress"})
		require(t, err)
		ids = append(ids, id)
	}
	require(t, s.refreshNativeSnapshot(ctx))
	return ids
}

type stopCountResolver struct {
	target string
	calls  *atomic.Int32
}

func (r stopCountResolver) ResolveLifecycle(setup b.LifecycleSetup) (b.LifecyclePort, error) {
	if setup.Snapshot.Session.WorkflowRunID == r.target {
		r.calls.Add(1)
	}
	return mcPort{&neutralLifecycle{Setup: setup}}, nil
}

func TestCandidateStopFactsDeliveryRaceAndRestart(t *testing.T) {
	f := newMCFixture(t)
	secondReceipt, _ := f.start(t, "candidate-b")
	thirdReceipt, _ := f.start(t, "candidate-c")
	_, _ = f.start(t, "candidate-d")
	mcStartTurn(t, f.s, f.first)
	mcFinishTurn(t, f.s, f.first)
	f.bind(t, f.first, 88, "open")
	f.proof(t, f.first, 88, "merged")
	ctx := context.Background()
	task := nativeTaskRecord(f.s.domain.Tasks, "8")
	capture := &resultCaptureStore{CommandStore: f.store}
	verifier := &mcCountVerifier{TaskDeliveryVerifier: testPublicationClient(f.c)}
	_, err := newTestCommandService(capture, f.c).CompleteNativeTask(ctx, f.c, "8", task.Version, f.first.RunID, verifier)
	if !errors.Is(err, errCapturedResult) {
		t.Fatal("delivery did not reach its write boundary", err)
	}
	req, payload, receipt := stopTestRequest(f.s, "8", f.first.RunID)
	_, err = f.commands.StopCandidate(ctx, req, payload, receipt)
	require(t, err)
	_, err = f.store.CommitTaskResult(ctx, capture.commit)
	if !isNativeErrorCode(err, "candidate_inactive") {
		t.Fatal("stale delivery committed a stopped candidate", err)
	}
	require(t, f.s.refreshNativeSnapshot(ctx))
	if len(f.s.domain.Deliveries) != 0 || nativeTaskRecord(f.s.domain.Tasks, "8").Status != "in_progress" {
		t.Fatal("stop/failed delivery changed TaskDelivery")
	}
	if _, err = f.commands.CompleteNativeTask(ctx, f.c, "8", task.Version, f.first.RunID, verifier); !isNativeErrorCode(err, "candidate_inactive") {
		t.Fatal("stopped candidate remained deliverable", err)
	}
	if len(verifier.urls) != 1 {
		t.Fatal("stopped candidate initiated another PR query")
	}
	run := f.s.find(f.first.RunID)
	w, e := f.s.workflowRecords(run)
	terminal, identity := digestValue(*w), digestValue(struct {
		Workspace m.WorkspaceEvidence
		Process   *b.ProcessIdentity
		Session   string
	}{e.Workspace, e.Process, e.SessionID})
	beforeAttempts, beforeCommands := len(f.s.domain.Attempts), 0
	require(t, f.store.db.QueryRow(`SELECT COUNT(*) FROM commands`).Scan(&beforeCommands))
	var calls atomic.Int32
	f.s.lifecycleResolver = stopCountResolver{target: run.RunID, calls: &calls}
	// A completion proposal is a received fact, not authorization to report it.
	event := WorkerEvent{RunID: run.RunID, AttemptID: run.AttemptID, Summary: "late original output",
		Fact: b.EventFact{SessionID: e.SessionID}, Candidate: &b.LifecycleCandidate{Action: b.LifecycleReportCompletion, Input: b.LifecycleInput{Completion: &b.CompletionInput{TurnID: e.CurrentTurnID, Outcome: "succeeded", Body: "late original final"}}}}
	require(t, f.s.eventWorkerFlowFor(run, event))
	endedAt := *f.s.executionRecord(run).TurnEndedAt
	require(t, f.s.eventWorkerFlowFor(run, event))
	if !f.s.executionRecord(run).TurnEndedAt.Equal(endedAt) {
		t.Fatal("duplicate late completion changed its first end time")
	}
	require(t, f.s.finishWorkerFlowFor(run, nil))
	require(t, f.s.pollNativePRs(ctx, run.RunID))
	wrong := event
	wrong.Fact.SessionID = "foreign-session"
	if err = f.s.eventWorkerFlowFor(run, wrong); !errors.Is(err, m.ErrTurnStale) {
		t.Fatal("foreign late fact accepted", err)
	}
	var commandsAfter int
	require(t, f.store.db.QueryRow(`SELECT COUNT(*) FROM commands`).Scan(&commandsAfter))
	if calls.Load() != 0 || len(f.s.domain.Attempts) != beforeAttempts || commandsAfter != beforeCommands || digestValue(*f.s.workflowRecord(run)) != terminal {
		t.Fatal("late facts revived/recovered/reported the stopped candidate")
	}
	e = f.s.executionRecord(run)
	if identity != digestValue(struct {
		Workspace m.WorkspaceEvidence
		Process   *b.ProcessIdentity
		Session   string
	}{e.Workspace, e.Process, e.SessionID}) || deref(e.Summary) != "late original final" {
		t.Fatal("late output lost its fixed resource/summary")
	}
	if _, err = os.Stat(e.Workspace.Worktree); err != nil {
		t.Fatal("stop removed the original workspace", err)
	}
	// A real delivery closes Task without implicitly ending its siblings.
	second := f.s.find(secondReceipt.WorkflowRunID)
	mcStartTurn(t, f.s, second)
	mcFinishTurn(t, f.s, second)
	f.bind(t, second, 89, "open")
	f.proof(t, second, 89, "merged")
	_, err = f.commands.CompleteNativeTask(ctx, f.c, "8", task.Version, second.RunID, testPublicationClient(f.c))
	require(t, err)
	require(t, f.s.refreshNativeSnapshot(ctx))
	closedTask, delivery := digestValue(nativeTaskRecord(f.s.domain.Tasks, "8")), digestValue(f.s.domain.Deliveries)
	req, payload, receipt = stopTestRequest(f.s, "8", thirdReceipt.WorkflowRunID)
	_, err = f.commands.StopCandidate(ctx, req, payload, receipt)
	require(t, err)
	require(t, f.s.refreshNativeSnapshot(ctx))
	if closedTask != digestValue(nativeTaskRecord(f.s.domain.Tasks, "8")) || delivery != digestValue(f.s.domain.Deliveries) {
		t.Fatal("stop on closed Task changed delivery/task facts")
	}
	require(t, f.s.Close())
	restored, err := newNeutralService(f.c)
	require(t, err)
	defer restored.Close()
	var resumed atomic.Int32
	restored.lifecycleResolver = stopCountResolver{target: run.RunID, calls: &resumed}
	require(t, restored.resumeWorkerFlow(ctx))
	if resumed.Load() != 0 || restored.find(run.RunID).WorkflowStatus != "abandoned" {
		t.Fatal("restart reconnected a stopped candidate")
	}
}

func stopTestRequest(s *Service, taskID, runID string) (api.ControlRequest, api.ControlPayload, api.ControlReceipt) {
	at := now()
	payload := api.ControlPayload{TaskID: taskID, RunID: runID, ExpectedRunRevision: s.find(runID).ControlRevision}
	req := api.ControlRequest{ProtocolVersion: 1, RequestID: newID(), RepositoryIdentity: s.Config.identity,
		InstanceID: s.State.Service.InstanceID, Action: "stop_candidate", ExpectedControlRevision: s.State.Control.Revision,
		Payload: mustJSON(payload), RequestedBy: "operator", CreatedAt: at, ExpiresAt: at.Add(2 * time.Minute)}
	r := s.baseReceipt(req, RequestDigest(req), "applied", "applied", "resources retained")
	r.RunID, r.CompletedAt, r.AcceptedAt = runID, &at, &at
	return req, payload, r
}

func TestCandidateStopTransactions(t *testing.T) {
	s, _ := newTaskStartFixture(t)
	ids := reserveStopTestCandidates(t, s, 5)
	ctx := context.Background()
	store := testSQLite(s)
	defer store.db.Close()
	commands := s.commandService(s.Config.identity)
	req, payload, receipt := stopTestRequest(s, "101", ids[0])
	before := digestValue(s.domain.Executions)
	// Abort the last write: neither workflow stop nor command may escape.
	_, err := store.db.Exec(`CREATE TRIGGER fail_candidate_stop BEFORE INSERT ON events WHEN NEW.type='candidate.stopped' BEGIN SELECT RAISE(ABORT,'injected stop event failure'); END`)
	require(t, err)
	if _, err = commands.StopCandidate(ctx, req, payload, receipt); err == nil {
		t.Fatal("stop escaped the failed transaction")
	}
	_, found, err := s.commandStore.LookupCommand(ctx, s.Config.identity, req.RequestID)
	require(t, err)
	require(t, s.refreshNativeSnapshot(ctx))
	if found || s.find(ids[0]).WorkflowStatus == "abandoned" {
		t.Fatal("failed stop left partial state")
	}
	_, err = store.db.Exec(`DROP TRIGGER fail_candidate_stop`)
	require(t, err)
	// Stop eligibility is independent of valid Task states; generic database
	// integrity still applies. Closed Task coverage uses real delivery below.
	var original []byte
	require(t, store.db.QueryRow(`SELECT payload FROM tasks WHERE project_identity=? AND task_id='101'`, s.Config.identity).Scan(&original))
	for i, taskStatus := range []string{"discarded", "in_progress", "in_progress"} {
		task := *nativeTaskRecord(s.domain.Tasks, "101")
		task.Status = taskStatus
		if taskStatus == "discarded" {
			task.DiscardedAt = ptr(now())
			task.DiscardReason = "test"
			task.DiscardedBy = "operator"
			task.DiscardRequestID = newID()
		}
		body := mustJSON(task)
		_, err = store.db.Exec(`UPDATE tasks SET status=?,payload=? WHERE project_identity=? AND task_id='101'`, taskStatus, body, s.Config.identity)
		require(t, err)
		if i != 0 {
			req, payload, receipt = stopTestRequest(s, "101", ids[i])
		}
		result, err := commands.StopCandidate(ctx, req, payload, receipt)
		require(t, err)
		if result.Status != "applied" || result.RunID != ids[i] {
			t.Fatal("stop did not retain its original target")
		}
		var after []byte
		require(t, store.db.QueryRow(`SELECT payload FROM tasks WHERE project_identity=? AND task_id='101'`, s.Config.identity).Scan(&after))
		if string(after) != string(body) {
			t.Fatal("stop modified the Task")
		}
		replayed, err := commands.StopCandidate(ctx, req, payload, receipt)
		require(t, err)
		if digestValue(result) != digestValue(replayed) {
			t.Fatal("replay changed the committed result")
		}
		changed := req
		changed.RequestedBy = "other"
		if _, err = commands.StopCandidate(ctx, changed, payload, receipt); !isNativeErrorCode(err, "idempotency_conflict") {
			t.Fatal("changed original stop was replayed", err)
		}
	}
	_, err = store.db.Exec(`UPDATE tasks SET status='in_progress',payload=? WHERE project_identity=? AND task_id='101'`, original, s.Config.identity)
	require(t, err)
	require(t, s.refreshNativeSnapshot(ctx))
	if digestValue(s.domain.Executions) != before {
		t.Fatal("stop modified execution/resource facts")
	}
	// Two writers compete for the final two candidates; only one can stop.
	reqA, pA, rA := stopTestRequest(s, "101", ids[3])
	reqB, pB, rB := stopTestRequest(s, "101", ids[4])
	results := make(chan error, 2)
	for _, in := range []struct {
		req     api.ControlRequest
		payload api.ControlPayload
		receipt api.ControlReceipt
	}{{reqA, pA, rA}, {reqB, pB, rB}} {
		go func(in struct {
			req     api.ControlRequest
			payload api.ControlPayload
			receipt api.ControlReceipt
		}) {
			_, err := commands.StopCandidate(ctx, in.req, in.payload, in.receipt)
			results <- err
		}(in)
	}
	rejected := 0
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			if !isNativeErrorCode(err, "last_candidate") {
				t.Fatal(err)
			}
			rejected++
		}
	}
	require(t, s.refreshNativeSnapshot(ctx))
	if rejected != 1 || participatingCandidates(s.domain.WorkflowRuns, "101") != 1 {
		t.Fatal("concurrent stops removed the last participant")
	}
	for _, w := range s.domain.WorkflowRuns {
		if m.CandidateParticipates(w.Status) {
			require(t, s.failReservedStart(s.find(w.WorkflowRunID), "definite startup failure"))
		}
	}
	if participatingCandidates(s.domain.WorkflowRuns, "101") != 0 {
		t.Fatal("automatic startup failure retained a participant")
	}
	newIDs := reserveStopTestCandidates(t, s, 1)
	if s.workflowRecord(s.find(newIDs[0])).CandidateNumber != 6 {
		t.Fatal("stopped history reused a number")
	}
	// No-base stopped history loads after restart without resource recovery.
	require(t, s.Config.ValidateStoredExecutions(s.domain, s.runtime.Directory))
	if _, err := m.CandidateCommonBase(s.domain.WorkflowRuns, s.domain.Executions, "101"); !isNativeErrorCode(err, "task_not_ready") {
		t.Fatal("no-base stopped history blocked the initial baseline", err)
	}
}

func TestCandidateStopControlReplayAndLatePreparation(t *testing.T) {
	s, c := newTaskStartFixture(t)
	ids := reserveStopTestCandidates(t, s, 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runContext, s.signalQueue = ctx, make(chan Signal, 4)
	port := &creationCheckpointPort{}
	input := b.LifecycleInput{Workspace: &b.WorkspaceIntent{Path: t.TempDir(), Branch: "candidate-test", BaseCommit: strings.Repeat("c", 40)}}
	err := s.executeLifecycleStep(ctx, s.find(ids[0]), port, b.LifecycleCandidate{Action: b.LifecyclePrepareWorkspace, Input: input})
	if !errors.Is(err, errLifecycleEffectPending) {
		t.Fatal("original action did not reach dispatch", err)
	}
	var inFlight Signal
	select {
	case inFlight = <-s.signalQueue:
	case <-time.After(10 * time.Second):
		t.Fatal("original receipt did not return")
	}
	s.State.Control.DispatchMode, s.State.Control.DispatchPaused = "auto", true
	req, payload, _ := stopTestRequest(s, "101", ids[0])
	require(t, publishControlRequest(c.RunDir, req))
	require(t, s.processCandidateStop(ctx, req, payload, s.host.ControlRequestDigest(req.RequestID)))
	r, err := ReadControlReceipt(c, req.RequestID)
	require(t, err)
	if r.Status != "applied" || r.RunID != ids[0] {
		t.Fatal("auto/paused mode blocked stop", r)
	}
	require(t, s.HandleSignal(inFlight))
	if port.executes != 1 || port.reconciles != 0 {
		t.Fatal("stopped in-flight mutation initiated reconciliation", port)
	}
	if err = s.executeLifecycleStep(ctx, s.find(ids[0]), port, b.LifecycleCandidate{Action: b.LifecyclePrepareWorkspace, Input: input}); !isNativeErrorCode(err, "candidate_inactive") {
		t.Fatal("stopped candidate authorized a new lifecycle step", err)
	}
	first := digestValue(*s.workflowRecord(s.find(ids[0])))
	original := api.ControlRequest{RequestID: ids[0]}
	s.startChecks = map[string]api.ControlRequest{ids[0]: original}
	require(t, s.applyTaskStartCheck(taskStartCheckResult{request: original, err: errors.New("late failure")}))
	if digestValue(*s.workflowRecord(s.find(ids[0]))) != first {
		t.Fatal("late preparation changed terminal facts")
	}
	replayed, err := s.replayTaskControl(ctx, req, s.host.ControlRequestDigest(req.RequestID))
	require(t, err)
	if !replayed || len(s.domain.WorkflowRuns) != 3 {
		t.Fatal("recovery did not use the original stop result")
	}
	bad := req
	bad.Payload = mustJSON(api.ControlPayload{TaskID: "105", RunID: ids[1], ExpectedRunRevision: s.find(ids[1]).ControlRevision})
	replayed, err = s.replayTaskControl(ctx, bad, RequestDigest(bad))
	require(t, err)
	if !replayed || s.find(ids[1]).WorkflowStatus == "abandoned" {
		t.Fatal("conflicting replay changed another candidate")
	}
	// Reconciliation already dispatched before stop may return unknown. Its
	// original creation checkpoint must still reach the fact-only journal.
	port = &creationCheckpointPort{checkpoint: true, unavailable: true}
	err = s.executeLifecycleStep(ctx, s.find(ids[1]), port, b.LifecycleCandidate{Action: b.LifecyclePrepareWorkspace, Input: input})
	if !errors.Is(err, errLifecycleEffectPending) {
		t.Fatal(err)
	}
	select {
	case inFlight = <-s.signalQueue:
	case <-time.After(10 * time.Second):
		t.Fatal("initial checkpoint did not return")
	}
	require(t, s.HandleSignal(inFlight))
	select {
	case inFlight = <-s.signalQueue:
	case <-time.After(10 * time.Second):
		t.Fatal("reconciliation did not return")
	}
	req, payload, _ = stopTestRequest(s, "101", ids[1])
	require(t, publishControlRequest(c.RunDir, req))
	require(t, s.processCandidateStop(ctx, req, payload, s.host.ControlRequestDigest(req.RequestID)))
	terminal := digestValue(*s.workflowRecord(s.find(ids[1])))
	require(t, s.HandleSignal(inFlight))
	op := s.operationByKey(inFlight.effect.request.Operation.IdempotencyKey)
	if op == nil {
		t.Fatal("original operation disappeared")
	}
	var saved b.LifecycleReceipt
	require(t, json.Unmarshal(op.Receipt, &saved))
	if string(saved.Operation.Receipt) != `{"original_workspace":"instance-1"}` || !strings.Contains(op.UnknownReason, "terminal query unavailable") || !strings.Contains(op.UnknownReason, "reconciliation unavailable") || port.reconciles != 1 || port.executes != 1 || digestValue(*s.workflowRecord(s.find(ids[1]))) != terminal {
		t.Fatalf("late reconciliation lost original facts or advanced the stopped candidate: checkpoint=%q diagnostic=%q calls=%+v", saved.Operation.Receipt, op.UnknownReason, port)
	}
}
