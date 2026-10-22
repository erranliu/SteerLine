package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

func TestGQDurableBatchAndSubmissionBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	batch := pm.GenerationBatch{ID: "batch", Items: []pm.GenerationBatchItem{{TaskID: "1", GenerationID: "generation", Inputs: "input"}}, CompletedAt: &now}
	if err = s.SaveGenerationBatch(ctx, "project", batch); err != nil {
		t.Fatal(err)
	}
	if err = s.SetMaterialProfile(ctx, "project", "local"); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveMaterialObservation(ctx, "project", pm.Observation{Commit: "commit"}); err != nil {
		t.Fatal(err)
	}
	g := testmanagement.Generation{Project: "project", ID: "generation", Revision: 1, State: "queued", Submission: "pending", Request: runnercontract.Request{RequestID: "original", Input: json.RawMessage(`{}`)}}
	if err = s.SaveGeneration(ctx, g, 0); err != nil {
		t.Fatal(err)
	}
	g.Revision++
	g.State = "unknown"
	g.Submission = "started"
	if err = s.SaveGeneration(ctx, g, 1); err != nil {
		t.Fatal(err)
	}
	g.Revision++
	g.State = "queued"
	g.Submission = "pending"
	if err = s.SaveGeneration(ctx, g, 2); err == nil {
		t.Fatal("started submission returned to queue")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.GenerationBatch(ctx, "project")
	if err != nil || got.ID != batch.ID || len(got.Items) != 1 || !got.CompletedAt.Equal(now) || s.MaterialProfile(ctx, "project") != "local" {
		t.Fatal(got, err)
	}
}

func TestTMGenerationStoreCASAndImmutableRequest(t *testing.T) {
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	id := strings.Repeat("a", 32)
	r := testmanagement.Generation{Project: "p", ID: id, Revision: 1, Command: testmanagement.GenerateCommand{RequestID: id, Plan: "docs/task.plan.md", Profile: "local"}, Request: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "original-runner-request", Input: json.RawMessage(`{"title":"original"}`)}, State: "unknown"}
	if err = s.SaveGeneration(ctx, r, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Generation(ctx, "other", id); !errors.Is(err, testmanagement.ErrNotFound) {
		t.Fatal("project leaked", err)
	}
	r.Revision++
	r.State = "accepted"
	r.RunID = "run"
	if err = s.SaveGeneration(ctx, r, 1); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveGeneration(ctx, r, 1); err == nil {
		t.Fatal("stale update admitted")
	}
	r.Revision++
	r.Request.Input = json.RawMessage(`{"title":"changed"}`)
	if err = s.SaveGeneration(ctx, r, 2); err == nil {
		t.Fatal("original request changed")
	}
	got, _ := s.Generation(ctx, "p", id)
	if got.RunID != "run" || got.Revision != 2 {
		t.Fatal(got)
	}
	rows, err := s.Generations(ctx, "p")
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
}
