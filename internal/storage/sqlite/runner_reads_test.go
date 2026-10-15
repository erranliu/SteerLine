package sqlite

import (
	"context"
	"encoding/json"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerPublicReadsKeepOriginalFinalAndCallerBinding(t *testing.T) {
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
	base := strings.Repeat("a", 40)
	input := runnerclient.RunInput{Title: tasks[0].Title, Repository: runnerclient.Repository{URL: contract.Repository, Remote: contract.Remote, BaseBranch: contract.Branch, BaseRevision: base}, Documents: runnerclient.Documents{Spec: runnerclient.Document{Kind: "git", Path: contract.SpecRef.Path, Commit: contract.SpecRef.Commit}, Plan: runnerclient.Document{Kind: "git", Path: contract.PlanRef.Path, Commit: contract.PlanRef.Commit}}, Publication: runnerclient.Publication{BaseBranch: contract.Branch}}
	body, _ := json.Marshal(input)
	c := runnerbridge.Candidate{Project: project, TaskID: tasks[0].TaskID, ID: "read-candidate", Revision: 1, Explicit: true, StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "operator", ContractDigest: contract.Digest, BaseRevision: base, Participation: "active", SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "read-submit", Input: body}}
	require(t, s.ReserveRunnerCandidate(ctx, taskmodel.FixedWork{Record: tasks[0], Contract: contract, Authority: graph.TaskGraph().Authority}, c, false))
	c, err = s.Candidate(ctx, project, c.ID)
	require(t, err)
	c.Revision++
	c.RunID = "original-run"
	c.SubmitState = "accepted"
	c.Projection = &runnerclient.Run{RunID: c.RunID, Revision: 2, Status: "running", Execution: runnerclient.Execution{AttemptID: "attempt"}}
	final := &runnerclient.Final{CommandID: "command", AttemptID: "attempt", SessionID: "session", TurnID: "turn", NativeOutcome: "succeeded", Verified: true, Source: "native.final", ItemID: "item", Text: "原始回复\nexact body"}
	at := time.Now().UTC()
	events := []runnerclient.Event{{EventID: "accepted", RunID: c.RunID, Revision: 1, AttemptID: "attempt", Type: "accepted", CreatedAt: at}, {EventID: "final", RunID: c.RunID, Revision: 2, AttemptID: "attempt", Type: "final", Final: final, CreatedAt: at}}
	require(t, s.SaveCandidate(ctx, c, 1, events))
	// Public event replay cannot duplicate history or change a stored final.
	c.Revision++
	require(t, s.SaveCandidate(ctx, c, 2, events))
	q := query.NewConversationService(s, project)
	page, err := q.ReadConversation(ctx, api.ConversationFilter{WorkflowID: c.ID, ExecutionID: c.RunID, FinalsOnly: true}, "", 100)
	require(t, err)
	if len(page.Events) != 1 || page.Events[0].SourceEventID != "final" || page.Events[0].SessionID != "session" || page.Events[0].TurnID != "turn" || page.Events[0].Body == nil {
		t.Fatalf("public provenance missing: %+v", page)
	}
	ref := *page.Events[0].Body
	contents, err := q.ReadMessageBody(ctx, ref, 0, 256<<10)
	require(t, err)
	if !contents.Complete || string(contents.Data) != final.Text {
		t.Fatalf("original final changed: %+v", contents)
	}
	forged := ref
	forged.AttemptID = "other"
	if _, err := q.ReadMessageBody(ctx, forged, 0, 256); err == nil {
		t.Fatal("cross-Attempt final accepted")
	}
	activity, err := query.NewService(s, project).ReadRecentActivity(ctx, c.ID, c.RunID, "attempt")
	require(t, err)
	if len(activity.Activity) != 2 {
		t.Fatal("Runner event projection missing", activity)
	}
	if _, err := query.NewService(s, project).ReadRecentActivity(ctx, c.ID, "other-run", "attempt"); err == nil {
		t.Fatal("cross-Run activity accepted")
	}
	snapshot, err := s.ReadApplicationProjectSnapshot(ctx, project, c.TaskID, 100)
	require(t, err)
	if snapshot.ProjectView.HistoryError != nil || len(snapshot.ProjectView.History.WorkflowRuns) != 1 || len(snapshot.ProjectView.SelectedTask.WorkflowRuns) != 1 {
		t.Fatalf("caller history unavailable: %+v", snapshot.ProjectView)
	}
	c.Revision++
	changed := events[1]
	copyFinal := *final
	copyFinal.Text = "replacement"
	changed.Final = &copyFinal
	if err := s.SaveCandidate(ctx, c, 3, []runnerclient.Event{changed}); err == nil {
		t.Fatal("immutable public event replaced")
	}
	saved, err := s.Candidate(ctx, project, c.ID)
	require(t, err)
	if saved.Revision != 3 {
		t.Fatal("event conflict did not roll back candidate CAS")
	}
	var retired int
	require(t, s.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('workflow_runs','executions','step_attempts','decisions','decision_answers','operations','execution_bindings','execution_operations','conversation_events','conversation_bodies')`).Scan(&retired))
	if retired != 0 {
		t.Fatal("old execution tables still created")
	}
}
