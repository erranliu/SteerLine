package query

import (
	"encoding/json"
	"strings"
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestStateNamingActivityRejectsMissingWorkflowFacts(t *testing.T) {
	binding := ports.ActivityBinding{ProjectIdentity: "project", ExecutionWorkflowRunID: "run", ExecutionID: "execution", AttemptExecutionID: "execution", AttemptID: "attempt"}
	var events []ports.DomainEvent
	for i, target := range []map[string]any{
		{"step_id": "publish", "status": "blocked"},
		{"workflow_stage": "publish"},
		{"workflow_stage": "publish", "workflow_status": "blocked"},
	} {
		raw, _ := json.Marshal(api.ActivityEvent{Kind: "worker_state", Meta: map[string]any{"to": target}})
		events = append(events, ports.DomainEvent{ProjectIdentity: "project", WorkflowRunID: "run", ExecutionID: "execution", AttemptID: "attempt", EventID: int64(i + 1), Type: "activity.worker_state", Payload: raw})
	}
	batch, err := ProjectActivity("project", "run", "execution", "attempt", api.ActivityCursor{}, binding, events)
	if err != nil || len(batch.Activity) != 1 || len(batch.Diagnostics) != 2 || batch.Cursor.AfterEventID != 3 || batch.Activity[0].Seq != 3 {
		t.Fatalf("malformed/retired events were fabricated or cursor stalled: %+v / %v", batch, err)
	}
}

func TestStateNamingActivityReportsMalformedTransitionFields(t *testing.T) {
	binding := ports.ActivityBinding{ProjectIdentity: "project", ExecutionWorkflowRunID: "run", ExecutionID: "execution", AttemptExecutionID: "execution", AttemptID: "attempt"}
	to := map[string]any{"workflow_stage": "publish", "workflow_status": "blocked"}
	for _, tc := range []struct {
		name string
		meta map[string]any
	}{
		{"malformed source stage", map[string]any{"from": map[string]any{"workflow_stage": 42}, "to": to}},
		{"malformed source resource", map[string]any{"from": map[string]any{"resource_status": []string{"idle"}}, "to": to}},
		{"malformed target version", map[string]any{"to": map[string]any{"workflow_stage": "publish", "workflow_status": "blocked", "input_version": "bad"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(api.ActivityEvent{Kind: "worker_state", Meta: tc.meta})
			event := ports.DomainEvent{ProjectIdentity: "project", WorkflowRunID: "run", ExecutionID: "execution", AttemptID: "attempt", EventID: 1, Type: "activity.worker_state", Payload: raw}
			batch, err := ProjectActivity("project", "run", "execution", "attempt", api.ActivityCursor{}, binding, []ports.DomainEvent{event})
			if err != nil || len(batch.Activity) != 0 || !strings.Contains(strings.Join(batch.Diagnostics, "\n"), "字段损坏") || batch.Cursor.AfterEventID != 1 {
				t.Fatalf("malformed transition disappeared without diagnostics: %+v / %v", batch, err)
			}
		})
	}
	// Reservation has no previous stage, so an empty source is a valid initial transition.
	raw, _ := json.Marshal(api.ActivityEvent{Kind: "worker_state", Meta: map[string]any{"from": map[string]any{}, "to": to}})
	event := ports.DomainEvent{ProjectIdentity: "project", WorkflowRunID: "run", ExecutionID: "execution", AttemptID: "attempt", EventID: 1, Type: "activity.worker_state", Payload: raw}
	batch, err := ProjectActivity("project", "run", "execution", "attempt", api.ActivityCursor{}, binding, []ports.DomainEvent{event})
	if err != nil || len(batch.Activity) != 1 || len(batch.Diagnostics) != 0 {
		t.Fatalf("valid initial transition rejected: %+v / %v", batch, err)
	}
}

func TestTM05CanonicalActivityIdentityAndNoRawFallback(t *testing.T) {
	binding := ports.ActivityBinding{
		ProjectIdentity: "project", ExecutionWorkflowRunID: "run", ExecutionID: "execution",
		AttemptExecutionID: "execution", AttemptID: "attempt",
	}
	cursor := api.ActivityCursor{}
	batch, err := ProjectActivity("project", "run", "execution", "attempt", cursor, binding, nil)
	if err != nil || len(batch.Activity) != 0 || len(batch.Diagnostics) == 0 || !strings.Contains(batch.Diagnostics[0], "不会从原始日志补画") {
		t.Fatalf("empty canonical data was fabricated: %+v / %v", batch, err)
	}
	payload, _ := json.Marshal(api.ActivityEvent{Source: "center", Kind: "worker", Title: "canonical"})
	event := ports.DomainEvent{ProjectIdentity: "project", WorkflowRunID: "run", ExecutionID: "execution", AttemptID: "attempt", EventID: 4, Type: "activity.worker", Payload: payload}
	batch, err = ProjectActivity("project", "run", "execution", "attempt", cursor, binding, []ports.DomainEvent{event})
	if err != nil || len(batch.Activity) != 1 || batch.Activity[0].Title != "canonical" || batch.Activity[0].Seq != 4 || batch.Cursor.AfterEventID != 4 {
		t.Fatalf("canonical identity/order changed: %+v / %v", batch, err)
	}
	if _, err = ProjectActivity("project", "run", "execution", "other", api.ActivityCursor{RunID: "run", ExecutionID: "execution", AttemptID: "other", Initialized: true}, binding, []ports.DomainEvent{event}); err == nil {
		t.Fatal("cursor retargeted a canonical activity read")
	}
	foreign := event
	foreign.AttemptID = "other"
	if _, err = ProjectActivity("project", "run", "execution", "attempt", cursor, binding, []ports.DomainEvent{foreign}); err == nil {
		t.Fatal("foreign canonical event was accepted")
	}
}
