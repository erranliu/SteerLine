package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/query"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

func TestAsyncActivityBlockedPreparationUsesOriginalGenerationFacts(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "domain.db"))
	require(t, err)
	defer s.Close()
	b := pm.GenerationBatch{ID: "batch", Status: "preparing"}
	for i := range 5 {
		b.Items = append(b.Items, pm.GenerationBatchItem{TaskID: fmt.Sprint(101 + i)})
	}
	require(t, s.RegisterPreparation(ctx, "p", "request", b))
	var blocked tm.Generation
	for i := range b.Items {
		id := b.Items[i].TaskID
		state, materialState := "generated", "ready"
		run := &runnercontract.Run{Status: "completed", StopStatus: "stopped", UpdatedAt: time.Now().UTC()}
		if i == 1 {
			state, materialState = "accepted", "accepted"
			run.Status, run.Phase, run.StopStatus = "blocked", "manual_intervention_required", ""
		}
		g := tm.Generation{Project: "p", ID: "gen-" + id, Revision: 1, Inputs: "input-" + id, State: state, Submission: "started", Projection: run, Request: runnercontract.Request{RequestID: "original-" + id}}
		if i == 1 {
			g.Diagnostic = "app-server did not become ready; original reconciliation exhausted"
			blocked = g
		}
		require(t, s.SaveMaterial(ctx, "p", pm.Material{TaskID: id, Inputs: g.Inputs, GenerationID: g.ID, Status: materialState}))
		require(t, s.SaveGeneration(ctx, g, 0))
		b.Items[i].Status, b.Items[i].Inputs, b.Items[i].GenerationID = materialState, g.Inputs, g.ID
	}
	b.Status = "generating"
	require(t, s.SaveGenerationBatch(ctx, "p", b))
	options := query.AsyncActivityOptions{Filter: "attention", Sources: []string{"test_preparation"}}
	page, err := query.ReadAsyncActivities(ctx, s, "p", "i", options)
	require(t, err)
	if len(page.Items) != 1 || page.Counts["attention"] != 1 || page.Counts["ongoing"] != 1 {
		t.Fatalf("blocked original missing from attention/counts: %+v", page)
	}
	a := page.Items[0]
	if a.State != "waiting" || a.Phase != "测试生成受阻，需人工处理" || !strings.Contains(a.Diagnostic, blocked.Diagnostic) || a.Progress.Succeeded != 4 || a.Progress.Total != 5 || a.FinishedAt != nil {
		t.Fatalf("blocked original rendered as active/completed: %+v", a)
	}
	stored, err := s.Generation(ctx, "p", blocked.ID)
	require(t, err)
	batch, err := s.GenerationBatch(ctx, "p")
	require(t, err)
	if stored.Revision != 1 || stored.State != "accepted" || batch.Status != "generating" {
		t.Fatal("GET changed original business state")
	}
	blocked = stored
	blocked.Projection.Status, blocked.Projection.Phase = "running", "execute"
	blocked.Diagnostic = ""
	blocked.Revision++
	require(t, s.SaveGeneration(ctx, blocked, 1))
	page, err = query.ReadAsyncActivities(ctx, s, "p", "i", options)
	require(t, err)
	if len(page.Items) != 0 || page.Counts["attention"] != 0 || page.Counts["ongoing"] != 1 {
		t.Fatalf("old blocker survived the original Run's recovery: %+v", page)
	}
}

