package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

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
