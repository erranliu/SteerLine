package app

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

type generationBatchStore interface {
	GenerationBatch(context.Context, string) (pm.GenerationBatch, error)
	SaveGenerationBatch(context.Context, string, pm.GenerationBatch) error
	SaveMaintenanceOperation(context.Context, string, string, []byte) error
}

func (m *projectMaintenance) retryable(ctx context.Context, material pm.Material) bool {
	switch material.Status {
	case "failed", "invalid", "rejected", "cancelled":
		if material.Inputs == "" {
			return false
		}
		g, err := m.store.Generation(ctx, m.config.Identity(), material.GenerationID)
		return errors.Is(err, testmanagement.ErrNotFound) || err == nil && g.Settled()
	}
	return false
}

func (m *projectMaintenance) retryGeneration(ctx context.Context, taskID, id string) (testmanagement.Generation, error) {
	store := m.store.(materialStore)
	material, err := store.Material(ctx, m.config.Identity(), taskID)
	if err != nil {
		return testmanagement.Generation{}, err
	}
	if material.GenerationID != id && !m.retryable(ctx, material) {
		return testmanagement.Generation{}, errors.New("原生成尚未明确失败或停止/资源仍待核实")
	}
	material.GenerationID, material.Status, material.Diagnostic = id, "pending", ""
	if err = store.SaveMaterial(ctx, m.config.Identity(), material); err != nil {
		return testmanagement.Generation{}, err
	}
	profile := store.MaterialProfile(ctx, m.config.Identity())
	if profile == "" {
		profile = m.config.RunnerExecution.DefaultProfile
	}
	g, err := m.tests.service.Generate(ctx, testmanagement.GenerateCommand{RequestID: id, Plan: material.Contract.PlanRef.Path, Profile: profile, Commit: material.Commit})
	material.Status = g.State
	if err != nil {
		material.Diagnostic = err.Error()
		material.Status = "failed"
		if saved, e := m.store.Generation(ctx, m.config.Identity(), id); e == nil {
			material.Status = saved.State
		}
	}
	if saveErr := store.SaveMaterial(ctx, m.config.Identity(), material); saveErr != nil {
		return g, saveErr
	}
	return g, err
}

func (m *projectMaintenance) batchItems(ctx context.Context) ([]pm.GenerationBatchItem, error) {
	graph, err := m.store.TaskGraphSnapshot(ctx, m.config.Identity())
	if err != nil {
		return nil, err
	}
	sort.Slice(graph.Tasks, func(i, j int) bool { return graph.Tasks[i].TaskID < graph.Tasks[j].TaskID })
	items := []pm.GenerationBatchItem{}
	seen := map[string]bool{}
	for _, task := range graph.Tasks {
		if task.Status != "open" && task.Status != "in_progress" {
			continue
		}
		mat, err := m.store.(materialStore).Material(ctx, m.config.Identity(), task.TaskID)
		if err != nil {
			return nil, err
		}
		if mat.Status == "ready" {
			continue
		}
		key := mat.Contract.PlanRef.Path + "\n" + mat.Inputs
		if mat.Inputs == "" {
			key = task.TaskID
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, pm.GenerationBatchItem{TaskID: mat.TaskID, Inputs: mat.Inputs, GenerationID: mat.GenerationID, Status: mat.Status, Diagnostic: mat.Diagnostic})
	}
	return items, nil
}

func (m *projectMaintenance) batchProgress(ctx context.Context, batch pm.GenerationBatch) pm.GenerationProgress {
	p := pm.GenerationProgress{ID: batch.ID, Total: len(batch.Items), CompletedAt: batch.CompletedAt}
	for _, item := range batch.Items {
		state := item.Status
		g, err := m.store.Generation(ctx, m.config.Identity(), item.GenerationID)
		if err == nil && g.Inputs == item.Inputs {
			state = g.State
		} else if mat, e := m.store.(materialStore).Material(ctx, m.config.Identity(), item.TaskID); e == nil && mat.Inputs == item.Inputs && mat.GenerationID == item.GenerationID {
			state = mat.Status
		} else {
			state = "blocked"
		}
		switch state {
		case "ready", "generated":
			p.Succeeded++
		case "queued":
			p.Queued++
		case "accepted":
			p.Running++
		case "failed", "invalid", "rejected", "cancelled":
			p.Failed++
		default:
			p.Blocked++
		}
	}
	return p
}

// syncGenerationBatch runs only in the project actor. Queries never create
// batches or reset completion times. Membership stays fixed while work runs.
func (m *projectMaintenance) completeGenerationBatch(ctx context.Context) error {
	store, ok := m.store.(generationBatchStore)
	if !ok {
		return errors.New("generation batch store unavailable")
	}
	batch, err := store.GenerationBatch(ctx, m.config.Identity())
	if err != nil && !errors.Is(err, storageports.ErrNotFound) {
		return err
	}
	progress := m.batchProgress(ctx, batch)
	if progress.Total > 0 && progress.Total == progress.Succeeded && batch.CompletedAt == nil {
		now := time.Now().UTC()
		batch.CompletedAt = &now
		if err := store.SaveGenerationBatch(ctx, m.config.Identity(), batch); err != nil {
			return err
		}
	}
	return nil
}