func TestAsyncActivityPreparationSurvivesReplacementAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "domain.db")
	s, e := OpenSQLiteStore(path)
	if e != nil {
		t.Fatal(e)
	}
	b := pm.GenerationBatch{ID: "old", Status: "preparing", Items: []pm.GenerationBatchItem{{TaskID: "1"}}}
	if e = s.RegisterPreparation(ctx, "p", "request-old", b); e != nil {
		t.Fatal(e)
	}
	if e = s.SaveMaterial(ctx, "p", pm.Material{TaskID: "1", Inputs: "input", GenerationID: "gen", Status: "pending"}); e != nil {
		t.Fatal(e)
	}
	g := tm.Generation{Project: "p", ID: "gen", Revision: 1, Inputs: "input", State: "accepted", Submission: "started", Request: runnercontract.Request{RequestID: "original"}}
	if e = s.SaveGeneration(ctx, g, 0); e != nil {
		t.Fatal(e)
	}
	b.Status = "failed"
	b.ResultSnapshot = &pm.GenerationProgress{Total: 1, Running: 1}
	if e = s.SaveGenerationBatch(ctx, "p", b); e != nil {
		t.Fatal(e)
	}
	if e = s.RegisterPreparation(ctx, "p", "request-new", pm.GenerationBatch{ID: "new", Status: "queued", Items: []pm.GenerationBatchItem{{TaskID: "2"}}}); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = OpenSQLiteStore(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	read := func() ports.AsyncActivityFact {
		rows, _, err := s.ReadAsyncActivityFacts(ctx, "p", ports.AsyncActivityRead{Source: "test_preparation", Limit: 50, RequestIDs: []string{"request-old"}})
		if err != nil || len(rows) != 1 {
			t.Fatalf("history read: %v %+v", err, rows)
		}
		return rows[0]
	}
	a, e := query.ProjectAsyncActivity("p", "i", read(), time.Now())
	if e != nil || a.State != "waiting" || a.SourceID != "old" {
		t.Fatalf("old batch lost or prematurely ended: %+v %v", a, e)
	}
	stamp := time.Now().UTC().Add(-2 * time.Minute)
	require(t, s.RecordSourceObservation(ctx, "p", "test_generation", "gen", 1, stamp, ""))
	require(t, s.RecordSourceObservation(ctx, "p", "test_generation", "gen", 1, time.Time{}, "inspection timed out"))
	a, e = query.ProjectAsyncActivity("p", "i", read(), time.Now())
	if e != nil || a.State != "waiting" || a.Observation != "unavailable" || a.ObservedAt == nil || !a.ObservedAt.Equal(stamp) {
		t.Fatalf("historical child observation was masked: %+v %v", a, e)
	}
	g, e = s.Generation(ctx, "p", g.ID)
	if e != nil {
		t.Fatal(e)
	}
	g.Revision++
	g.State = "failed"
	g.Projection = &runnercontract.Run{StopStatus: "stopped", UpdatedAt: time.Now().UTC()}
	if e = s.SaveGeneration(ctx, g, 1); e != nil {
		t.Fatal(e)
	}
	a, e = query.ProjectAsyncActivity("p", "i", read(), time.Now())
	if e != nil || a.State != "failed" || a.FinishedAt == nil {
		t.Fatalf("historical settlement did not converge: %+v %v", a, e)
	}
	current, e := s.GenerationBatch(ctx, "p")
	if e != nil || current.ID != "new" {
		t.Fatalf("read changed current batch: %+v %v", current, e)
	}
}

