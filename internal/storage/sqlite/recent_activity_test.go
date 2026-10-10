package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestPS1RecentActivityReadsLatestTenFromExactAttempt(t *testing.T) {
	ctx := context.Background()
	store, project, tasks := nativeAuditStore(t)
	require(t, store.transactSQLite(ctx, func(tx *sqliteTx) error {
		if err := tx.PutWorkflowRun(executionmodel.WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "run", TaskID: tasks[0].TaskID, WorkflowVersion: "minimal-v1", Status: "running"}); err != nil {
			return err
		}
		if err := tx.PutExecution(executionmodel.ExecutionRecord{ProjectIdentity: project, WorkflowRunID: "run", ExecutionID: "execution", Status: "active"}); err != nil {
			return err
		}
		if err := tx.PutStepAttempt(executionmodel.StepAttemptRecord{ProjectIdentity: project, ExecutionID: "execution", AttemptID: "attempt", StepID: "worker", AttemptNumber: 1, Status: "running"}); err != nil {
			return err
		}
		for i := 0; i < 35; i++ {
			payload, _ := json.Marshal(api.ActivityEvent{Title: fmt.Sprintf("summary-%d", i)})
			event := ports.DomainEvent{ProjectIdentity: project, WorkflowRunID: "run", ExecutionID: "execution", AttemptID: "attempt", ExternalSource: "recent", ExternalID: fmt.Sprint(i), Type: "activity.worker", Payload: payload}
			if _, err := tx.AppendEvent(event); err != nil {
				return err
			}
			other := event
			other.ExternalID = fmt.Sprintf("noise-%d", i)
			other.Type = "command.accepted"
			if _, err := tx.AppendEvent(other); err != nil {
				return err
			}
		}
		return nil
	}))
	batch, err := NewQueryService(store, project).ReadRecentActivity(ctx, "run", "execution", "attempt")
	require(t, err)
	if len(batch.Activity) != 10 || batch.Activity[0].Title != "summary-25" || batch.Activity[9].Title != "summary-34" {
		t.Fatalf("recent read paged from the head or included noise: %+v", batch.Activity)
	}
	if _, err := NewQueryService(store, project).ReadRecentActivity(ctx, "other", "execution", "attempt"); err == nil {
		t.Fatal("foreign workflow accepted")
	}
	if _, err := NewQueryService(store, project).ReadRecentActivity(ctx, "run", "execution", "old-attempt"); err == nil {
		t.Fatal("foreign attempt accepted")
	}
	require(t, store.transactSQLite(ctx, func(tx *sqliteTx) error {
		for i := 0; i < 6; i++ {
			payload, _ := json.Marshal(api.ActivityEvent{Kind: "worker_state", Title: fmt.Sprintf("state-%d", i), Meta: map[string]any{"from": map[string]any{"workflow_stage": "implement.report", "workflow_status": "running"}, "to": map[string]any{"workflow_stage": "publish", "workflow_status": "queued"}}})
			_, err := tx.AppendEvent(ports.DomainEvent{ProjectIdentity: project, WorkflowRunID: "run", ExecutionID: "execution", AttemptID: "attempt", ExternalSource: "state", ExternalID: fmt.Sprint(i), Type: "activity.worker_state", Payload: payload})
			if err != nil {
				return err
			}
		}
		for i := 0; i < 20; i++ {
			payload, _ := json.Marshal(api.ActivityEvent{Kind: "worker", Title: fmt.Sprintf("tool-%d", i)})
			_, err := tx.AppendEvent(ports.DomainEvent{ProjectIdentity: project, WorkflowRunID: "run", ExecutionID: "execution", AttemptID: "attempt", ExternalSource: "tool", ExternalID: fmt.Sprint(i), Type: "activity.worker", Payload: payload})
			if err != nil {
				return err
			}
		}
		return nil
	}))
	batch, err = NewQueryService(store, project).ReadRecentActivity(ctx, "run", "execution", "attempt")
	require(t, err)
	if len(batch.Activity) != 15 || batch.Activity[0].Title != "state-1" || batch.Activity[4].Title != "state-5" || batch.Activity[14].Title != "tool-19" {
		t.Fatal("tool activity hid recent state changes", batch.Activity)
	}
	require(t, store.transactSQLite(ctx, func(tx *sqliteTx) error {
		for i := 0; i < 20; i++ {
			payload, _ := json.Marshal(api.ActivityEvent{Kind: "worker_state", Title: fmt.Sprintf("technical-%d", i), Meta: map[string]any{"from": map[string]any{"workflow_stage": "publish", "workflow_status": "queued", "current_turn_id": "old", "resource_status": "idle"}, "to": map[string]any{"workflow_stage": "publish", "workflow_status": "queued", "current_turn_id": fmt.Sprint(i), "resource_status": "pending"}}})
			if _, err := tx.AppendEvent(ports.DomainEvent{ProjectIdentity: project, WorkflowRunID: "run", ExecutionID: "execution", AttemptID: "attempt", ExternalSource: "technical", ExternalID: fmt.Sprint(i), Type: "activity.worker_state", Payload: payload}); err != nil {
				return err
			}
		}
		return nil
	}))
	batch, err = NewQueryService(store, project).ReadRecentActivity(ctx, "run", "execution", "attempt")
	require(t, err)
	if len(batch.Activity) != 20 || batch.Activity[0].Title != "state-1" || batch.Activity[4].Title != "state-5" || batch.Activity[19].Title != "technical-19" {
		t.Fatal("technical-only changes hid visible history or lost recent raw records", batch.Activity)
	}
	require(t, store.transactSQLite(ctx, func(tx *sqliteTx) error {
		for i := 0; i < 5; i++ {
			payload, _ := json.Marshal(api.ActivityEvent{Kind: "worker_state", Title: fmt.Sprintf("large-technical-%d", i), Detail: strings.Repeat("x", 400000), Meta: map[string]any{"from": map[string]any{"workflow_stage": "publish", "workflow_status": "queued", "resource_status": "idle"}, "to": map[string]any{"workflow_stage": "publish", "workflow_status": "queued", "resource_status": "pending"}}})
			if _, err := tx.AppendEvent(ports.DomainEvent{ProjectIdentity: project, WorkflowRunID: "run", ExecutionID: "execution", AttemptID: "attempt", ExternalSource: "budget", ExternalID: fmt.Sprint(i), Type: "activity.worker_state", Payload: payload}); err != nil {
				return err
			}
		}
		return nil
	}))
	_, events, err := store.ReadRecentActivityView(ctx, project, "run", "execution", "attempt")
	require(t, err)
	total, visible := 0, 0
	for _, event := range events {
		total += len(event.Payload)
		var activity api.ActivityEvent
		if json.Unmarshal(event.Payload, &activity) == nil && strings.HasPrefix(activity.Title, "state-") {
			visible++
		}
	}
	if total > 1048576 || visible != 5 {
		t.Fatal("raw payload budget displaced visible transitions", total, visible)
	}
}
