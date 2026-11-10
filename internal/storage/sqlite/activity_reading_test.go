package sqlite

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/product/query"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestActivityReadingTerminalWritersKeepInputStateCurrent(t *testing.T) {
	for _, mode := range []string{"withdrawal", "task-stop"} {
		t.Run(mode, func(t *testing.T) {
			s, project, reserve := candidateStopFixture(t)
			c := reserve("first", false)
			reserve("second", true)
			c.Actions = []runnerstate.Action{{Command: "reply", State: "pending", Request: runnercontract.Request{RequestID: "answer", Input: json.RawMessage(`{"text":"user answer","kind":"answer"}`)}}}
			c.Revision++
			ctx := context.Background()
			require(t, s.SaveCandidate(ctx, c, c.Revision-1, nil))
			switch mode {
			case "withdrawal":
				_, err := s.StopRunnerCandidate(ctx, project, c.ID, c.Revision, "withdraw", "operator")
				require(t, err)
			case "task-stop":
				require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.requestRunnerTaskStops(project, c.TaskID) }))
			}
			w, err := s.ReadReadingWindow(ctx, project, "task_candidate", c.ID, 0, 0, 50)
			require(t, err)
			if len(w.Records) != 1 || w.Records[0].State != "cancelled" {
				t.Fatalf("terminal owner left stale delivery state: %+v", w)
			}
		})
	}
}