func TestAsyncActivityUnknownActionRemainsInOngoingList(t *testing.T) {
	s, project, tasks := nativeAuditStore(t)
	ctx := context.Background()
	c := runnerstate.Candidate{Project: project, ID: "c", TaskID: tasks[0].TaskID, Participation: "abandoned", SubmitState: "accepted", CreatedAt: time.Now().UTC(), Projection: &runnercontract.Run{Status: "stopped", StopStatus: "stopped", UpdatedAt: time.Now().UTC()}, Actions: []runnerstate.Action{{State: "unknown", Command: "reply"}}}
	for _, state := range []string{"unknown", "accepted", "completed"} {
		c.Actions[0].State = state
		if state == "completed" {
			c.Actions[0].State, c.Actions[0].ActionStatus = "accepted", "completed"
		}
		raw, _ := json.Marshal(c)
		_, err := s.db.ExecContext(ctx, `INSERT INTO runner_candidates VALUES(?,?,?,1,?,?,?) ON CONFLICT(project_identity,candidate_id) DO UPDATE SET payload=excluded.payload`, project, c.ID, c.TaskID, "request", "run", raw)
		require(t, err)
		p, err := query.ReadAsyncActivities(ctx, s, project, "i", query.AsyncActivityOptions{Filter: "ongoing", Sources: []string{"task_candidate"}})
		require(t, err)
		want := 1
		if state == "completed" {
			want = 0
		}
		if len(p.Items) != want || p.Counts["ongoing"] != want {
			t.Fatalf("%s count/list diverged: %+v", state, p)
		}
		if want == 0 {
			recent, err := query.ReadAsyncActivities(ctx, s, project, "i", query.AsyncActivityOptions{Filter: "recent", Sources: []string{"task_candidate"}})
			require(t, err)
			if len(recent.Items) != 1 || recent.Items[0].State != "cancelled" {
				t.Fatalf("settled action missing from history: %+v", recent)
			}
		}
	}
}
func TestAsyncActivityPaginationAndRequestLookup(t *testing.T) {
	ctx := context.Background()
	s, e := OpenSQLiteStore(filepath.Join(t.TempDir(), "domain.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for n := 0; n < 520; n++ {
		id := fmt.Sprintf("%032d", n)
		at := time.Date(2026, 11, 6, 0, 0, n, 0, time.UTC)
		payload, _ := json.Marshal(map[string]any{"action": "maintenance_check", "instance_id": "i"})
		_, e = s.db.Exec(`INSERT INTO commands VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, "p", id, "project_maintenance", "p", 1, id, "local", payload, "pending", nil, at.Format(time.RFC3339Nano), at.Format(time.RFC3339Nano))
		if e != nil {
			t.Fatal(e)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for n := 0; n < 30; n++ {
		p, e := query.ReadAsyncActivities(ctx, s, "p", "i", query.AsyncActivityOptions{Limit: 25, Cursor: cursor, RequestIDs: []string{fmt.Sprintf("%032d", 519)}})
		if e != nil {
			t.Fatal(e)
		}
		if len(p.Links) != 1 {
			t.Fatal("association lookup depended on page")
		}
		for _, a := range p.Items {
			if seen[a.ID] {
				t.Fatalf("duplicate %s", a.ID)
			}
			seen[a.ID] = true
		}
		if !p.HasMore {
			break
		}
		if p.NextCursor == "" || p.NextCursor == cursor {
			t.Fatal("cursor failed to advance")
		}
		cursor = p.NextCursor
	}
	if len(seen) != 520 {
		t.Fatalf("pagination lost records: %d", len(seen))
	}
	if _, e = query.ReadAsyncActivities(ctx, s, "other", "i", query.AsyncActivityOptions{Cursor: cursor}); e == nil {
		t.Fatal("cross-project cursor accepted")
	}
}
func TestAsyncActivityObservationMetadataIsSeparate(t *testing.T) {
	ctx := context.Background()
	s, e := OpenSQLiteStore(filepath.Join(t.TempDir(), "domain.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	at := time.Now().UTC()
	if e = s.RecordSourceObservation(ctx, "p", "task_candidate", "c", 3, at, ""); e != nil {
		t.Fatal(e)
	}
	if e = s.RecordSourceObservation(ctx, "p", "task_candidate", "c", 3, time.Time{}, "offline"); e != nil {
		t.Fatal(e)
	}
	var stamp, diagnostic string
	if e = s.db.QueryRow(`SELECT last_success,diagnostic FROM source_observations`).Scan(&stamp, &diagnostic); e != nil || stamp != at.Format(time.RFC3339Nano) || diagnostic != "offline" {
		t.Fatalf("observation overwrote success %s %s %v", stamp, diagnostic, e)
	}
}
