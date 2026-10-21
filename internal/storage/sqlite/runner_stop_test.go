package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

func candidateStopFixture(t *testing.T) (*SQLiteStore, string, func(string, bool) runnerbridge.Candidate) {
	t.Helper()
	ctx := context.Background()
	s, project, tasks := nativeAuditStore(t)
	require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error {
		_, err := tx.tx.ExecContext(ctx, `UPDATE project_task_authorities SET dispatch_paused=0 WHERE project_identity=?`, project)
		return err
	}))
	return s, project, func(id string, extra bool) runnerbridge.Candidate {
		t.Helper()
		graph, err := s.TaskGraphSnapshot(ctx, project)
		require(t, err)
		record := graph.ByID[tasks[0].TaskID]
		contract, err := s.TaskContract(ctx, project, record)
		require(t, err)
		input := runnerclient.RunInput{Title: record.Title, Repository: runnerclient.Repository{URL: contract.Repository, Remote: contract.Remote, BaseBranch: contract.Branch, BaseRevision: strings.Repeat("a", 40)}, Documents: runnerclient.Documents{Spec: runnerclient.Document{Path: contract.SpecRef.Path}, Plan: runnerclient.Document{Path: contract.PlanRef.Path}}, Publication: runnerclient.Publication{Provider: "gitea", Repository: "Org/Repo", BaseBranch: contract.Branch, Settings: json.RawMessage(`{"base_url":"https://example.invalid"}`)}}
		raw, _ := json.Marshal(input)
		c := runnerbridge.Candidate{Project: project, TaskID: record.TaskID, ID: id, Revision: 1, Explicit: true, StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "fixture", ContractDigest: contract.Digest, BaseRevision: input.Repository.BaseRevision, Participation: "active", SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "submit-" + id, Input: raw}}
		require(t, s.ReserveRunnerCandidate(ctx, taskmodel.FixedWork{Record: record, Contract: contract, Authority: graph.TaskGraph().Authority}, c, extra))
		c, err = s.Candidate(ctx, project, id)
		require(t, err)
		return c
	}
}

func TestRunnerCandidateStopAtomicityAndReplay(t *testing.T) {
	ctx := context.Background()
	s, project, reserve := candidateStopFixture(t)
	first := reserve("first", false)
	for _, id := range []string{"second", "third", "fourth", "fifth"} {
		reserve(id, true)
	}
	first.SubmitState = "unknown"
	first.Revision++
	first.Actions = []runnerbridge.Action{{Command: "reply", State: "pending"}, {Command: "resume", State: "unknown"}}
	require(t, s.SaveCandidate(ctx, first, 1, nil))
	before, err := s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	// Dispatch mode/pause cannot restrict withdrawal, and rollback cannot release a slot.
	_, err = s.db.Exec(`UPDATE project_task_authorities SET dispatch_mode='auto',dispatch_paused=1; CREATE TRIGGER fail_withdrawal BEFORE UPDATE ON runner_candidates BEGIN SELECT RAISE(ABORT,'withdrawal fault'); END`)
	require(t, err)
	if _, err := s.StopRunnerCandidate(ctx, project, first.ID, first.Revision, "stop-first", "operator"); err == nil {
		t.Fatal("injected withdrawal failure ignored")
	}
	current, err := s.Candidate(ctx, project, first.ID)
	require(t, err)
	if digestValue(current) != digestValue(first) {
		t.Fatal("partial withdrawal after rollback")
	}
	_, err = s.db.Exec(`DROP TRIGGER fail_withdrawal`)
	require(t, err)
	stopped, err := s.StopRunnerCandidate(ctx, project, first.ID, first.Revision, "stop-first", "operator")
	require(t, err)
	if stopped.Participation != "abandoned" || stopped.StopRequested || stopped.InputsRevoked || stopped.Withdrawal == nil || stopped.Withdrawal.At.IsZero() || stopped.Withdrawal.RequestedBy != "operator" || stopped.SubmitState != "unknown" || stopped.Actions[0].State != "cancelled" || stopped.Actions[1].State != "unknown" {
		t.Fatal("withdrawal changed resource facts or lost original intent", stopped)
	}
	again, err := s.StopRunnerCandidate(ctx, project, first.ID, first.Revision, "stop-first", "operator")
	require(t, err)
	if digestValue(again) != digestValue(stopped) {
		t.Fatal("replay rewrote fixed stop facts")
	}
	for _, change := range []struct {
		version        uint64
		request, actor string
	}{{first.Revision + 1, "stop-first", "operator"}, {first.Revision, "stop-first", "other"}, {stopped.Revision, "another-stop", "operator"}} {
		if _, err := s.StopRunnerCandidate(ctx, project, first.ID, change.version, change.request, change.actor); err == nil {
			t.Fatal("changed request or terminal candidate accepted")
		}
	}
	after, err := s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	if digestValue(before.Tasks) != digestValue(after.Tasks) || before.Authority.GraphRevision != after.Authority.GraphRevision || len(after.Deliveries) != 0 {
		t.Fatal("candidate withdrawal changed Task/DAG/delivery")
	}
	_, err = s.db.Exec(`UPDATE project_task_authorities SET dispatch_mode='manual',dispatch_paused=0`)
	require(t, err)
	next := reserve("sixth", true)
	if next.Number != 6 || next.BaseRevision != stopped.BaseRevision {
		t.Fatal("slot not released or candidate history/base renumbered")
	}
	stopped.Revision++
	stopped.Withdrawal.RequestedBy = "rewritten"
	if s.SaveCandidate(ctx, stopped, stopped.Revision-1, nil) == nil {
		t.Fatal("late projection overwrote withdrawal fact")
	}
}

