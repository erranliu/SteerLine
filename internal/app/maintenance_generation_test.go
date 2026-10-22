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
	// Two tasks share exactly the same generator inputs: one generation item.
	duplicate := store.graph.Tasks[0]
	duplicate.TaskID = "102"
	store.graph.Tasks = append(store.graph.Tasks, duplicate)
	if _, err := m.check(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if cli.submits != 1 || len(store.batch.Items) != 1 {
		t.Fatal("shared input not deduplicated", store.batch)
	}
	// A completed item remains in the same active batch across another click.
	completed := store.generations[store.materials["101"].GenerationID]
	completed.ID, completed.Inputs, completed.State = "previous", "previous-input", "generated"
	completed.Projection = &runnercontract.Run{StopStatus: "stopped"}
	store.generations[completed.ID] = completed
	store.batch.Items = append(store.batch.Items, pm.GenerationBatchItem{TaskID: "previous-task", Inputs: completed.Inputs, GenerationID: completed.ID})
	id := strings.Repeat("d", 32)
	p, err := m.generateAll(ctx, id)
	if err != nil || p.Total != 2 || p.Running != 1 || p.Succeeded != 1 {
		t.Fatal(p, err)
	}
	if _, err = m.generateAll(ctx, id); err != nil || cli.submits != 1 {
		t.Fatal("batch replay submitted", err)
	}
	cli.finish()
	if err = m.tests.service.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	m.observation.Diagnostic = "remote temporarily offline"
	if err = m.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	batch := store.batch
	p = m.batchProgress(ctx, batch)
	if p.Succeeded != 2 || p.CompletedAt == nil {
		t.Fatal(p)
	}
	if err = m.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if !store.batch.CompletedAt.Equal(*batch.CompletedAt) {
		t.Fatal("completion timestamp restarted")
	}
	if _, err := m.generateAll(ctx, id); err != nil || !store.batch.CompletedAt.Equal(*batch.CompletedAt) {
		t.Fatal("lost outer receipt reset the completed display window", err)
	}
	reopened := newProjectMaintenance(m.config, store)
	p = reopened.batchProgress(ctx, store.batch)
	if p.Succeeded != 2 || !p.CompletedAt.Equal(*batch.CompletedAt) {
		t.Fatal("page/restart lost progress", p)
	}
	if _, err = m.generateAll(ctx, strings.Repeat("e", 32)); err != nil || cli.submits != 1 {
		t.Fatal("ready item regenerated", err)
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
	if err != nil || p.Running != 1 || cli.submits != 2 {
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
