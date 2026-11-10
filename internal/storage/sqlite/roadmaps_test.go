package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestRoadmapStorePrivateBindingCASAndRestart(t *testing.T) {
	t.Run("document executions", testRoadmapExecutionsStore)
	file := filepath.Join(t.TempDir(), "state.db")
	store, e := OpenSQLiteStore(file)
	require(t, e)
	ctx := context.Background()
	source := roadmap.Source{ID: roadmap.SourceID("p", roadmap.Root+"one.md"), Path: roadmap.Root + "one.md", Body: "# Roadmap"}
	source.Digest = roadmap.Digest([]byte(source.Body))
	r := roadmap.Record{ID: strings.Repeat("a", 32), Project: "p", RequestID: strings.Repeat("a", 32), Revision: 1, CreatedAt: time.Now().UTC(), Source: source, State: "queued", Submission: "pending", GraphDigest: "graph", Profile: "local", Runner: runnercontract.BuildPin{StateDir: "original-dir", SHA256: "original-exe"}, Request: runnercontract.Request{RequestID: "runner-original", Input: json.RawMessage(`{"title":"original"}`)}}
	require(t, store.SaveRoadmap(ctx, r, 0))
	r.State = "unknown"
	r.Submission = "started"
	r.Revision++
	require(t, store.SaveRoadmap(ctx, r, 1))
	require(t, store.Close())
	store, e = OpenSQLiteStore(file)
	require(t, e)
	defer store.Close()
	got, e := store.Roadmap(ctx, "p", r.ID)
	require(t, e)
	if got.Request.RequestID != r.Request.RequestID || got.Runner != r.Runner || got.Source.Body != source.Body || got.Submission != "started" {
		t.Fatal(got)
	}
	if _, e = store.Roadmap(ctx, "different", r.ID); e == nil {
		t.Fatal("project read leaked")
	}
	got.Revision++
	got.Submission = "pending"
	if e = store.SaveRoadmap(ctx, got, 2); e == nil {
		t.Fatal("started request returned to pending")
	}
	got.Submission = "started"
	got.Source.Body = "mutated"
	if e = store.SaveRoadmap(ctx, got, 2); e == nil {
		t.Fatal("source changed while keeping its digest")
	}
	got.Source.Body = source.Body
	got.Request.Input = json.RawMessage(`{"title":"replacement"}`)
	if e = store.SaveRoadmap(ctx, got, 2); e == nil {
		t.Fatal("immutable runner request replaced")
	}
}

func testRoadmapExecutionsStore(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "state.db")
	store, err := OpenSQLiteStore(file)
	require(t, err)
	r := roadmap.Record{ID: strings.Repeat("b", 32), Project: "p", Purpose: "documents", RequestID: "batch", Revision: 1,
		Generations: []roadmap.Generation{{ID: strings.Repeat("c", 32), Key: "T1", CreatedAt: time.Now().UTC(), State: "accepted", Submission: "started", Request: runnercontract.Request{RequestID: "original", Input: json.RawMessage(`{"secret":"original input"}`)}, RunID: "run", InputDigest: "digest", Projection: &runnercontract.Run{Revision: 2}}}}
	require(t, store.SaveRoadmap(ctx, r, 0))
	r.Generations = append(r.Generations, roadmap.Generation{ID: strings.Repeat("d", 32), Key: "T1", CreatedAt: time.Now().UTC(), RetryRequest: "retry", State: "queued", Submission: "pending", Request: runnercontract.Request{RequestID: "retry-original", Input: r.Generations[0].Request.Input}})
	r.Revision++
	require(t, store.SaveRoadmap(ctx, r, 1))
	require(t, store.Close())
	store, err = OpenSQLiteStore(file)
	require(t, err)
	defer store.Close()
	r, err = store.RoadmapRequest(ctx, "p", "retry")
	require(t, err)
	if len(r.Generations) != 2 || r.Generations[0].Request.RequestID != "original" || string(r.Generations[1].Request.Input) != `{"secret":"original input"}` {
		t.Fatal("original execution lost on reopen")
	}
	wire, err := json.Marshal(r)
	require(t, err)
	if strings.Contains(string(wire), "original input") || strings.Contains(string(wire), "runner_request") {
		t.Fatal("private requests exposed")
	}
	for _, mode := range []string{"request", "run", "input", "time", "history", "projection", "duplicate", "cas"} {
		t.Run(mode, func(t *testing.T) {
			changed, err := store.Roadmap(ctx, "p", r.ID)
			require(t, err)
			changed.Revision++
			g := &changed.Generations[0]
			switch mode {
			case "request":
				g.Request.RequestID = "replacement"
			case "run":
				g.RunID = "replacement"
			case "input":
				g.InputDigest = "replacement"
			case "time":
				g.CreatedAt = g.CreatedAt.Add(time.Second)
			case "history":
				changed.Generations = changed.Generations[1:]
			case "projection":
				g.Projection.Revision--
			case "duplicate":
				changed.Generations = append(changed.Generations, *g)
			case "cas":
				changed.Revision++
			}
			if err = store.SaveRoadmap(ctx, changed, r.Revision); err == nil {
				t.Fatal("immutable execution changed")
			}
		})
	}
}
