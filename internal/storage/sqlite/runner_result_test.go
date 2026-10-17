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
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestRunnerTaskDeliveryWithoutWorkflowReady(t *testing.T) {
	ctx := context.Background()
	s, project, tasks := nativeAuditStore(t)
	require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error {
		if _, err := tx.tx.ExecContext(ctx, `UPDATE project_task_authorities SET dispatch_paused=0 WHERE project_identity=?`, project); err != nil {
			return err
		}
		return tx.PutTaskDependency(taskmodel.TaskDependencyRecord{ProjectIdentity: project, PredecessorID: tasks[0].TaskID, SuccessorID: tasks[1].TaskID})
	}))
	reserve := func(id string, extra bool) runnerbridge.Candidate {
		graph, err := s.TaskGraphSnapshot(ctx, project)
		require(t, err)
		record := graph.ByID[tasks[0].TaskID]
		contract, err := s.TaskContract(ctx, project, record)
		require(t, err)
		input := runnerclient.RunInput{Title: record.Title, Repository: runnerclient.Repository{URL: contract.Repository, Remote: contract.Remote, BaseBranch: contract.Branch, BaseRevision: strings.Repeat("a", 40)}, Documents: runnerclient.Documents{Spec: runnerclient.Document{Kind: "git", Path: contract.SpecRef.Path, Commit: contract.SpecRef.Commit}, Plan: runnerclient.Document{Kind: "git", Path: contract.PlanRef.Path, Commit: contract.PlanRef.Commit}}, Publication: runnerclient.Publication{Provider: "gitea", Repository: "Org/Repo", BaseBranch: contract.Branch, Settings: json.RawMessage(`{"base_url":"https://example.invalid"}`)}}
		raw, _ := json.Marshal(input)
		c := runnerbridge.Candidate{Project: project, TaskID: record.TaskID, ID: id, Revision: 1, Explicit: true, StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "fixture", ContractDigest: contract.Digest, BaseRevision: input.Repository.BaseRevision, Participation: "active", SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "submit-" + id, Input: raw}}
		require(t, s.ReserveRunnerCandidate(ctx, taskmodel.FixedWork{Record: record, Contract: contract, Authority: graph.TaskGraph().Authority}, c, extra))
		current, err := s.Candidate(ctx, project, c.ID)
		require(t, err)
		return current
	}
	selected := reserve(strings.Repeat("c", 32), false)
	sibling := reserve(strings.Repeat("d", 32), true) // Original submission remains unknown/unsent.
	withdrawn := reserve(strings.Repeat("e", 32), true)
	withdrawn, err := s.StopRunnerCandidate(ctx, project, withdrawn.ID, withdrawn.Revision, "manual-withdrawal", "operator")
	require(t, err)
	at := time.Now().UTC()
	merge := strings.Repeat("b", 40)
	selected.RunID, selected.InputDigest, selected.Branch, selected.PRURL, selected.SubmitState = "original-run", "frozen-input", "original-head", "https://example.invalid/Org/Repo/pulls/84", "accepted"
	selected.Projection = &runnerclient.Run{RunID: selected.RunID, Revision: 8, InputDigest: selected.InputDigest, Status: "running", Step: "publish.report", Execution: runnerclient.Execution{SessionID: "original-session", Branch: selected.Branch, Workspace: `C:\owned\workspace`}, PR: &runnerclient.PRFact{Provider: "gitea", Repository: "Org/Repo", HeadRepository: "Org/Repo", URL: selected.PRURL, Number: 84, BaseBranch: "main", HeadBranch: selected.Branch, State: "merged", Merged: true, MergedAt: &at, MergeCommit: &merge, CheckedAt: at}}
	selected.Revision++
	require(t, s.SaveCandidate(ctx, selected, 1, nil))
	graph, err := s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	input := RunnerResult{Project: project, TaskID: selected.TaskID, CandidateID: selected.ID, TaskVersion: graph.ByID[selected.TaskID].Version, CandidateRevision: selected.Revision, AuthorityVersion: graph.Authority.AuthorityVersion, GraphRevision: graph.Authority.GraphRevision, VerificationStarted: at.Add(-time.Second)}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_runner_delivery BEFORE INSERT ON task_results BEGIN SELECT RAISE(ABORT,'injected final commit failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CommitRunnerTaskResult(ctx, input); err == nil {
		t.Fatal("transaction fault was ignored")
	}
	unchanged, err := s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	if unchanged.ByID[selected.TaskID].Status != "in_progress" || len(unchanged.Deliveries) != 0 || unchanged.Authority.GraphRevision != graph.Authority.GraphRevision {
		t.Fatal("partial task delivery after rollback")
	}
	oldSibling, err := s.Candidate(ctx, project, sibling.ID)
	require(t, err)
	if oldSibling.StopRequested {
		t.Fatal("failed task transaction leaked stop obligation")
	}
	if _, err := s.db.Exec(`DROP TRIGGER fail_runner_delivery`); err != nil {
		t.Fatal(err)
	}
	receipt, err := s.CommitRunnerTaskResult(ctx, input)
	require(t, err)
	if receipt.Status != "closed" || receipt.Delivery.SelectedWorkflowRunID != selected.ID {
		t.Fatal("wrong candidate delivery", receipt)
	}
	retry, err := s.CommitRunnerTaskResult(ctx, input)
	require(t, err)
	if digestValue(retry) != digestValue(receipt) {
		t.Fatal("delivery retry changed original receipt")
	}
	for _, changed := range []RunnerResult{
		func() RunnerResult { v := input; v.TaskVersion++; return v }(),
		func() RunnerResult { v := input; v.CandidateID = sibling.ID; return v }(),
	} {
		if _, err := s.CommitRunnerTaskResult(ctx, changed); err == nil {
			t.Fatal("changed delivery identity reused original receipt")
		}
	}
	for _, query := range []string{
		`SELECT count(*) FROM task_results WHERE project_identity=?`,
		`SELECT count(*) FROM events WHERE project_identity=? AND type='task.delivered'`,
	} {
		var count int
		require(t, s.db.QueryRowContext(ctx, query, project).Scan(&count))
		if count != 1 {
			t.Fatal("delivery replay duplicated result or event", count)
		}
	}
	closed, err := s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	if closed.ByID[selected.TaskID].Status != "closed" || closed.Authority.GraphRevision != graph.Authority.GraphRevision+1 || len(closed.Deliveries) != 1 {
		t.Fatal("delivery was not unique or graph was not released")
	}
	for _, id := range []string{selected.ID, sibling.ID, withdrawn.ID} {
		c, err := s.Candidate(ctx, project, id)
		require(t, err)
		if !c.StopRequested || c.InputsRevoked {
			t.Fatal("delivery did not preserve pending native stop", c)
		}
		if id == withdrawn.ID && digestValue(c.Withdrawal) != digestValue(withdrawn.Withdrawal) {
			t.Fatal("Task terminal overwrote manual withdrawal facts")
		}
	}
	if ready, _ := taskmodel.DependencyStateForTask(closed.ByID[tasks[1].TaskID], closed.TaskGraph()); !ready {
		t.Fatal("successor remained blocked after committed delivery")
	}
}
