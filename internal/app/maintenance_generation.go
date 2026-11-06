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
	RegisterPreparation(context.Context, string, string, pm.GenerationBatch) error
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

func (m *projectMaintenance) retryGenerationProfile(ctx context.Context, taskID, id, profile string) (testmanagement.Generation, error) {
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
	if profile == "" {
		profile = store.MaterialProfile(ctx, m.config.Identity())
	}
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
	p := pm.GenerationProgress{ID: batch.ID, Status: batch.Status, Diagnostic: batch.Diagnostic, Total: len(batch.Items), CompletedAt: batch.CompletedAt}
	for _, item := range batch.Items {
		state := item.Status
		if batch.Status == "queued" || batch.Status == "preparing" {
			p.Queued++
			continue
		}
		if mat, e := m.store.(materialStore).Material(ctx, m.config.Identity(), item.TaskID); e == nil && mat.Inputs == item.Inputs && mat.GenerationID == item.GenerationID {
			state = mat.Status
		} else if g, err := m.store.Generation(ctx, m.config.Identity(), item.GenerationID); err == nil && g.Inputs == item.Inputs && g.State != "generated" {
			state = g.State
		} else {
			state = "blocked"
		}
		switch state {
		case "ready":
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

// syncGenerationBatch runs only during maintenance work. Queries never create
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
		if batch.Status != "" {
			observed := m.current()
			if batch.Commit == "" || observed.Commit != batch.Commit || observed.Diagnostic != "" {
				return nil
			}
			batch.Status, batch.Diagnostic = "completed", ""
		}
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
	if batch.Status != "" || progress.Queued+progress.Running > 0 {
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

// A click records fixed task membership before any Git or Runner work. The
// maintenance worker owns preparation and can resume this intent after restart.
func (m *projectMaintenance) generateAll(ctx context.Context, id string) (pm.GenerationProgress, error) {
	return m.requestPreparation(ctx, id, "")
}

func (m *projectMaintenance) requestPreparation(ctx context.Context, id, taskID string) (pm.GenerationProgress, error) {
	store := m.store.(generationBatchStore)
	if raw, err := m.store.MaintenanceReceipt(ctx, m.config.Identity(), id); err == nil {
		var original pm.GenerationBatch
		if err = json.Unmarshal(raw, &original); err != nil {
			return pm.GenerationProgress{}, err
		}
		current, readErr := store.GenerationBatch(ctx, m.config.Identity())
		if readErr != nil && !errors.Is(readErr, storageports.ErrNotFound) {
			return pm.GenerationProgress{}, readErr
		}
		if readErr == nil && current.ID == original.ID {
			original = current
		}
		return m.batchProgress(ctx, original), nil
	} else if !errors.Is(err, storageports.ErrNotFound) {
		return pm.GenerationProgress{}, err
	}
	current, err := store.GenerationBatch(ctx, m.config.Identity())
	if err != nil && !errors.Is(err, storageports.ErrNotFound) {
		return pm.GenerationProgress{}, err
	}
	if current.Status == "queued" || current.Status == "preparing" || current.Status == "generating" {
		current.Processed = true
		raw, _ := json.Marshal(current)
		return m.batchProgress(ctx, current), store.SaveMaintenanceOperation(ctx, m.config.Identity(), id, raw)
	}
	graph, err := m.store.TaskGraphSnapshot(ctx, m.config.Identity())
	if err != nil {
		return pm.GenerationProgress{}, err
	}
	profile := m.store.(materialStore).MaterialProfile(ctx, m.config.Identity())
	if profile == "" {
		profile = m.config.RunnerExecution.DefaultProfile
	}
	batch := pm.GenerationBatch{ID: id, Profile: profile, Status: "queued", Processed: true, Items: []pm.GenerationBatchItem{}}
	if selected, err := m.config.RunnerExecution.Profile(profile, ""); err == nil {
		batch.ProfileDigest = selected.Digest()
	}
	for _, task := range graph.Tasks {
		if task.Status != "open" && task.Status != "in_progress" || taskID != "" && task.TaskID != taskID {
			continue
		}
		// Include ready tasks too: the worker first checks the remote and reuses
		// their existing result if its inputs are unchanged.
		item := pm.GenerationBatchItem{TaskID: task.TaskID, Status: "pending"}
		mat, readErr := m.store.(materialStore).Material(ctx, m.config.Identity(), task.TaskID)
		if readErr != nil && !errors.Is(readErr, storageports.ErrNotFound) {
			return pm.GenerationProgress{}, readErr
		}
		if readErr == nil {
			item.Inputs, item.GenerationID, item.Status = mat.Inputs, mat.GenerationID, mat.Status
		}
		batch.Items = append(batch.Items, item)
	}
	if len(batch.Items) == 0 {
		if taskID != "" {
			return pm.GenerationProgress{}, storageports.ErrNotFound
		}
		batch.Status = "completed"
	}
	if err = store.RegisterPreparation(ctx, m.config.Identity(), id, batch); err != nil {
		return pm.GenerationProgress{}, err
	}
	return m.batchProgress(ctx, batch), nil
}
