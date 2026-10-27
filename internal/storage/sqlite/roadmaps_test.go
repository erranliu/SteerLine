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
