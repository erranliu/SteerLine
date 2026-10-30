package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
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
	input := runnerclient.RunInput{Title: tasks[0].Title, Repository: runnerclient.Repository{URL: contract.Repository, Remote: contract.Remote, BaseBranch: contract.Branch, BaseRevision: base}, Documents: runnerclient.Documents{Spec: runnerclient.Document{Path: contract.SpecRef.Path}, Plan: runnerclient.Document{Path: contract.PlanRef.Path}}, Publication: runnerclient.Publication{BaseBranch: contract.Branch}}
	body, _ := json.Marshal(input)
	c := runnerbridge.Candidate{Project: project, TaskID: tasks[0].TaskID, ID: "read-candidate", Revision: 1, Explicit: true, StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "operator", ContractDigest: contract.Digest, BaseRevision: base, Participation: "active", SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "read-submit", Input: body}}
	require(t, reserveFixtureCandidate(t, s, ctx, taskmodel.FixedWork{Record: tasks[0], Contract: contract, Authority: graph.TaskGraph().Authority}, c, false))
	c, err = s.Candidate(ctx, project, c.ID)
	require(t, err)
	c.Revision++
	c.RunID = "original-run"
	c.SubmitState = "accepted"
	c.Projection = &runnerclient.Run{RunID: c.RunID, Revision: 2, Status: "running", Execution: runnerclient.Execution{AttemptID: "attempt", NativeOutcome: "unreported"}}
	final := &runnerclient.Final{CommandID: "command", AttemptID: "attempt", SessionID: "session", TurnID: "turn", NativeOutcome: "unreported", Verified: true, Source: "native.final", ItemID: "item", Text: "原始回复\nexact body"}
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
	if snapshot.ProjectView.HistoryError != nil || len(snapshot.ProjectView.Candidates) != 1 || len(snapshot.ProjectView.SelectedTask.Candidates) != 1 {
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
	if saved.Revision != 3 || saved.Projection.Execution.NativeOutcome != "unreported" {
		t.Fatal("event conflict did not roll back candidate CAS")
	}
	var retired int
	require(t, s.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('workflow_runs','executions','step_attempts','decisions','decision_answers','operations','execution_bindings','execution_operations','conversation_events','conversation_bodies')`).Scan(&retired))
	if retired != 0 {
		t.Fatal("old execution tables still created")
	}
	t.Run("activity stream stays pinned and releases its connection", func(t *testing.T) {
		binding, records, err := s.OpenActivityView(ctx, project, c.ID, c.RunID, "attempt", 0, 200, false)
		require(t, err)
		defer records.Close()
		writer, err := OpenSQLiteStore(s.path)
		require(t, err)
		defer writer.Close()
		late := runnerclient.Event{EventID: "during-snapshot", RunID: c.RunID, AttemptID: "attempt", Type: "operation_committed", CreatedAt: at}
		raw, err := json.Marshal(late)
		require(t, err)
		_, err = writer.db.ExecContext(ctx, `INSERT INTO runner_events(project_identity,candidate_id,event_id,payload) VALUES(?,?,?,?)`, project, c.ID, late.EventID, raw)
		require(t, err)
		count := 0
		for {
			record, err := records.Next()
			require(t, err)
			if record == nil {
				break
			}
			count++
			if record.Event.EventID == late.EventID || record.Event.RunID != binding.ExecutionID {
				t.Fatal("later event or foreign identity entered original sample", record)
			}
		}
		if count != 2 {
			t.Fatal("pinned activity sample changed", count)
		}
		require(t, records.Close())
		require(t, records.Close())
		if _, err := records.Next(); err == nil {
			t.Fatal("closed stream accepted another read")
		}
		latest, err := query.NewService(s, project).ReadActivity(ctx, c.ID, c.RunID, "attempt", api.ActivityCursor{})
		require(t, err)
		if len(latest.Activity) != 3 || latest.Activity[2].ItemID != late.EventID {
			t.Fatal("stream close kept a connection or hid the next committed sample", latest)
		}
	})
	t.Run("cancelled activity stream releases its transaction", func(t *testing.T) {
		readCtx, cancel := context.WithCancel(ctx)
		_, records, err := s.OpenActivityView(readCtx, project, c.ID, c.RunID, "attempt", 0, 200, false)
		require(t, err)
		defer records.Close()
		cancel()
		if _, err := records.Next(); !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled stream did not preserve cancellation", err)
		}
		_ = records.Close() // Cancellation may already have rolled the read transaction back.
		nextCtx, nextCancel := context.WithTimeout(ctx, 5*time.Second)
		defer nextCancel()
		_, err = query.NewService(s, project).ReadRecentActivity(nextCtx, c.ID, c.RunID, "attempt")
		require(t, err)
	})
	t.Run("readable changes survive technical event noise", func(t *testing.T) {
		noise := []runnerclient.Event{}
		for i := 0; i < 7; i++ {
			noise = append(noise, runnerclient.Event{EventID: fmt.Sprintf("change-%d", i), RunID: c.RunID, AttemptID: "attempt", Type: "workflow_changed", CreatedAt: at, Transition: &runnercontract.WorkflowTransition{From: runnercontract.WorkflowState{Stage: "implement.execute", Status: "running"}, To: runnercontract.WorkflowState{Stage: "implement.report", Status: "blocked", Diagnostic: fmt.Sprintf("HTTP 401 #%d", i)}}})
		}
		for i := 0; i < 40; i++ {
			noise = append(noise, runnerclient.Event{EventID: fmt.Sprintf("technical-%d", i), RunID: c.RunID, AttemptID: "attempt", Type: "operation_committed", CreatedAt: at})
		}
		saved.Revision++
		require(t, s.SaveCandidate(ctx, saved, 3, noise))
		activity, err := query.NewService(s, project).ReadRecentActivity(ctx, c.ID, c.RunID, "attempt")
		require(t, err)
		changes, technical := 0, 0
		for _, event := range activity.Activity {
			if event.Kind == "worker_state" {
				changes++
				if !strings.Contains(event.Title, "implement.report") || !strings.Contains(event.Title, "已阻塞") || !strings.Contains(event.Detail, "HTTP 401") || event.ItemID == "change-0" || event.ItemID == "change-1" {
					t.Fatal("unreadable or stale transition", event)
				}
			} else {
				technical++
			}
		}
		if changes != 5 || technical != 10 {
			t.Fatal("changes lost to internal noise", changes, technical)
		}
		if _, err := query.NewService(s, project).ReadRecentActivity(ctx, c.ID, c.RunID, "other-attempt"); err == nil {
			t.Fatal("foreign Attempt accepted")
		}
	})
}
