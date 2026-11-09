package query

import (
	"encoding/json"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

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