func TestActivityReadingEnvironmentSummaryExcludesLargeOriginalPayloads(t *testing.T) {
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "domain.db"))
	require(t, err)
	defer s.Close()
	ctx := context.Background()
	large := strings.Repeat("large original text ", 150000)
	r := env.Record{FormatVersion: 1, Project: "p", ID: strings.Repeat("a", 32), Revision: 1, Kind: "adaptation", State: "running", CreatedAt: time.Now().UTC(), Requests: []env.Request{{ID: "start", Action: "start", Original: json.RawMessage(`{"text":"` + large + `"}`)}}, Rounds: []env.Round{{Number: 1, RunID: "run", Request: runnercontract.Request{Input: json.RawMessage(`{"text":"` + large + `"}`)}, Run: &runnercontract.Run{Status: "blocked", Diagnostic: "needs context", Execution: runnercontract.Execution{AttemptID: "attempt"}, Final: &runnercontract.Final{Verified: true, Text: large, AttemptID: "attempt", SessionID: "session", TurnID: "turn"}}}}}
	raw, err := json.Marshal(r)
	require(t, err)
	tx, err := s.db.BeginTx(ctx, nil)
	require(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO project_environments VALUES(?,?,?,?)`, r.Project, r.ID, r.Revision, raw)
	require(t, err)
	require(t, indexEnvironmentReading(ctx, tx, r))
	require(t, tx.Commit())
	for _, filter := range []string{"summary", "ongoing"} {
		facts, _, err := s.ReadAsyncActivityFacts(ctx, "p", ports.AsyncActivityRead{Source: "environment_detection", Filter: filter, Limit: 50})
		require(t, err)
		if len(facts) != 1 || len(facts[0].Payload)+len(facts[0].Related) > 4096 || strings.Contains(string(facts[0].Payload), "large original") {
			t.Fatalf("summary materialized original bodies: %d facts", len(facts))
		}
		a, err := query.ProjectAsyncActivity("p", "i", facts[0], time.Now())
		require(t, err)
		if a.State != "waiting" || a.Diagnostic != "needs context" || len(a.RequestIDs) != 1 || a.RequestIDs[0] != "start" {
			t.Fatalf("narrow summary lost original state: %+v", a)
		}
	}
}

func TestActivityReadingHistoryIncludesMissingFinishAcrossPages(t *testing.T) {
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "domain.db"))
	require(t, err)
	defer s.Close()
	ctx := context.Background()
	for i := 0; i < 75; i++ {
		id := fmt.Sprintf("%032x", i+1)
		r := roadmap.Record{Project: "p", ID: id, RequestID: id, Revision: 1, State: "admitted", Submission: "started", Source: roadmap.Source{ID: id, Path: "draft.md"}, Publication: &roadmap.Publication{RequestID: "publish-" + id}, Projection: &runnercontract.Run{Status: "completed", StopStatus: "stopped", UpdatedAt: time.Now().UTC()}}
		if i%2 == 0 {
			r.CreatedAt = time.Now().UTC()
		}
		require(t, s.SaveRoadmap(ctx, r, 0))
	}
	seen := map[string]bool{}
	cursor := ""
	for n := 0; n < 10; n++ {
		page, err := query.ReadAsyncActivities(ctx, s, "p", "i", query.AsyncActivityOptions{Filter: "history", Limit: 17, Cursor: cursor, Sources: []string{"roadmap"}})
		require(t, err)
		for _, a := range page.Items {
			if a.FinishedAt != nil || seen[a.ID] {
				t.Fatalf("invented finish or duplicate: %+v", a)
			}
			seen[a.ID] = true
		}
		if !page.HasMore {
			break
		}
		if page.NextCursor == cursor {
			t.Fatal("cursor stuck")
		}
		cursor = page.NextCursor
	}
	if len(seen) != 75 {
		t.Fatalf("missing null-ended history: %d", len(seen))
	}
}

func TestActivityReadingIndexesOriginalFieldsWithoutBodiesAndRejectsCorruption(t *testing.T) {
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "domain.db"))
	require(t, err)
	defer s.Close()
	ctx := context.Background()
	id := strings.Repeat("a", 32)
	text := strings.Repeat("消息正文\n", 90000)
	r := roadmap.Record{Project: "p", ID: id, RequestID: id, Revision: 1, State: "queued", Submission: "pending", Source: roadmap.Source{ID: id, Path: "draft.md", Body: text}}
	require(t, s.SaveRoadmap(ctx, r, 0))
	q := query.ReadingService{Reader: s, Project: "p", Instance: "i", Bodies: query.NewReadingBodyCache()}
	activity := base64.RawURLEncoding.EncodeToString([]byte("p\x00roadmap\x00" + id))
	page, err := q.Page(ctx, activity, "", 50)
	require(t, err)
	if len(page.Items) != 1 || len(page.Items[0].Preview) > 512 || page.Items[0].Bytes != len(text) {
		t.Fatalf("unbounded or missing original: %+v", page)
	}
	item := page.Items[0]
	body, err := q.Body(ctx, activity, item.ID, item.SHA256, 0)
	require(t, err)
	if body.Complete || len(body.Text) > 256<<10 {
		t.Fatalf("body auto fetched: %+v", body)
	}
	var storedBytes int
	require(t, s.db.QueryRow(`SELECT length(CAST(preview AS BLOB)) FROM reading_fields`).Scan(&storedBytes))
	if storedBytes > 512 {
		t.Fatal("body copied into index")
	}
	_, err = s.db.Exec(`UPDATE roadmaps SET payload=json_set(payload,'$.source.body','corrupted') WHERE project_identity='p'`)
	require(t, err)
	q.Bodies = query.NewReadingBodyCache()
	if _, err = q.Body(ctx, activity, item.ID, item.SHA256, 0); err == nil {
		t.Fatal("corrupt original body accepted")
	}
}

func TestActivityReadingInputDoesNotBorrowCurrentAttemptAndSnapshotIsSeparate(t *testing.T) {
	s, p, tasks := nativeAuditStore(t)
	ctx := context.Background()
	c := runnerstate.Candidate{Project: p, ID: "c", TaskID: tasks[0].TaskID, RunID: "run", SubmitState: "accepted", Participation: "active", Projection: &runnercontract.Run{Status: "running", Execution: runnercontract.Execution{AttemptID: "current", SessionID: "session"}}, Actions: []runnerstate.Action{{Command: "reply", State: "unknown", Request: runnercontract.Request{RequestID: "input", Input: json.RawMessage(`{"text":"old answer","kind":"answer","expected_revision":1}`)}}}}
	raw, _ := json.Marshal(c)
	_, err := s.db.Exec(`INSERT INTO runner_candidates VALUES(?,?,?,1,?,?,?)`, p, c.ID, c.TaskID, "submit", c.RunID, raw)
	require(t, err)
	tx, err := s.db.BeginTx(ctx, nil)
	require(t, err)
	require(t, indexCandidateReading(ctx, tx, runnerstate.Candidate{}, c, nil))
	require(t, tx.Commit())
	w, err := s.ReadReadingWindow(ctx, p, "task_candidate", "c", 0, 0, 50)
	require(t, err)
	if len(w.Records) != 1 || w.Records[0].AttemptID != "" || w.Records[0].Group != "" || w.Records[0].State != "unknown" {
		t.Fatalf("borrowed input identity: %+v", w)
	}
	m := ports.MessageObservation{Project: p, CandidateID: c.ID, RunID: c.RunID, AttemptID: "current", SessionID: "session", TurnID: "turn", Text: "assistant", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	require(t, s.SaveMessageObservation(ctx, m))
	m.AttemptID = "other"
	m.Text = "wrong"
	require(t, s.SaveMessageObservation(ctx, m))
	m.AttemptID = "current"
	saved, err := s.LoadMessageObservation(ctx, m)
	require(t, err)
	if saved.Text != "assistant" {
		t.Fatal("late snapshot crossed Attempt")
	}
	var rev int
	require(t, s.db.QueryRow(`SELECT revision FROM runner_candidates WHERE candidate_id='c'`).Scan(&rev))
	if rev != 1 {
		t.Fatal("message changed business revision")
	}
}
