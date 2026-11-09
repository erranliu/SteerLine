package query

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestAsyncActivityAdvisoryStateSettlementAndSourceResolution(t *testing.T) {
	for _, v := range []struct {
		state           string
		settled, active bool
		want            string
		attention       bool
	}{{"queued", true, true, "queued", false}, {"preparing", true, true, "running", false}, {"ready", false, true, "waiting", true}, {"blocked", true, true, "failed", true}, {"blocked", true, false, "failed", false}, {"ready", true, true, "succeeded", true}, {"cancelled", true, true, "cancelled", false}, {"unknown", false, true, "waiting", false}} {
		raw, _ := json.Marshal(map[string]any{"project": "p", "id": "id", "revision": 2, "source": map[string]any{"candidate_id": "candidate", "task_id": "task"}, "state": v.state, "settled": v.settled, "source_active": v.active, "created_at": "2026-11-09T00:00:00Z"})
		a, err := ProjectAsyncActivity("p", "instance", ports.AsyncActivityFact{Source: "main_agent_advisory", ID: "id", Payload: raw, Related: json.RawMessage(`{}`)}, time.Now())
		if err != nil || a.State != v.want || a.NeedsAttention != v.attention || a.Details.Kind != "advisory" || a.Details.CandidateID != "candidate" || len(a.RequestIDs) != 1 {
			t.Fatal(v, a, err)
		}
	}
}
