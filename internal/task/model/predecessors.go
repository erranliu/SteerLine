package model

import (
	"sort"
)

func NormalizePredecessorIDs(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, newError("invalid_batch", "at least one predecessor is required")
	}
	out := append([]string(nil), ids...)
	for _, id := range out {
		if err := ValidateID(id); err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) < len(out[j])
		}
		return out[i] < out[j]
	})
	for i := 1; i < len(out); i++ {
		if out[i] == out[i-1] {
			return nil, newError("duplicate_dependency", "predecessor appears more than once")
		}
	}
	return out, nil
}

func ValidatePredecessorAddition(graph Snapshot, taskID string, predecessorIDs []string) error {
	successor, exists := graph.ByID[taskID]
	if !exists {
		return newError("task_not_found", "successor is not present in the native project graph")
	}
	_, delivered := graph.Deliveries[taskID]
	if successor.Status == "closed" || delivered {
		return newError("already_merged", "delivered task cannot acquire a new predecessor")
	}
	if successor.Status == "discarded" {
		return newError("task_discarded", "discarded task cannot acquire a new predecessor")
	}
	edges := append([]TaskDependencyRecord(nil), graph.Edges...)
	seen := map[string]bool{}
	for _, edge := range graph.Edges {
		seen[edge.PredecessorID+"\x00"+edge.SuccessorID] = true
	}
	for _, predecessorID := range predecessorIDs {
		if predecessorID == taskID {
			return newError("dependency_cycle", "task cannot depend on itself")
		}
		predecessor, found := graph.ByID[predecessorID]
		if !found {
			if graph.RepositoryTaskIDs[predecessorID] {
				return newError("cross_project_dependency", "predecessor belongs to another native project")
			}
			return newError("missing_predecessor", "predecessor is not present in the native project graph")
		}
		if predecessor.Status == "discarded" {
			return newError("discarded_predecessor", "discarded task cannot become a new predecessor")
		}
		key := predecessorID + "\x00" + taskID
		if seen[key] {
			return newError("duplicate_dependency", "dependency already exists")
		}
		seen[key] = true
		edges = append(edges, TaskDependencyRecord{ProjectIdentity: graph.Authority.ProjectIdentity, PredecessorID: predecessorID, SuccessorID: taskID})
	}
	if len(dependencyCycle(edges)) != 0 {
		return newError("dependency_cycle", "new predecessor would create a dependency cycle")
	}
	return nil
}
