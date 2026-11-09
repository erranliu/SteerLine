package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/query"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

// The reference reads ALL source facts, bypassing the SQL activity filter. It
// then uses the public final projection. Filtered SQL must be a superset, and
// paginated final output must equal this reference (including counts).
func assertAsyncActivityFilterContract(t *testing.T, s *SQLiteStore, project, source string) {
	t.Helper()
	ctx := context.Background()
	facts, more, err := s.ReadAsyncActivityFacts(ctx, project, ports.AsyncActivityRead{Source: source, Filter: "summary", Limit: 500})
	require(t, err)
	if more || len(facts) == 0 {
		t.Fatalf("reference must cover complete nonempty matrix: %s %d", source, len(facts))
	}
	for _, filter := range []string{"ongoing", "attention", "recent"} {
		t.Run(filter, func(t *testing.T) {
			want := map[string]bool{}
			for _, f := range facts {
				a, err := query.ProjectAsyncActivity(project, "i", f, time.Now())
				require(t, err)
				if query.MatchesAsyncActivity(a, filter) {
					want[f.ID] = true
				}
			}
			coarse, more, err := s.ReadAsyncActivityFacts(ctx, project, ports.AsyncActivityRead{Source: source, Filter: filter, Limit: 500})
			require(t, err)
			if more {
				t.Fatal("coarse matrix truncated")
			}
			seen := map[string]bool{}
			for _, f := range coarse {
				seen[f.ID] = true
			}
			for id := range want {
				if !seen[id] {
					t.Errorf("SQL omitted final %s match %s", filter, id)
				}
			}
			// Recent deliberately exposes at most 50; the unbounded superset
			// assertion above still covers every terminal matrix combination.
			got := map[string]bool{}
			cursor := ""
			for n := 0; n < 100; n++ {
				page, err := query.ReadAsyncActivities(ctx, s, project, "i", query.AsyncActivityOptions{Sources: []string{source}, Filter: filter, Limit: 7, Cursor: cursor})
				require(t, err)
				if !page.CountsKnown {
					t.Fatalf("unexpected partial source %+v", page.Sources)
				}
				count := len(want)
				if filter == "recent" && count > 50 {
					count = 50
				}
				if page.Counts[filter] != count {
					t.Fatalf("count=%d reference=%d", page.Counts[filter], count)
				}
				for _, a := range page.Items {
					if got[a.SourceID] || !want[a.SourceID] {
						t.Fatalf("duplicate or unexpected match %+v", a)
					}
					got[a.SourceID] = true
				}
				if !page.HasMore {
					break
				}
				if page.NextCursor == "" || page.NextCursor == cursor {
					t.Fatal("cursor did not advance")
				}
				cursor = page.NextCursor
			}
			if filter == "recent" && len(want) > 50 {
				if len(got) != 50 {
					t.Fatalf("recent cap lost records: %d", len(got))
				}
			} else if !reflect.DeepEqual(got, want) {
				t.Fatalf("filtered pages differ: got=%d want=%d", len(got), len(want))
			}
		})
	}
}

func TestAsyncActivitySQLCandidateFilterIsProjectionSuperset(t *testing.T) {
	s, project, tasks := nativeAuditStore(t)
	index := 0
	for _, status := range []string{"nil", "queued", "running", "waiting_input", "waiting_external", "pr_ready", "blocked", "failed", "completed", "closed", "stopped", "future_state"} {
		for _, participation := range []string{"active", "abandoned", "completed"} {
			for _, stop := range []bool{false, true} {
				for _, effect := range []string{"none", "unknown", "accepted", "settled"} {
					index++
					id := fmt.Sprintf("c%03d", index)
					c := runnerstate.Candidate{Project: project, ID: id, TaskID: tasks[0].TaskID, CreatedAt: time.Now().UTC(), Participation: participation, StopRequested: stop, SubmitState: []string{"pending", "accepted", "unknown", "rejected", "cancelled", "future_state"}[index%6]}
					if status != "nil" {
						c.Projection = &runnercontract.Run{Status: status, StopStatus: []string{"active", "stopped"}[index%2], UpdatedAt: time.Now().UTC(), ResumeRequired: index%5 == 0}
						if index%7 == 0 {
							c.Projection.Question = &runnercontract.Question{ID: "q"}
						}
					}
					if effect != "none" {
						c.Actions = []runnerstate.Action{{State: effect, Command: "reply"}}
						if effect == "settled" {
							c.Actions[0].State = "accepted"
							c.Actions[0].ActionStatus = "completed"
						}
					}
					raw, _ := json.Marshal(c)
					_, err := s.db.Exec(`INSERT INTO runner_candidates VALUES(?,?,?,1,?,?,?)`, project, id, c.TaskID, id, "run-"+id, raw)
					require(t, err)
				}
			}
		}
	}
	assertAsyncActivityFilterContract(t, s, project, "task_candidate")
}