func (m *projectMaintenance) syncGenerationBatch(ctx context.Context) error {
	if err := m.completeGenerationBatch(ctx); err != nil {
		return err
	}
	store := m.store.(generationBatchStore)
	batch, err := store.GenerationBatch(ctx, m.config.Identity())
	if err != nil && !errors.Is(err, storageports.ErrNotFound) {
		return err
	}
	progress := m.batchProgress(ctx, batch)
	if progress.Queued+progress.Running > 0 {
		return nil
	}
	items, err := m.batchItems(ctx)
	if err != nil || len(items) == 0 {
		return err
	}
	raw, _ := json.Marshal(items)
	id := pm.Hash([]string{string(raw)})[:32]
	// An explicit batch has its own ID; compare membership rather than ID.
	old, _ := json.Marshal(batch.Items)
	if string(old) == string(raw) {
		return nil
	}
	return store.SaveGenerationBatch(ctx, m.config.Identity(), pm.GenerationBatch{ID: id, Items: items})
}

func (m *projectMaintenance) generateAll(ctx context.Context, id string) (pm.GenerationProgress, error) {
	store := m.store.(generationBatchStore)
	var batch pm.GenerationBatch
	// Retain the fixed intent independently of the current progress projection,
	// so a lost outer receipt cannot broaden or replay a batch against new input.
	raw, err := m.store.MaintenanceReceipt(ctx, m.config.Identity(), id)
	if err == nil {
		if err = json.Unmarshal(raw, &batch); err != nil {
			return pm.GenerationProgress{}, err
		}
		if batch.Processed {
			return m.batchProgress(ctx, batch), nil
		}
	} else {
		if !errors.Is(err, storageports.ErrNotFound) {
			return pm.GenerationProgress{}, err
		}
		if _, err = m.check(ctx); err != nil {
			return pm.GenerationProgress{}, err
		}
		if err = m.refresh(ctx); err != nil {
			return pm.GenerationProgress{}, err
		}
		items, err := m.batchItems(ctx)
		if err != nil {
			return pm.GenerationProgress{}, err
		}
		for i := range items {
			mat, err := m.store.(materialStore).Material(ctx, m.config.Identity(), items[i].TaskID)
			if err != nil {
				return pm.GenerationProgress{}, err
			}
			if m.retryable(ctx, mat) {
				items[i].GenerationID = pm.Hash([]string{id, mat.Contract.PlanRef.Path, mat.Inputs})[:32]
				items[i].Status = "pending"
			}
		}
		current, err := store.GenerationBatch(ctx, m.config.Identity())
		if err != nil && !errors.Is(err, storageports.ErrNotFound) {
			return pm.GenerationProgress{}, err
		}
		progress := m.batchProgress(ctx, current)
		if progress.Queued+progress.Running > 0 {
			// Repeated clicks join the running batch and retain its successes.
			// Explicit retries replace only their original item references.
			merged := append([]pm.GenerationBatchItem(nil), current.Items...)
			for _, item := range items {
				found := false
				for i, old := range merged {
					if old.TaskID == item.TaskID || old.Inputs != "" && old.Inputs == item.Inputs {
						merged[i] = item
						found = true
						break
					}
				}
				if !found {
					merged = append(merged, item)
				}
			}
			items = merged
		}
		batch = pm.GenerationBatch{ID: id, Items: items}
		raw, _ = json.Marshal(batch)
		if err = store.SaveMaintenanceOperation(ctx, m.config.Identity(), id, raw); err != nil {
			return pm.GenerationProgress{}, err
		}
	}
	if len(batch.Items) == 0 {
		batch.Processed = true
		raw, _ = json.Marshal(batch)
		return pm.GenerationProgress{}, store.SaveMaintenanceOperation(ctx, m.config.Identity(), id, raw)
	}
	if err = store.SaveGenerationBatch(ctx, m.config.Identity(), batch); err != nil {
		return pm.GenerationProgress{}, err
	}
	for _, item := range batch.Items {
		if item.Status != "pending" || item.GenerationID == "" {
			continue
		}
		if _, err := m.store.Generation(ctx, m.config.Identity(), item.GenerationID); err == nil {
			continue
		}
		mat, err := m.store.(materialStore).Material(ctx, m.config.Identity(), item.TaskID)
		if err != nil {
			return pm.GenerationProgress{}, err
		}
		if mat.Inputs != item.Inputs {
			continue
		}
		// Preparation errors remain on the material and do not stop other items.
		_, _ = m.retryGeneration(ctx, item.TaskID, item.GenerationID)
	}
	batch.Processed = true
	raw, _ = json.Marshal(batch)
	if err := store.SaveMaintenanceOperation(ctx, m.config.Identity(), id, raw); err != nil {
		return pm.GenerationProgress{}, err
	}
	return m.batchProgress(ctx, batch), nil
}
