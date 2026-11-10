package app

import (
	"context"

	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	roadmapservice "github.com/erranliu/SteerLine/internal/roadmap"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// Only an admitted batch's promised tests may wait without blocking its own
// producers. The stored material remains invalid until real definitions exist.
func (m *projectMaintenance) waitingForPlannedTests(ctx context.Context, task taskmodel.TaskRecord, graph storageports.TaskGraphSnapshot, material pm.Material) (bool, error) {
	if task.Status != "open" || material.Status != "invalid" || len(material.MissingTests) == 0 {
		return false, nil
	}
	_, blockers := taskmodel.DependencyStateForTask(task, graph.TaskGraph())
	if len(blockers) == 0 {
		return false, nil
	}
	for _, blocker := range blockers {
		if blocker.Blocker != "predecessor_not_delivered" {
			return false, nil
		}
	}
	store, ok := m.store.(interface {
		Roadmaps(context.Context, string) ([]roadmap.Record, error)
	})
	if !ok {
		return false, nil
	}
	rows, err := store.Roadmaps(ctx, m.config.Identity())
	if err != nil {
		return false, err
	}
	for _, r := range rows {
		if r.State != "admitted" || roadmap.ValidateDocuments(r) != nil {
			continue
		}
		catalogue, err := roadmapservice.TestCatalogue(r)
		if err != nil || roadmapservice.ValidateTestIDs(r, catalogue) != nil {
			continue
		}
		consumer := ""
		for key, id := range r.Mapping {
			if id == task.TaskID {
				consumer = key
			}
		}
		if consumer == "" {
			continue
		}
		matches := false
		for _, d := range r.Documents {
			if d.Key == consumer && d.Plan.Path == material.Contract.PlanRef.Path && d.Spec.Path == material.Contract.SpecRef.Path {
				matches = true
			}
		}
		if !matches {
			continue
		}
		providers, _ := roadmap.TestProviders(r)
		all := true
		for _, id := range material.MissingTests {
			provider := providers[id]
			providerID := r.Mapping[provider]
			pred, exists := graph.ByID[providerID]
			_, delivered := graph.Deliveries[providerID]
			if !roadmap.IsPredecessor(r.Analysis, provider, consumer) || !exists || delivered || (pred.Status != "open" && pred.Status != "in_progress") || !graphPredecessor(graph, providerID, task.TaskID) {
				all = false
				break
			}
		}
		if all {
			return true, nil
		}
	}
	return false, nil
}

func graphPredecessor(graph storageports.TaskGraphSnapshot, provider, consumer string) bool {
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if seen[id] {
			return false
		}
		seen[id] = true
		for _, edge := range graph.Edges {
			if edge.SuccessorID == id && (edge.PredecessorID == provider || visit(edge.PredecessorID)) {
				return true
			}
		}
		return false
	}
	return provider != consumer && visit(consumer)
}
