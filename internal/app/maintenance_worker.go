package app

import (
	"context"
	"errors"
	"time"

	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// The worker borrows the project's store and pinned Runner. It never creates a
// candidate, owns a native process, or waits for an LLM on the project actor.
func (m *projectMaintenance) runPreparation(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		_ = m.processPreparation(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (m *projectMaintenance) processPreparation(ctx context.Context) error {
	store := m.store.(generationBatchStore)
	batch, err := store.GenerationBatch(ctx, m.config.Identity())
	if err != nil && !errors.Is(err, storageports.ErrNotFound) {
		return err
	}
	// Only observe already registered generation requests. A tick must never
	// register a new input or turn a failed attempt into an automatic retry.
	if m.tests != nil {
		observeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = m.tests.service.ObserveNext(observeCtx)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	if batch.Status != "queued" && batch.Status != "preparing" && batch.Status != "generating" {
		if err != nil {
			return err
		}
		return m.refreshMaterials(ctx, nil, false, "", "")
	}
	selected := map[string]bool{}
	observedAt := time.Now().UTC()
	batch.ObservedAt = &observedAt
	for _, item := range batch.Items {
		selected[item.TaskID] = true
	}
	if batch.Status == "queued" || batch.Status == "preparing" {
		if batch.ProfileDigest != "" {
			if _, err := m.config.RunnerExecution.Profile(batch.Profile, batch.ProfileDigest); err != nil {
				return m.failPreparation(ctx, batch, err)
			}
		}
		batch.Status = "preparing"
		if err = store.SaveGenerationBatch(ctx, m.config.Identity(), batch); err != nil {
			return err
		}
		if batch.Commit == "" {
			checkCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
			observed, checkErr := m.check(checkCtx)
			cancel()
			if checkErr != nil {
				return m.failPreparation(ctx, batch, checkErr)
			}
			batch.Commit = observed.Commit
			if err = store.SaveGenerationBatch(ctx, m.config.Identity(), batch); err != nil {
				return err
			}
		}
		if m.current().Commit != batch.Commit || m.current().Diagnostic != "" {
			return m.failPreparation(ctx, batch, errors.New("远端基线已变化或待核实，请重新准备材料与测试安排"))
		}
		// A request-scoped immutable blob cache is shared by material validation
		// and generation submission, and discarded when this batch finishes.
		work := materialReads(ctx)
		if err = m.refreshMaterials(work, selected, true, batch.Profile, batch.Commit); err != nil {
			return m.failPreparation(ctx, batch, err)
		}
		for _, task := range batch.Items {
			mat, readErr := m.store.(materialStore).Material(ctx, m.config.Identity(), task.TaskID)
			if readErr != nil {
				return m.failPreparation(ctx, batch, readErr)
			}
			if task.Inputs != "" && task.Inputs == mat.Inputs && task.GenerationID == mat.GenerationID && m.retryable(ctx, mat) {
				// A new click gets one new retry ID; restart/replay keeps it.
				id := pm.Hash([]string{batch.ID, mat.Contract.PlanRef.Path, mat.Inputs})[:32]
				if mat.GenerationID != id {
					_, _ = m.retryGenerationProfile(work, task.TaskID, id, batch.Profile)
				}
			}
		}
		batch.Status = "generating"
	}
	if m.current().Commit != batch.Commit || m.current().Diagnostic != "" {
		return m.failPreparation(ctx, batch, errors.New("远端基线已变化或待核实，请重新准备材料与测试安排"))
	}
	if err = m.refreshMaterials(ctx, selected, false, batch.Profile, batch.Commit); err != nil {
		return m.failPreparation(ctx, batch, err)
	}
	batch.Items = nil
	graph, err := m.store.TaskGraphSnapshot(ctx, m.config.Identity())
	if err != nil {
		return m.failPreparation(ctx, batch, err)
	}
	for _, task := range graph.Tasks {
		if !selected[task.TaskID] || task.Status != "open" && task.Status != "in_progress" {
			continue
		}
		mat, err := m.store.(materialStore).Material(ctx, m.config.Identity(), task.TaskID)
		if err != nil {
			return m.failPreparation(ctx, batch, err)
		}
		batch.Items = append(batch.Items, pm.GenerationBatchItem{TaskID: mat.TaskID, Inputs: mat.Inputs, GenerationID: mat.GenerationID, Status: mat.Status, Diagnostic: mat.Diagnostic})
	}
	progress := m.batchProgress(ctx, batch)
	if progress.Running+progress.Queued == 0 {
		batch.Status = "completed"
		if progress.Failed+progress.Blocked > 0 {
			batch.Status = "failed"
		}
		if progress.Succeeded == progress.Total {
			now := time.Now().UTC()
			batch.CompletedAt = &now
		}
		batch.ResultSnapshot = &progress
	}
	return store.SaveGenerationBatch(ctx, m.config.Identity(), batch)
}

func (m *projectMaintenance) failPreparation(ctx context.Context, batch pm.GenerationBatch, cause error) error {
	if ctx.Err() != nil {
		return cause
	} // leave the registered intent resumable
	batch.Status, batch.Diagnostic = "failed", cause.Error()
	progress := m.batchProgress(ctx, batch)
	batch.ResultSnapshot = &progress
	return errors.Join(cause, m.store.(generationBatchStore).SaveGenerationBatch(ctx, m.config.Identity(), batch))
}
