package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestRunnerDiscardRequiresCompleteFreshPRSetAndAtomicStops(t *testing.T) {
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
	input := runnerclient.RunInput{Title: tasks[0].Title, Repository: runnerclient.Repository{URL: contract.Repository, Remote: contract.Remote, BaseBranch: contract.Branch, BaseRevision: strings.Repeat("a", 40)}, Documents: runnerclient.Documents{Spec: runnerclient.Document{Path: contract.SpecRef.Path}, Plan: runnerclient.Document{Path: contract.PlanRef.Path}}, Publication: runnerclient.Publication{Provider: "gitea", Repository: "Org/Repo", BaseBranch: contract.Branch, Settings: json.RawMessage(`{"base_url":"https://example.invalid"}`)}}
	body, _ := json.Marshal(input)
	c := runnerbridge.Candidate{Project: project, TaskID: tasks[0].TaskID, ID: strings.Repeat("d", 32), Revision: 1, Explicit: true, StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "operator", ContractDigest: contract.Digest, BaseRevision: input.Repository.BaseRevision, Participation: "active", SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "original-submit", Input: body}}
	require(t, s.ReserveRunnerCandidate(ctx, taskmodel.FixedWork{Record: tasks[0], Contract: contract, Authority: graph.TaskGraph().Authority}, c, false))
	c, err = s.Candidate(ctx, project, c.ID)
	require(t, err)
	c.SubmitState = "unknown"
	c.Revision++
	require(t, s.SaveCandidate(ctx, c, c.Revision-1, nil))
	graph, err = s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	at := time.Now().UTC()
	commit := ports.RunnerDiscard{Project: project, RequestID: strings.Repeat("e", 32), Intent: taskmodel.DiscardIntent{TaskID: c.TaskID, ExpectedVersion: graph.ByID[c.TaskID].Version, Reason: "operator cancelled", RequestedBy: "operator"}, AuthorityVersion: graph.Authority.AuthorityVersion, GraphRevision: graph.Authority.GraphRevision, CandidateRevisions: map[string]uint64{c.ID: c.Revision}, VerificationStarted: at}
	if _, err := s.CommitRunnerDiscard(ctx, commit); err == nil {
		t.Fatal("unknown original submission was treated as no PR")
	}
	c.RunID = "original-run"
	c.SubmitState = "accepted"
	c.InputDigest = "frozen-input"
	c.Branch = "original-head"
	c.PRURL = "https://example.invalid/Org/Repo/pulls/84"
	c.Projection = &runnerclient.Run{RunID: c.RunID, Revision: 1, InputDigest: c.InputDigest, Execution: runnerclient.Execution{SessionID: "original-session", Workspace: "original-workspace", Branch: c.Branch}, PR: &runnerclient.PRFact{URL: c.PRURL, Provider: "gitea", Repository: "Org/Repo", HeadRepository: "Org/Repo", Number: 84, BaseBranch: contract.Branch, HeadBranch: c.Branch, State: "merged", Merged: true, CheckedAt: at.Add(time.Millisecond)}}
	c.Revision++
	require(t, s.SaveCandidate(ctx, c, c.Revision-1, nil))
	commit.CandidateRevisions[c.ID] = c.Revision
	if _, err := s.CommitRunnerDiscard(ctx, commit); err == nil {
		t.Fatal("merged candidate was discarded")
	}
	c.Projection.PR.Merged = false
	c.Projection.PR.State = "open"
	c.Revision++
	require(t, s.SaveCandidate(ctx, c, c.Revision-1, nil))
	commit.CandidateRevisions[c.ID] = c.Revision
	_, err = s.db.ExecContext(ctx, `CREATE TRIGGER fail_runner_discard_receipt BEFORE UPDATE ON commands WHEN NEW.kind='runner_discard' BEGIN SELECT RAISE(ABORT,'receipt failure'); END`)
	require(t, err)
	if _, err := s.CommitRunnerDiscard(ctx, commit); err == nil {
		t.Fatal("discard ignored receipt failure")
	}
	unchanged, err := s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	old, err := s.Candidate(ctx, project, c.ID)
	require(t, err)
	if unchanged.ByID[c.TaskID].Status != "in_progress" || old.StopRequested || unchanged.Authority.GraphRevision != graph.Authority.GraphRevision {
		t.Fatal("discard state escaped transaction rollback")
	}
	_, err = s.db.ExecContext(ctx, `DROP TRIGGER fail_runner_discard_receipt`)
	require(t, err)
	receipt, err := s.CommitRunnerDiscard(ctx, commit)
	require(t, err)
	if receipt.Status != "discarded" || len(receipt.StopPending) != 1 {
		t.Fatal("discard omitted pending native cleanup", receipt)
	}
	old, err = s.Candidate(ctx, project, c.ID)
	require(t, err)
	if !old.StopRequested || old.InputsRevoked || old.Participation != "abandoned" {
		t.Fatal("discard invented native revocation or lost stop obligation")
	}
	replayed, err := s.CommitRunnerDiscard(ctx, commit)
	require(t, err)
	if digestValue(receipt) != digestValue(replayed) {
		t.Fatal("discard replay changed its original receipt")
	}
}
