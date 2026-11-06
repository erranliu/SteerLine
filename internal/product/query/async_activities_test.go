package query

import (
	"encoding/json"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestAsyncActivityBusinessAndSettlementAreIndependent(t *testing.T) {
	now := time.Now().UTC()
	batch := pm.GenerationBatch{ID: "batch", Status: "failed", CreatedAt: now, BusinessFinishedAt: &now, ResultSnapshot: &pm.GenerationProgress{Total: 1, Running: 1}}
	raw, _ := json.Marshal(batch)
	f := ports.AsyncActivityFact{Source: "test_preparation", ID: batch.ID, Payload: raw, Related: json.RawMessage(`{"current":0,"requests":["request"],"generations":[{"id":"g","present":1,"state":"accepted","submission":"started","stop_status":"","relevant":1}]}`)}
	a, err := ProjectAsyncActivity("p", "i", f, now)
	if err != nil || a.State != "waiting" || a.FinishedAt != nil || a.Summary != "测试准备失败" {
		t.Fatalf("premature completion: %+v %v", a, err)
	}
	f.Related = json.RawMessage(`{"current":0,"requests":["request"],"generations":[{"id":"g","present":1,"state":"failed","submission":"started","stop_status":"stopped","settled":true,"relevant":1,"updated_at":"2026-11-06T12:00:00Z"}]}`)
	a, err = ProjectAsyncActivity("p", "i", f, now)
	if err != nil || a.State != "failed" || !a.NeedsAttention || a.FinishedAt == nil {
		t.Fatalf("settlement failed: %+v %v", a, err)
	}
}
func TestAsyncActivityHistoricalQuestionsNeverBecomeCurrentActions(t *testing.T) {
	now := time.Now().UTC()
	c := runnerstate.Candidate{Project: "p", ID: "c", TaskID: "1", Participation: "abandoned", Projection: &runnercontract.Run{Status: "waiting_input", Question: &runnercontract.Question{ID: "q"}}}
	raw, _ := json.Marshal(c)
	f := ports.AsyncActivityFact{Source: "task_candidate", ID: "c", Payload: raw, Related: json.RawMessage(`{"task_status":"closed","winner":"other"}`)}
	a, err := ProjectAsyncActivity("p", "i", f, now)
	if err != nil || a.NeedsAttention || a.State != "cancelled" {
		t.Fatalf("historical question acted as current: %+v %v", a, err)
	}
	c.StopRequested = true
	raw, _ = json.Marshal(c)
	f.Payload = raw
	a, err = ProjectAsyncActivity("p", "i", f, now)
	if err != nil || a.State != "waiting" || a.NeedsAttention {
		t.Fatalf("unknown stop lost: %+v %v", a, err)
	}
}
func TestAsyncActivityObservationFailureDoesNotFailRunningWork(t *testing.T) {
	now := time.Now().UTC()
	r := env.Record{Project: "p", ID: "env", State: "running", CreatedAt: now}
	raw, _ := json.Marshal(r)
	a, err := ProjectAsyncActivity("p", "i", ports.AsyncActivityFact{Source: "environment_detection", ID: "env", Payload: raw, Related: json.RawMessage(`{"current":1,"observation":{"observed_at":"2026-11-06T12:00:00Z","diagnostic":"connection lost"}}`)}, now)
	if err != nil || a.State != "running" || a.Observation != "unavailable" || a.NeedsAttention {
		t.Fatalf("observation changed business state: %+v %v", a, err)
	}
}

func TestAsyncActivityNormalPreparationIsNotAUserBlocker(t *testing.T) {
	for _, status := range []string{"queued", "preparing", "generating"} {
		raw, _ := json.Marshal(pm.GenerationBatch{ID: "batch", Status: status, Items: []pm.GenerationBatchItem{{TaskID: "1", Status: "pending"}}})
		a, err := ProjectAsyncActivity("p", "i", ports.AsyncActivityFact{Source: "test_preparation", ID: "batch", Payload: raw, Related: json.RawMessage(`{"current":1}`)}, time.Now())
		if err != nil || a.NeedsAttention || activityTerminal(a.State) {
			t.Fatalf("normal %s became a blocker: %+v %v", status, a, err)
		}
	}
}

func TestAsyncActivityPreparationFreshnessUsesEveryUnsettledGeneration(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-2 * time.Minute)
	raw, _ := json.Marshal(pm.GenerationBatch{ID: "b", Status: "generating", ObservedAt: &now})
	for _, tc := range []struct {
		name, want string
		children   []map[string]any
		wantTime   *time.Time
	}{
		{"missing", "unverified", []map[string]any{{"id": "a", "observed_at": now}, {"id": "b"}}, nil},
		{"oldest", "stale", []map[string]any{{"id": "a", "observed_at": now}, {"id": "b", "observed_at": old}}, &old},
		{"known_error", "unavailable", []map[string]any{{"id": "a", "observed_at": old, "observation_diagnostic": "deadline exceeded"}}, &old},
		{"fresh", "current", []map[string]any{{"id": "a", "observed_at": now}}, &now},
		{"settled_ignored", "current", []map[string]any{{"id": "a", "observed_at": now}, {"id": "b", "settled": true, "observation_diagnostic": "old outage"}}, &now},
	} {
		t.Run(tc.name, func(t *testing.T) {
			related, _ := json.Marshal(map[string]any{"generations": tc.children})
			a, err := ProjectAsyncActivity("p", "i", ports.AsyncActivityFact{Source: "test_preparation", ID: "b", Payload: raw, Related: related}, now)
			if err != nil || a.State != "running" || a.Observation != tc.want || (a.ObservedAt == nil) != (tc.wantTime == nil) || tc.wantTime != nil && !a.ObservedAt.Equal(*tc.wantTime) {
				t.Fatalf("local worker tick masked child observations: %+v %v", a, err)
			}
		})
	}
}
