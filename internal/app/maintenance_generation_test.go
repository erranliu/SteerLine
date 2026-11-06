package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestGQBatchReusesActiveAndPersistsCompletion(t *testing.T) {
	ctx := context.Background()
	m, store, cli := pmsFixture(t)
	duplicate := store.graph.Tasks[0]
	duplicate.TaskID = "102"
	store.graph.Tasks = append(store.graph.Tasks, duplicate)
	id := strings.Repeat("d", 32)
	p, err := m.generateAll(ctx, id)
	if err != nil || p.Total != 2 || p.Status != "queued" || cli.submits != 0 || len(store.materials) != 0 {
		t.Fatal("registration performed preparation inline", p, err)
	}
	if err = m.processPreparation(ctx); err != nil {
		t.Fatal(err)
	}
	if cli.submits != 1 {
		t.Fatal("shared Plan was not deduplicated", cli.submits)
	}
	p = m.batchProgress(ctx, store.batch)
	if p.Running != 2 {
		t.Fatal(p)
	}
	if _, err = m.generateAll(ctx, id); err != nil || cli.submits != 1 {
		t.Fatal("batch replay submitted", err)
	}
	cli.finish()
	if err = m.processPreparation(ctx); err != nil {
		t.Fatal(err)
	}
	batch := store.batch
	p = m.batchProgress(ctx, batch)
	if p.Succeeded != 2 || p.CompletedAt == nil || p.Status != "completed" {
		t.Fatal(p)
	}
	if err = m.processPreparation(ctx); err != nil {
		t.Fatal(err)
	}
	if !store.batch.CompletedAt.Equal(*batch.CompletedAt) {
		t.Fatal("completion timestamp restarted")
	}
	if _, err = m.generateAll(ctx, id); err != nil || !store.batch.CompletedAt.Equal(*batch.CompletedAt) {
		t.Fatal("replay reset completion", err)
	}
	reopened := newProjectMaintenance(m.config, store)
	p = reopened.batchProgress(ctx, store.batch)
	if p.Succeeded != 2 || !p.CompletedAt.Equal(*batch.CompletedAt) {
		t.Fatal("restart lost progress", p)
	}
	if _, err = m.generateAll(ctx, strings.Repeat("e", 32)); err != nil {
		t.Fatal(err)
	}
	if err = m.processPreparation(ctx); err != nil || cli.submits != 1 {
		t.Fatal("ready item regenerated", err)
	}
}

func TestGQOriginalObservationRecoveryCompletesOnlyCurrentSource(t *testing.T) {
	m, store, _ := pmsFixture(t)
	m.observation.Commit = "source"
	store.materials["101"] = pm.Material{TaskID: "101", Commit: "source", Status: "ready", Inputs: "inputs", GenerationID: "generation"}
	batch := pm.GenerationBatch{ID: "batch", Commit: "source", Status: "failed", Diagnostic: "original result was unknown", Items: []pm.GenerationBatchItem{{TaskID: "101", Inputs: "inputs", GenerationID: "generation"}}}
	store.batch = batch
	if err := m.completeGenerationBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.batch.Status != "completed" || store.batch.Diagnostic != "" || store.batch.CompletedAt == nil {
		t.Fatal("recovered original generation still looked failed", store.batch)
	}
	store.batch = batch
	m.observation.Commit = "new-source"
	if err := m.completeGenerationBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.batch.Status != "failed" || store.batch.CompletedAt != nil {
		t.Fatal("stale source was declared completed", store.batch)
	}
}

func TestGQPreparationFailureDoesNotLookQueued(t *testing.T) {
	ctx := context.Background()
	m, store, cli := pmsFixture(t)
	m.tests.service.Prepare = func(context.Context, string) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
		return runnercontract.ExecutionInput{}, nil, errors.New("catalogue unavailable")
	}
	if _, err := m.check(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if store.materials["101"].Status != "failed" || cli.submits != 0 {
		t.Fatal("unsaved request appeared queued", store.materials["101"])
	}
	p := m.batchProgress(ctx, store.batch)
	if p.Failed != 1 || p.Queued != 0 {
		t.Fatal(p)
	}
}

func TestGQBatchRetriesSettledFailureOnceAndBlocksUnknown(t *testing.T) {
	ctx := context.Background()
	m, store, cli := pmsFixture(t)
	if _, err := m.check(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	cli.finish()
	cli.run.ContentResult.JSON = []byte(`{"tests":["does-not-exist"]}`)
	if err := m.tests.service.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := m.generateAll(ctx, strings.Repeat("b", 32))
	if err != nil {
		t.Fatal(err)
	}
	if err = m.processPreparation(ctx); err != nil {
		t.Fatal(err)
	}
	p = m.batchProgress(ctx, store.batch)
	if p.Running != 1 || cli.submits != 2 {
		t.Fatal(p, cli.submits, err)
	}
	if _, err = m.generateAll(ctx, strings.Repeat("b", 32)); err != nil || cli.submits != 2 {
		t.Fatal("duplicate batch retry", err)
	}
	mat := store.materials["101"]
	g := store.generations[mat.GenerationID]
	g.State, g.RunID, g.Projection = "unknown", "", nil
	store.generations[g.ID] = g
	mat.Status = "unknown"
	store.materials["101"] = mat
	p = m.batchProgress(ctx, pm.GenerationBatch{ID: "unknown", Items: []pm.GenerationBatchItem{{TaskID: mat.TaskID, Inputs: mat.Inputs, GenerationID: g.ID}}})
	if p.Blocked != 1 || p.Succeeded != 0 {
		t.Fatal("unknown displayed as success", p)
	}
	if _, err = m.generateAll(ctx, strings.Repeat("c", 32)); err != nil || cli.submits != 2 {
		t.Fatal("unknown retried", err)
	}
}
