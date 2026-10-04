package steerlineclient

import (
	"context"
	"encoding/json"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSimplifiedCanonicalActivityNeverFallsBackToRawFiles(t *testing.T) {
	r := &readerFixture{domain: contracts.DomainSnapshot{Executions: []contracts.ExecutionRecord{{ExecutionID: "exec", WorkflowRunID: "run"}}, Attempts: []contracts.StepAttemptRecord{{ExecutionID: "exec", AttemptID: "attempt", EventPath: "events.jsonl"}}}}
	c := testClient(t, api.Resources{OpenReader: func(context.Context) (api.Reader, error) { return r, nil }})
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"thread.started","thread_id":"raw-only"}`), 0600); err != nil {
		t.Fatal(err)
	}
	batch, err := c.Activity(context.Background(), "run", "exec", "attempt", api.ActivityCursor{})
	if err != nil || len(batch.Activity) != 0 || len(batch.Diagnostics) == 0 || !strings.Contains(batch.Diagnostics[0], "数据库") {
		t.Fatalf("raw file became product data: %+v / %v", batch, err)
	}
	payload, _ := json.Marshal(contracts.ActivityEvent{Source: "center", Kind: "worker", Title: "canonical"})
	r.activity = []contracts.DomainEvent{{EventID: 4, ProjectIdentity: c.Project().Identity, WorkflowRunID: "run", ExecutionID: "exec", AttemptID: "attempt", Type: "activity.worker", Payload: payload}}
	batch, err = c.Activity(context.Background(), "run", "exec", "attempt", batch.Cursor)
	if err != nil || len(batch.Activity) != 1 || batch.Activity[0].Title != "canonical" || batch.Cursor.AfterEventID != 4 {
		t.Fatalf("canonical page: %+v / %v", batch, err)
	}
	next, err := c.Activity(context.Background(), "run", "exec", "attempt", batch.Cursor)
	if err != nil || len(next.Activity) != 0 {
		t.Fatalf("repeated activity: %+v / %v", next, err)
	}
	r.activity[0].AttemptID = "other"
	if _, err := c.Activity(context.Background(), "run", "exec", "attempt", api.ActivityCursor{}); err == nil {
		t.Fatal("foreign canonical event accepted")
	}
}