func TestAsyncActivitySQLDetectionFiltersAreProjectionSupersets(t *testing.T) {
	s, err := OpenSQLiteStore(t.TempDir() + "/domain.db")
	require(t, err)
	defer s.Close()
	at := time.Now().UTC().Format(time.RFC3339Nano)
	for n, state := range []string{"queued", "running", "stopping", "ready", "failed", "cancelled", "unverified", "future_state", ""} {
		for round := 0; round < 3; round++ {
			id := fmt.Sprintf("e%d-%d", n, round)
			payload := map[string]any{"project": "p", "id": id, "state": state, "created_at": at}
			if state == "" {
				delete(payload, "state")
			}
			if round > 0 {
				run := map[string]any{"updated_at": at}
				if state == "running" && round == 1 {
					run["status"] = "blocked"
					run["phase"] = "operation_rejected"
					run["diagnostic"] = "remote Orca repository query timed out"
				}
				payload["rounds"] = []any{map[string]any{"settled": round == 2, "run": run}}
			}
			raw, _ := json.Marshal(payload)
			_, err = s.db.Exec(`INSERT INTO project_environments VALUES(?,?,1,?)`, "p", id, raw)
			require(t, err)
			current, _ := json.Marshal(map[string]any{"activity_id": id})
			_, err = s.db.Exec(`INSERT INTO project_environment_current VALUES(?,?,1,?)`, "p", id, current)
			require(t, err)
		}
	}
	for _, filter := range []string{"ongoing", "attention", "recent"} {
		page, err := query.ReadAsyncActivities(context.Background(), s, "p", "i", query.AsyncActivityOptions{Filter: filter, Sources: []string{"environment_detection"}, Limit: 100})
		require(t, err)
		found := false
		for _, a := range page.Items {
			if a.SourceID != "e1-1" {
				continue
			}
			found = true
			if a.Phase != "检测阻断" || a.Diagnostic != "remote Orca repository query timed out" || !a.NeedsAttention || a.FinishedAt != nil || a.State != "waiting" {
				t.Fatalf("blocked detection lost original cause or appeared finished: %+v", a)
			}
		}
		if found != (filter != "recent") {
			t.Fatal("blocked detection is ongoing and needs attention until settled", filter, found)
		}
	}
	for n, status := range []string{"pending", "applied", "rejected", "unknown", "future_state"} {
		for _, instance := range []string{"i", "old"} {
			id := fmt.Sprintf("check%d-%s", n, instance)
			raw, _ := json.Marshal(map[string]any{"action": "maintenance_check", "instance_id": instance})
			_, err = s.db.Exec(`INSERT INTO commands VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, "p", id, "project_maintenance", "p", 1, id, "fixture", raw, status, nil, at, at)
			require(t, err)
		}
	}
	t.Run("environment", func(t *testing.T) { assertAsyncActivityFilterContract(t, s, "p", "environment_detection") })
	t.Run("baseline", func(t *testing.T) { assertAsyncActivityFilterContract(t, s, "p", "baseline_check") })
}

func TestAsyncActivitySQLPreparationFilterIsProjectionSuperset(t *testing.T) {
	s, err := OpenSQLiteStore(t.TempDir() + "/domain.db")
	require(t, err)
	defer s.Close()
	ctx := context.Background()
	for n, status := range []string{"queued", "preparing", "generating", "completed", "failed", "future_state"} {
		for child := 0; child < 3; child++ {
			id := fmt.Sprintf("batch%d-%d", n, child)
			require(t, s.RegisterPreparation(ctx, "p", id, pm.GenerationBatch{ID: id, Status: "preparing", Items: []pm.GenerationBatchItem{{TaskID: "1"}}}))
			if child > 0 {
				require(t, s.SaveMaterial(ctx, "p", pm.Material{TaskID: "1", Inputs: id, GenerationID: id}))
				g := tm.Generation{Project: "p", ID: id, Inputs: id, Revision: 1, State: "accepted", Submission: "started", Request: runnercontract.Request{RequestID: id}}
				if child == 2 {
					g.State = "failed"
					g.Projection = &runnercontract.Run{StopStatus: "stopped", UpdatedAt: time.Now().UTC()}
				}
				require(t, s.SaveGeneration(ctx, g, 0))
			}
			require(t, s.SaveGenerationBatch(ctx, "p", pm.GenerationBatch{ID: id, Status: status, Items: []pm.GenerationBatchItem{{TaskID: "1"}}}))
		}
	}
	assertAsyncActivityFilterContract(t, s, "p", "test_preparation")
}
