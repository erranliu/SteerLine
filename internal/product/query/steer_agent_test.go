package query

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steeragent"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

type classificationReader struct{ fact ports.AsyncActivityFact }

func (r classificationReader) ReadAsyncActivityFacts(_ context.Context, _ string, in ports.AsyncActivityRead) ([]ports.AsyncActivityFact, bool, error) {
	if in.Source != r.fact.Source {
		return nil, false, nil
	}
	return []ports.AsyncActivityFact{r.fact}, false, nil
}

func TestSteerAgentClassificationUsesOnlyContentOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, source, purpose, state string
		want                         *api.SteerAgentClassification
	}{
		{"suggestion", "main_agent_advisory", "", "ready", &api.SteerAgentClassification{Type: steeragent.Continuation, Operation: steeragent.Suggest}},
		{"parse", "roadmap", "", "review", &api.SteerAgentClassification{Type: steeragent.Planning, Operation: steeragent.Parse}},
		{"documents", "roadmap", "documents", "documents-review", &api.SteerAgentClassification{Type: steeragent.Material, Operation: steeragent.GenerateDocuments}},
		{"publishing", "roadmap", "documents", "publishing", &api.SteerAgentClassification{Type: steeragent.Material, Operation: steeragent.GenerateDocuments}},
		{"admission", "roadmap", "documents", "admission-unknown", &api.SteerAgentClassification{Type: steeragent.Material, Operation: steeragent.GenerateDocuments}},
		{"unknown-purpose", "roadmap", "future", "review", nil},
		{"business-control", "baseline_check", "", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{"project": "p", "id": "record", "purpose": tc.purpose, "state": tc.state, "revision": 1, "request_id": "request", "run_id": "original-run", "settled": true, "source_active": true, "source": map[string]any{"candidate_id": "candidate", "task_id": "task"}, "created_at": "2026-11-10T00:00:00Z"}
			local := tc.name == "publishing" || tc.name == "admission"
			action, phase := "roadmap_publish", "发布 Roadmap 文档"
			if tc.name == "admission" {
				action, phase = "roadmap_admit", "Roadmap 正在入图"
			}
			if local {
				payload["operation"] = map[string]any{"id": "local-command", "action": action, "status": "pending", "instance": "i"}
				payload["publication"] = map[string]any{"state": "preparing"}
			}
			if tc.source == "baseline_check" {
				payload = map[string]any{"command_id": "record", "status": "pending", "payload": map[string]any{"instance_id": "i"}}
			}
			raw, _ := json.Marshal(payload)
			requests := []string{"request"}
			if local {
				requests = append(requests, "local-command")
			}
			related, _ := json.Marshal(map[string]any{"settled": true, "current": 1, "requests": requests})
			fact := ports.AsyncActivityFact{Source: tc.source, ID: "record", Payload: raw, Related: related, SortAt: "2026-11-10T00:00:00Z"}
			a, err := ProjectAsyncActivity("p", "i", fact, time.Now())
			if err != nil || !reflect.DeepEqual(a.SteerAgent, tc.want) {
				t.Fatalf("wrong classification: %+v, %v", a, err)
			}
			if local && (a.Execution.RunID != "" || a.Execution.AttemptID != "" || a.Execution.Action != action || a.Execution.RequestID != "local-command" || a.Phase != phase) {
				t.Fatalf("classification replaced local execution facts: %+v", a)
			}
			filter := "recent"
			if local || tc.source == "baseline_check" {
				filter = "ongoing"
			}
			page, err := ReadAsyncActivities(context.Background(), classificationReader{fact}, "p", "i", AsyncActivityOptions{Filter: filter, Sources: []string{tc.source}, RequestIDs: a.RequestIDs})
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Items) != 1 {
				t.Fatalf("classification changed activity count: %+v", page)
			}
			for _, item := range page.Items {
				if !reflect.DeepEqual(item.SteerAgent, tc.want) || item.Execution != a.Execution || item.Phase != a.Phase {
					t.Fatal("list classification or execution facts differ", item)
				}
			}
			for _, id := range a.RequestIDs {
				linked, ok := page.Links[id]
				if !ok || !reflect.DeepEqual(linked.SteerAgent, tc.want) || linked.Execution != a.Execution || linked.Phase != a.Phase {
					t.Fatalf("request link lost original classification or execution: %+v", page)
				}
			}
			wire, _ := json.Marshal(a)
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(wire, &fields)
			if tc.want == nil && fields["steer_agent"] != nil {
				t.Fatal("unknown or unrelated work acquired a capability", string(wire))
			}
		})
	}
}
