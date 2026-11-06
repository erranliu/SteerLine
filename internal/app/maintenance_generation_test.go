package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestGQBlockedGenerationRemainsObservedAndCannotRetryBeforeSettlement(t *testing.T) {
	ctx := context.Background()
	m, store, cli := pmsFixture(t)
	_, err := m.generateAll(ctx, strings.Repeat("a", 32))
	require(t, err)
	require(t, m.processPreparation(ctx))
	cli.run.Status, cli.run.Phase = "blocked", "manual_intervention_required"
	cli.run.Diagnostic = "Original operation reconciliation deadline exhausted"
	cli.run.Revision++
	for range 2 {
		require(t, m.processPreparation(ctx))
		p := m.batchProgress(ctx, store.batch)
		if p.Running != 1 || p.Attention != 1 || p.Status != "generating" || store.batch.CompletedAt != nil || cli.submits != 1 {
			t.Fatalf("display classification changed batch lifecycle: %+v submits=%d", p, cli.submits)
		}
	}
	assertView := func(attention, retry bool) {
		t.Helper()
		view, err := m.baselineView(ctx)
		require(t, err)
		raw, err := json.Marshal(view)
		require(t, err)
		var v struct {
			Tasks []struct {
				NeedsAttention bool `json:"needs_attention"`
				CanRetry       bool `json:"can_retry"`
				Diagnostic     string
			}
		}
		require(t, json.Unmarshal(raw, &v))
		if len(v.Tasks) != 1 || v.Tasks[0].NeedsAttention != attention || v.Tasks[0].CanRetry != retry || attention && v.Tasks[0].Diagnostic != cli.run.Diagnostic {
			t.Fatalf("incorrect task display/qualification: %s", raw)
		}
	}
	assertView(true, false)
	mat := store.materials["101"]
	if m.retryable(ctx, mat) {
		t.Fatal("blocked original allowed a retry")
	}
	// A late verified result is still received from the original Run.
	cli.finish()
	require(t, m.processPreparation(ctx))
	if p := m.batchProgress(ctx, store.batch); p.Succeeded != 1 || p.Attention != 0 || p.Status != "completed" || cli.submits != 1 {
		t.Fatalf("original late result was lost: %+v", p)
	}
	assertView(false, false)
	// Cancellation intent alone must not enable the retry button.
	g := store.generations[mat.GenerationID]
	g.State, g.Cancelled, g.Projection.StopStatus = "cancelled", true, ""
	store.generations[g.ID] = g
	mat.Status = "cancelled"
	store.materials[mat.TaskID] = mat
	assertView(false, false)
	g.Projection.StopStatus = "stopped"
	store.generations[g.ID] = g
	assertView(false, true)
}

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
