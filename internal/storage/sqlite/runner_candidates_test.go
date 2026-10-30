package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestRunnerCandidateReadErrors(t *testing.T) {
	s := openDomainTestStore(t)
	if _, err := s.Candidate(context.Background(), "project", "missing"); !errors.Is(err, storageports.ErrNotFound) {
		t.Fatalf("absent candidate must use the storage contract: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Candidate(ctx, "project", "missing"); !errors.Is(err, context.Canceled) || errors.Is(err, storageports.ErrNotFound) {
		t.Fatalf("cancellation was misclassified as an absent candidate: %v", err)
	}
	require(t, s.Close())
	if _, err := s.Candidate(context.Background(), "project", "missing"); err == nil || errors.Is(err, storageports.ErrNotFound) {
		t.Fatalf("closed storage was misclassified as an absent candidate: %v", err)
	}
}

func TestRunnerCandidateReservationAndProjection(t *testing.T) {
	ctx := context.Background()
	s, project, tasks := nativeAuditStore(t)
	require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error {
		_, err := tx.tx.ExecContext(ctx, `UPDATE project_task_authorities SET dispatch_paused=0 WHERE project_identity=?`, project)
		return err
	}))
	graph, err := s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	contract, err := s.TaskContract(ctx, project, tasks[0])
	require(t, err)
	work := taskmodel.FixedWork{Record: tasks[0], Contract: contract, Authority: graph.TaskGraph().Authority}
	fixed := runnerclient.RunInput{Title: work.Record.Title, Repository: runnerclient.Repository{URL: contract.Repository, Remote: contract.Remote, BaseBranch: contract.Branch, BaseRevision: strings.Repeat("a", 40)}, Documents: runnerclient.Documents{Spec: runnerclient.Document{Path: contract.SpecRef.Path}, Plan: runnerclient.Document{Path: contract.PlanRef.Path}}, Publication: runnerclient.Publication{BaseBranch: contract.Branch}}
	payload, _ := json.Marshal(fixed)
	c := runnerbridge.Candidate{Project: project, TaskID: tasks[0].TaskID, ID: "candidate", Revision: 1, StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "fixture", ContractDigest: contract.Digest, BaseRevision: strings.Repeat("a", 40), Participation: "active", SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "original-request", Input: payload}}
	c.Explicit = true
	require(t, reserveFixtureCandidate(t, s, ctx, work, c, false))
	// The original request remains replayable after its task version advances.
	require(t, reserveFixtureCandidate(t, s, ctx, work, c, false))
	graph, err = s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	if graph.ByID[c.TaskID].Status != "in_progress" || graph.ByID[c.TaskID].Claimant != "fixture" {
		t.Fatal("candidate and claim did not commit together")
	}
	saved, err := s.Candidate(ctx, project, c.ID)
	require(t, err)
	var firstInput runnerclient.RunInput
	require(t, json.Unmarshal(saved.Submit.Input, &firstInput))
	if saved.Number != 1 || firstInput.NamePrefix != "sl-"+c.TaskID {
		t.Fatal("candidate number and submit prefix were not frozen together", saved, firstInput)
	}
	second := c
	second.ID, second.Submit.RequestID = "candidate-two", "second-request"
	claimedWork := work
	claimedWork.Record, claimedWork.Authority = graph.ByID[c.TaskID], graph.TaskGraph().Authority
	require(t, reserveFixtureCandidate(t, s, ctx, claimedWork, second, true))
	second, err = s.Candidate(ctx, project, second.ID)
	require(t, err)
	var secondInput runnerclient.RunInput
	require(t, json.Unmarshal(second.Submit.Input, &secondInput))
	if second.Number != 2 || secondInput.NamePrefix != "sl-"+c.TaskID+"-c2" {
		t.Fatal("second candidate used an ID or stale number for its name", second, secondInput)
	}
	// Replay after another allocation must reuse the first number and exact bytes.
	require(t, reserveFixtureCandidate(t, s, ctx, work, c, false))
	replayed, err := s.Candidate(ctx, project, c.ID)
	require(t, err)
	if digestValue(saved) != digestValue(replayed) {
		t.Fatal("reservation replay changed the frozen submission")
	}
	changedName := saved
	changedName.Revision++
	firstInput.NamePrefix = "sl-other-c1"
	changedName.Submit.Input, err = json.Marshal(firstInput)
	require(t, err)
	if s.SaveCandidate(ctx, changedName, saved.Revision, nil) == nil {
		t.Fatal("saved submission name was mutable")
	}
	saved.Revision++
	saved.RunID = "run"
	saved.SubmitState = "accepted"
	saved.Cursor = "cursor"
	saved.InputDigest = "input"
	saved.Actions = []runnerbridge.Action{{CallerRequestID: "product-request", CallerRequestDigest: "product-digest", Command: "reply", Request: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "wire-request", Input: json.RawMessage(`{"expected_revision":1}`)}, State: "pending"}}
	event := runnerclient.Event{EventID: "event", RunID: "run", Revision: 1, Type: "accepted"}
	require(t, s.SaveCandidate(ctx, saved, 1, []runnerclient.Event{event}))
	if err := s.SaveCandidate(ctx, c, 1, nil); err == nil {
		t.Fatal("stale candidate overwrote accepted association")
	}
	changed := saved
	changed.Revision++
	changed.RunID = "another"
	if err := s.SaveCandidate(ctx, changed, 2, nil); err == nil {
		t.Fatal("Run association was replaced")
	}
	changed = saved
	changed.Revision++
	changed.Cursor = "should-rollback"
	bad := event
	bad.Type = "conflicting"
	if err := s.SaveCandidate(ctx, changed, 2, []runnerclient.Event{bad}); err == nil {
		t.Fatal("conflicting duplicate event accepted")
	}
	current, err := s.Candidate(ctx, project, c.ID)
	require(t, err)
	if current.Cursor != "cursor" || current.Revision != 2 {
		t.Fatal("event fault advanced cursor or projection")
	}
	for _, alter := range []func(*runnerbridge.Candidate){
		func(c *runnerbridge.Candidate) { c.Actions = nil },
		func(c *runnerbridge.Candidate) { c.Actions[0].CallerRequestID = "other-product-request" },
		func(c *runnerbridge.Candidate) { c.Actions[0].CallerRequestDigest = "other-product-digest" },
		func(c *runnerbridge.Candidate) { c.Actions[0].Request.RequestID = "other-wire-request" },
		func(c *runnerbridge.Candidate) {
			c.Actions[0].Request.Input = json.RawMessage(`{"expected_revision":2}`)
		},
		func(c *runnerbridge.Candidate) { c.Actions[0].Command = "resume" },
	} {
		bad := current
		bad.Revision++
		bad.Actions = append([]runnerbridge.Action(nil), current.Actions...)
		alter(&bad)
		if s.SaveCandidate(ctx, bad, current.Revision, nil) == nil {
			t.Fatal("saved product-to-Runner action binding changed")
		}
	}
	var count int
	require(t, s.db.QueryRowContext(ctx, `SELECT count(*) FROM runner_events`).Scan(&count))
	if count != 1 {
		t.Fatal("stable events duplicated")
	}
	claimed := graph.ByID[c.TaskID]
	discarded, err := taskmodel.Discard(claimed, "fixture terminal", "fixture", strings.Repeat("c", 32), nil, now())
	require(t, err)
	require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.UpdateNativeTaskCAS(discarded, claimed.Version) }))
	terminal, err := s.Candidate(ctx, project, c.ID)
	require(t, err)
	if !terminal.StopRequested || terminal.InputsRevoked || terminal.Participation != "abandoned" {
		t.Fatal("Task terminal did not atomically preserve pending Run stop", terminal)
	}
	saved.Revision++
	if err := s.SaveCandidate(ctx, saved, 2, nil); err == nil {
		t.Fatal("late CLI response erased terminal stop obligation")
	}
}
