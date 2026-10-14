package execution

import (
	"context"
	"errors"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

func additionalStartRequest(t *testing.T, s *Service, taskID string) api.ControlRequest {
	t.Helper()
	graph, err := s.commandStore.TaskGraphSnapshot(context.Background(), s.Config.identity)
	require(t, err)
	_, profile, err := s.runtime.Profiles.ResolveExecutionProfile("second", "")
	require(t, err)
	at := now()
	payload := api.ControlPayload{TaskID: taskID, ExpectedTaskVersion: graph.ByID[taskID].Version, ExpectedGraphRevision: graph.Authority.GraphRevision, ExecutionProfileID: profile.ID, ExecutionProfileDigest: profile.Digest}
	req := api.ControlRequest{ProtocolVersion: ControlProtocolVersion, RequestID: newID(), RepositoryIdentity: s.Config.identity, InstanceID: s.State.Service.InstanceID, ExpectedControlRevision: s.State.Control.Revision, Action: "start_candidate", Payload: mustJSON(payload), RequestedBy: "operator", CreatedAt: at, ExpiresAt: at.Add(2 * time.Minute)}
	require(t, publishControlRequest(s.Config.RunDir, req))
	return req
}

func TestCandidateStartReservationsRetainFailuresAndBoundParticipation(t *testing.T) {
	s, c := newTaskStartFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	s.runContext, s.signalQueue = ctx, make(chan Signal, 16)
	t.Cleanup(func() { cancel(); s.shutdown() })
	gate := &startGate{entered: make(chan struct{}), release: make(chan struct{}), failure: errors.New("startup environment unavailable")}
	s.taskWork = slowStartWork{TaskWorkPort: s.taskWork, gate: gate}
	first := startControlRequest(t, s, "101", "first")
	require(t, s.pollControl(ctx, cancel))
	select {
	case <-gate.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("preparation did not start")
	}
	if r := s.find(first.RequestID); r == nil || s.workflowRecord(r).CandidateNumber != 1 || !s.workflowRecord(r).StartPending {
		t.Fatal("identity was not persisted before the external check")
	}
	for i := 2; i <= 5; i++ {
		req := additionalStartRequest(t, s, "101")
		require(t, s.pollControl(ctx, cancel))
		if r := s.find(req.RequestID); r == nil || s.workflowRecord(r).CandidateNumber != i {
			t.Fatalf("candidate %d could not be admitted while first check was pending", i)
		}
	}
	rejected := additionalStartRequest(t, s, "101")
	require(t, s.pollControl(ctx, cancel))
	r, err := ReadControlReceipt(c, rejected.RequestID)
	require(t, err)
	if r.Status != "rejected" || len(s.domain.WorkflowRuns) != 5 {
		t.Fatal("sixth participating candidate was admitted", r)
	}
	close(gate.release)
	select {
	case message := <-s.signalQueue:
		require(t, s.HandleSignal(message))
	case <-time.After(10 * time.Second):
		t.Fatal("failed check did not return")
	}
	firstRun := s.find(first.RequestID)
	if firstRun == nil || firstRun.WorkflowStage != "start.failed" || firstRun.WorkflowStatus != "abandoned" || firstRun.CandidateEndedAt == nil || participatingCandidates(s.domain.WorkflowRuns, "101") != 4 {
		t.Fatal("failed start was not retained as terminal history")
	}
	next := additionalStartRequest(t, s, "101")
	require(t, s.pollControl(ctx, cancel))
	if run := s.find(next.RequestID); run == nil || s.workflowRecord(run).CandidateNumber != 6 || len(s.domain.WorkflowRuns) != 6 {
		t.Fatal("failed history reused a number or retained its slot")
	}
	// Replaying the original request preserves the original failed workflow.
	replayed, err := s.replayTaskControl(ctx, first, s.host.ControlRequestDigest(first.RequestID))
	require(t, err)
	if !replayed {
		t.Fatal("original committed start was not replayed")
	}
	require(t, s.pollTaskControlReceipts(ctx))
	require(t, s.pollControl(ctx, cancel))
	if len(s.domain.Executions) != 6 || len(s.domain.Attempts) != 6 {
		t.Fatal("request reconciliation created replacement identities")
	}
}

func TestCandidateReservationConcurrentLimit(t *testing.T) {
	s, _ := newTaskStartFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := startControlRequest(t, s, "101", "first")
	require(t, s.pollControl(ctx, cancel))
	if s.find(first.RequestID) == nil {
		t.Fatal("initial reservation missing")
	}
	commands := s.commandService(s.Config.identity)
	config, profile, err := s.runtime.Profiles.ResolveExecutionProfile("second", "")
	require(t, err)
	graph, err := s.commandStore.TaskGraphSnapshot(ctx, s.Config.identity)
	require(t, err)
	prepare := func() preparedTaskClaim {
		prepared, err := commands.prepareNativeTaskClaim(ctx, config, TaskClaimOptions{TaskID: "101", RequestID: newID(), WorkflowRunID: newID(), ExpectedTaskVersion: graph.ByID["101"].Version, ExpectedAuthorityVersion: graph.Authority.AuthorityVersion, ExpectedGraphRevision: graph.Authority.GraphRevision, RequestedBy: "operator", Explicit: true, ExecutionProfile: &profile, ServiceInstanceID: s.State.Service.InstanceID, startCandidate: true, reserveOnly: true})
		require(t, err)
		return prepared
	}
	for i := 0; i < 3; i++ {
		_, err := commands.commitPreparedTaskClaim(ctx, prepare())
		require(t, err)
	}
	// Both requests sample four participants. Only one may commit the fifth.
	a, b := prepare(), prepare()
	results := make(chan error, 2)
	for _, prepared := range []preparedTaskClaim{a, b} {
		go func(prepared preparedTaskClaim) {
			_, err := commands.commitPreparedTaskClaim(ctx, prepared)
			results <- err
		}(prepared)
	}
	rejected := 0
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			if !isNativeErrorCode(err, "candidate_limit_reached") {
				t.Fatal(err)
			}
			rejected++
		}
	}
	require(t, s.refreshNativeSnapshot(ctx))
	if rejected != 1 || len(s.domain.WorkflowRuns) != 5 || participatingCandidates(s.domain.WorkflowRuns, "101") != 5 {
		t.Fatal("concurrent transactions exceeded five participants")
	}
	numbers := map[int]bool{}
	for _, w := range s.domain.WorkflowRuns {
		if w.CandidateNumber == 0 || numbers[w.CandidateNumber] {
			t.Fatal("concurrent admission reused a number")
		}
		numbers[w.CandidateNumber] = true
	}
}
