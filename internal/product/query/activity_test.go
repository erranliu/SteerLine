package query

import (
	"encoding/json"
	"strings"
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

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