func TestRunnerCandidateStopConcurrentLastParticipant(t *testing.T) {
	ctx := context.Background()
	s, project, reserve := candidateStopFixture(t)
	first, second := reserve("first", false), reserve("second", true)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, c := range []runnerbridge.Candidate{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.StopRunnerCandidate(ctx, project, c.ID, c.Revision, "stop-"+c.ID, "operator")
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
			continue
		}
		var domain *taskmodel.Error
		if !errors.As(err, &domain) || domain.Code != "last_candidate" {
			t.Fatal("unexpected concurrent rejection", err)
		}
	}
	if success != 1 {
		t.Fatal("last participant protection failed", success)
	}
}

func TestRunnerCandidateStopDeliveryRace(t *testing.T) {
	ctx := context.Background()
	s, project, reserve := candidateStopFixture(t)
	c := reserve("selected", false)
	reserve("sibling", true)
	at := time.Now().UTC()
	merge := strings.Repeat("b", 40)
	c.RunID, c.InputDigest, c.Branch, c.PRURL, c.SubmitState = "run", "input", "head", "https://example.invalid/Org/Repo/pulls/84", "accepted"
	c.Projection = &runnerclient.Run{RunID: c.RunID, Revision: 8, InputDigest: c.InputDigest, Status: "running", Execution: runnerclient.Execution{Branch: c.Branch}, PR: &runnerclient.PRFact{Provider: "gitea", Repository: "Org/Repo", HeadRepository: "Org/Repo", URL: c.PRURL, Number: 84, BaseBranch: "main", HeadBranch: c.Branch, State: "merged", Merged: true, MergedAt: &at, MergeCommit: &merge, CheckedAt: at}}
	c.Revision++
	require(t, s.SaveCandidate(ctx, c, 1, nil))
	graph, err := s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	input := RunnerResult{Project: project, TaskID: c.TaskID, CandidateID: c.ID, TaskVersion: graph.ByID[c.TaskID].Version, CandidateRevision: c.Revision, AuthorityVersion: graph.Authority.AuthorityVersion, GraphRevision: graph.Authority.GraphRevision, VerificationStarted: at.Add(-time.Second)}
	start := make(chan struct{})
	stopResult, deliveryResult := make(chan error, 1), make(chan error, 1)
	go func() {
		<-start
		_, err := s.StopRunnerCandidate(ctx, project, c.ID, c.Revision, "stop-selected", "operator")
		stopResult <- err
	}()
	go func() { <-start; _, err := s.CommitRunnerTaskResult(ctx, input); deliveryResult <- err }()
	close(start)
	stopErr, deliveryErr := <-stopResult, <-deliveryResult
	if (stopErr == nil) == (deliveryErr == nil) {
		t.Fatal("stop/delivery must have exactly one winner", stopErr, deliveryErr)
	}
	current, err := s.Candidate(ctx, project, c.ID)
	require(t, err)
	after, err := s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	if stopErr == nil {
		if current.Participation != "abandoned" || len(after.Deliveries) != 0 {
			t.Fatal("withdrawn candidate was delivered")
		}
		input.CandidateRevision = current.Revision
		if _, err := s.CommitRunnerTaskResult(ctx, input); err == nil {
			t.Fatal("fresh version bypassed withdrawal gate")
		}
	} else if current.Participation != "completed" || current.Withdrawal != nil || len(after.Deliveries) != 1 {
		t.Fatal("delivery winner was rewritten as withdrawal")
	}
}

