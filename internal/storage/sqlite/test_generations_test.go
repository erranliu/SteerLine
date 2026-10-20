package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

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
