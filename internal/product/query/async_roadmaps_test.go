package query

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestAsyncActivityRoadmapLifecycles(t *testing.T) {
	t.Run("per-task-progress", func(t *testing.T) {
		raw := []byte(`{"project":"p","id":"r","purpose":"documents","state":"accepted","request_id":"batch","generations":[{"generation_id":"old","local_key":"a","state":"failed"},{"generation_id":"current","local_key":"a","state":"documents-review"},{"generation_id":"other","local_key":"b","state":"accepted","projection":{"status":"running"}}]}`)
		a, err := ProjectAsyncActivity("p", "i", ports.AsyncActivityFact{Source: "roadmap", ID: "r", Payload: raw, Related: []byte(`{"settled":false,"current":1,"requests":["batch","retry"]}`)}, time.Now().UTC())
		if err != nil || a.State != "running" || a.Phase != "生成文档 1/2" || a.Execution.RunID != "" || a.Execution.AttemptID != "" {
			t.Fatalf("incorrect batch activity: %+v %v", a, err)
		}
	})
	t.Run("per-task-native-projections", func(t *testing.T) {
		for _, tc := range []struct {
			name, status, want, phase string
			resume, attention         bool
		}{
			{"queued", "queued", "queued", "排队中", false, false},
			{"running", "running", "running", "生成文档", false, false},
			{"blocked", "blocked", "waiting", "需要处理", false, true},
			{"input", "waiting_input", "waiting", "需要处理", false, true},
			{"external", "waiting_external", "waiting", "等待外部结果", false, false},
			{"completed-not-stopped", "completed", "waiting", "结果收尾中", false, false},
			{"resume-required", "running", "waiting", "需要处理", true, true},
			{"unverified", "future-status", "waiting", "执行状态待核实", false, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				payload := map[string]any{"project": "p", "id": "r", "purpose": "documents", "state": "accepted", "request_id": "batch", "generations": []any{
					map[string]any{"local_key": "T1", "state": "accepted", "projection": map[string]any{"status": "blocked", "resume_required": true}},
					map[string]any{"local_key": "T1", "state": "accepted", "projection": map[string]any{"status": tc.status, "stop_status": "active", "resume_required": tc.resume}},
				}}
				raw, _ := json.Marshal(payload)
				a, err := ProjectAsyncActivity("p", "i", ports.AsyncActivityFact{Source: "roadmap", ID: "r", Payload: raw, Related: []byte(`{"settled":false,"current":1,"requests":["batch"]}`)}, time.Now().UTC())
				if err != nil || a.State != tc.want || a.NeedsAttention != tc.attention || !strings.Contains(a.Phase, tc.phase) {
					t.Fatalf("incorrect current execution projection: %+v %v", a, err)
				}
			})
		}
	})
	for _, tc := range []struct {
		name, purpose, state, run, stop, want  string
		settled, cancelled, current, attention bool
	}{
		{"parse_queued", "", "queued", "", "", "queued", false, false, true, false},
		{"documents_running", "documents", "accepted", "running", "", "running", false, false, true, false},
		{"unknown_runner", "documents", "accepted", "future_state", "", "waiting", false, false, true, false},
		{"runner_blocked", "documents", "accepted", "blocked", "", "waiting", false, false, true, true},
		{"parse_review", "", "review", "completed", "stopped", "succeeded", true, false, true, true},
		{"documents_review", "documents", "documents-review", "completed", "stopped", "succeeded", true, false, true, true},
		{"documents_questions", "documents", "documents-blocked", "completed", "stopped", "waiting", true, false, true, true},
		{"historical_parse_questions", "", "blocked", "completed", "stopped", "failed", true, false, false, false},
		{"historical_document_questions", "documents", "documents-blocked", "completed", "stopped", "failed", true, false, false, false},
		{"historical_questions_still_stopping", "documents", "documents-blocked", "running", "", "waiting", false, false, false, false},
		{"failure_not_settled", "", "failed", "failed", "", "waiting", false, false, true, true},
		{"failure_settled", "", "failed", "failed", "stopped", "failed", true, false, true, true},
		{"historical_failure", "documents", "invalid", "completed", "stopped", "failed", true, false, false, false},
		{"stop_pending", "documents", "cancelled", "running", "", "waiting", false, true, true, false},
		{"stopped", "documents", "cancelled", "stopped", "stopped", "cancelled", true, true, true, false},
		{"publishing", "documents", "publishing", "completed", "stopped", "waiting", true, false, true, true},
		{"merge", "documents", "awaiting-merge", "completed", "stopped", "waiting", true, false, true, false},
		{"admit", "documents", "awaiting-admission", "completed", "stopped", "waiting", true, false, true, true},
		{"admission_unknown", "documents", "admission-unknown", "completed", "stopped", "waiting", true, false, true, true},
		{"admitted", "documents", "admitted", "completed", "stopped", "succeeded", true, false, true, false},
		{"unknown", "", "future_state", "", "", "waiting", false, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			payload := map[string]any{"project": "p", "id": "r", "purpose": tc.purpose, "state": tc.state, "cancelled": tc.cancelled, "request_id": "request", "source": map[string]any{"path": "draft.md"}, "projection": map[string]any{"status": tc.run, "stop_status": tc.stop, "updated_at": now}}
			if tc.state == "publishing" || tc.state == "awaiting-merge" || tc.state == "awaiting-admission" || tc.state == "admission-unknown" || tc.state == "admitted" {
				payload["publication"] = map[string]any{"state": "pr"}
			}
			raw, _ := json.Marshal(payload)
			current := 0
			if tc.current {
				current = 1
			}
			related, _ := json.Marshal(map[string]any{"settled": tc.settled, "current": current, "requests": []string{"request"}})
			a, err := ProjectAsyncActivity("p", "i", ports.AsyncActivityFact{Source: "roadmap", ID: "r", Payload: raw, Related: related}, now)
			if err != nil || a.State != tc.want || a.NeedsAttention != tc.attention || a.Details.Kind != "roadmap" || len(a.RequestIDs) != 1 {
				t.Fatalf("incorrect roadmap projection: %+v %v", a, err)
			}
			if tc.purpose == "documents" && a.Kind != "roadmap_documents" {
				t.Fatalf("documents mislabeled: %+v", a)
			}
			if (!activityTerminal(a.State) || tc.state == "admitted") && a.FinishedAt != nil {
				t.Fatalf("invented completion time: %+v", a)
			}
		})
	}
}

func TestAsyncActivityRoadmapPendingCommandsBelongToCurrentInstance(t *testing.T) {
	for _, state := range []string{"publishing", "admission-unknown"} {
		for _, instance := range []string{"i", "old"} {
			raw, _ := json.Marshal(map[string]any{"project": "p", "id": "r", "state": state, "publication": map[string]any{"state": "preparing"}, "operation": map[string]any{"status": "pending", "instance": instance}})
			a, err := ProjectAsyncActivity("p", "i", ports.AsyncActivityFact{Source: "roadmap", ID: "r", Payload: raw, Related: json.RawMessage(`{"settled":true,"current":1}`)}, time.Now())
			want := "waiting"
			if instance == "i" {
				want = "running"
			}
			if err != nil || a.State != want || a.NeedsAttention != (instance != "i") {
				t.Fatalf("pending %s instance %s: %+v %v", state, instance, a, err)
			}
		}
	}
}