func TestRunnerControlStopRecoversOnlyOriginalWithdrawal(t *testing.T) {
	ctx := context.Background()
	s, project, reserve := candidateStopFixture(t)
	first := reserve(strings.Repeat("b", 32), false)
	reserve("second", true)
	svc := &taskrunner.Service{Project: project, Store: s, Bridge: &runnerbridge.Bridge{Store: s}}
	controls := taskrunner.Controls{Tasks: svc, Store: s, ResolveProfile: func(string, string) (string, error) { t.Fatal("withdrawal consulted startup profile"); return "", nil }}
	graph, err := s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	payload, _ := json.Marshal(api.ControlPayload{TaskID: first.TaskID, RunID: first.ID, ExpectedRunRevision: first.Revision})
	at := time.Now().UTC()
	req := api.ControlRequest{ProtocolVersion: 1, RequestID: strings.Repeat("e", 32), RepositoryIdentity: project, InstanceID: "original", Action: "stop_candidate", ExpectedControlRevision: graph.Authority.AuthorityVersion, RequestedBy: "operator", CreatedAt: at, ExpiresAt: at.Add(time.Minute), Payload: payload}
	_, err = s.db.Exec(`CREATE TRIGGER lose_stop_receipt BEFORE UPDATE ON commands WHEN NEW.kind='runner_control' BEGIN SELECT RAISE(ABORT,'receipt fault'); END`)
	require(t, err)
	if receipt, err := controls.Handle(ctx, req, "original", graph.Authority.AuthorityVersion); err == nil || receipt.Status != "applied" {
		t.Fatal("fixture did not lose receipt after committed stop", receipt, err)
	}
	_, err = s.db.Exec(`DROP TRIGGER lose_stop_receipt`)
	require(t, err)
	receipt, err := controls.Handle(ctx, req, "replacement", 99)
	require(t, err)
	if receipt.Status != "applied" || receipt.RunID != first.ID {
		t.Fatal("original withdrawal was not recovered", receipt)
	}
	changed := req
	changed.RequestedBy = "different"
	conflict, err := controls.Handle(ctx, changed, "replacement", 99)
	require(t, err)
	if conflict.Code != "idempotency_conflict" {
		t.Fatal("original request identity changed", conflict)
	}
	// A distinct request cannot claim an unrelated manual or Task stop as its effect.
	req.RequestID = strings.Repeat("f", 32)
	body, err := json.Marshal(req)
	require(t, err)
	_, _, err = s.EnqueueCommand(ctx, storageports.CommandRecord{ProjectIdentity: project, CommandID: req.RequestID, Kind: "runner_control", TargetID: project, ExpectedVersion: req.ExpectedControlRevision, IdempotencyKey: req.RequestID, RequestedBy: req.RequestedBy, Payload: body, Status: "pending", CreatedAt: at})
	require(t, err)
	claimed := graph.ByID[first.TaskID]
	discarded, err := taskmodel.Discard(claimed, "Task ends while another request is pending", "operator", strings.Repeat("a", 32), nil, time.Now().UTC())
	require(t, err)
	require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.UpdateNativeTaskCAS(discarded, claimed.Version) }))
	denied, err := controls.Handle(ctx, req, "replacement", 99)
	require(t, err)
	if denied.Status != "rejected" || denied.Code != "stale_instance" {
		t.Fatal("different request inferred success from terminal candidate", denied)
	}
}
