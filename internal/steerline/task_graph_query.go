package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	nativeTaskGraphProtocol   = "native-task-graph/v1"
	maxTaskGraphNodes         = 500
	maxTaskGraphDepth         = 10
	maxTaskGraphBoundaryEdges = 500
)

// TaskGraphFilter describes a read-only projection over the center task graph.
// A center is always retained as the anchor, even when it does not match Status.
type TaskGraphFilter struct {
	ProjectID    string `json:"project_id"`
	Status       string `json:"status,omitempty"`
	CenterTaskID string `json:"center_task_id,omitempty"`
	Direction    string `json:"direction,omitempty"`
	Depth        int    `json:"depth"`
	Limit        int    `json:"limit"`
}

func (f TaskGraphFilter) Filtered() bool {
	return f.Status != "" || f.CenterTaskID != ""
}

type TaskGraphBoundaryEdge struct {
	Predecessor string `json:"predecessor"`
	Successor   string `json:"successor"`
	Reason      string `json:"reason"`
}

// TaskGraphBoundary makes every omitted part explicit so consumers cannot
// mistake a bounded projection for a complete graph.
type TaskGraphBoundary struct {
	Truncated                bool                    `json:"truncated"`
	MaxDepth                 int                     `json:"max_depth"`
	MaxNodes                 int                     `json:"max_nodes"`
	OmittedNodeCount         int                     `json:"omitted_node_count"`
	OmittedBoundaryEdgeCount int                     `json:"omitted_boundary_edge_count"`
	Reasons                  []string                `json:"reasons"`
	Edges                    []TaskGraphBoundaryEdge `json:"edges"`
}

// QueryTaskGraph reads the task, dependency, delivery and readiness projection
// from one SQLite read snapshot. It never writes task or scheduler state.
func (q QueryService) QueryTaskGraph(ctx context.Context, filter TaskGraphFilter) (TaskGraphPage, error) {
	if q.Store == nil || q.Project == "" {
		return TaskGraphPage{}, errors.New("query service store and project identity are required")
	}
	if filter.ProjectID != "" && filter.ProjectID != q.Project {
		return TaskGraphPage{}, nativeErr("authority_mismatch", "graph project_id must match the configured project")
	}
	filter.ProjectID = q.Project
	if filter.Status != "" && !validTaskStatus(filter.Status) {
		return TaskGraphPage{}, nativeErr("invalid_filter", "status is not a supported native task status")
	}
	if filter.CenterTaskID != "" {
		if err := ValidateNativeTaskID(filter.CenterTaskID); err != nil {
			return TaskGraphPage{}, err
		}
		if filter.Direction == "" {
			filter.Direction = "both"
		}
		if filter.Direction != "upstream" && filter.Direction != "downstream" && filter.Direction != "both" {
			return TaskGraphPage{}, nativeErr("invalid_filter", "direction must be upstream, downstream or both")
		}
	} else if filter.Direction != "" {
		return TaskGraphPage{}, nativeErr("invalid_filter", "direction requires a center task_id")
	}
	if filter.Depth < 0 {
		return TaskGraphPage{}, nativeErr("invalid_filter", "depth cannot be negative")
	}
	if filter.CenterTaskID == "" {
		if filter.Depth > 1 {
			return TaskGraphPage{}, nativeErr("invalid_filter", "depth requires a center task_id")
		}
		filter.Depth = 0
	} else {
		if filter.Depth == 0 {
			filter.Depth = 1
		}
		if filter.Depth > maxTaskGraphDepth {
			return TaskGraphPage{}, nativeErr("invalid_filter", fmt.Sprintf("depth cannot exceed %d", maxTaskGraphDepth))
		}
	}
	if filter.Limit < 0 {
		return TaskGraphPage{}, nativeErr("invalid_filter", "node limit cannot be negative")
	}
	if filter.Limit == 0 {
		filter.Limit = maxTaskGraphNodes
	}
	if filter.Limit > maxTaskGraphNodes {
		return TaskGraphPage{}, nativeErr("invalid_filter", fmt.Sprintf("node limit cannot exceed %d", maxTaskGraphNodes))
	}

	snapshot, err := taskGraphFromStore(ctx, q.Store, q.Project)
	if err != nil {
		return TaskGraphPage{}, err
	}
	byID := snapshot.ByID
	if filter.CenterTaskID != "" {
		if _, ok := byID[filter.CenterTaskID]; !ok {
			return TaskGraphPage{}, nativeErr("task_not_found", "graph center task was not found")
		}
	}

	incoming := make(map[string][]string, len(snapshot.Tasks))
	outgoing := make(map[string][]string, len(snapshot.Tasks))
	for _, edge := range snapshot.Edges {
		if _, ok := byID[edge.PredecessorID]; !ok {
			return TaskGraphPage{}, nativeErr("contract_mismatch", "graph edge has no predecessor task")
		}
		if _, ok := byID[edge.SuccessorID]; !ok {
			return TaskGraphPage{}, nativeErr("contract_mismatch", "graph edge has no successor task")
		}
		incoming[edge.SuccessorID] = append(incoming[edge.SuccessorID], edge.PredecessorID)
		outgoing[edge.PredecessorID] = append(outgoing[edge.PredecessorID], edge.SuccessorID)
	}
	for id := range incoming {
		sortTaskIDs(incoming[id])
	}
	for id := range outgoing {
		sortTaskIDs(outgoing[id])
	}

	depths := map[string]int{}
	candidateOrder := make([]string, 0, len(snapshot.Tasks))
	depthFrontier := map[string]bool{}
	if filter.CenterTaskID == "" {
		for _, task := range snapshot.Tasks {
			candidateOrder = append(candidateOrder, task.TaskID)
		}
	} else {
		depths[filter.CenterTaskID] = 0
		reachable := []string{filter.CenterTaskID}
		for head := 0; head < len(reachable); head++ {
			id := reachable[head]
			currentDepth := depths[id]
			neighbors := graphNeighbors(id, filter.Direction, incoming, outgoing)
			for _, neighbor := range neighbors {
				if _, seen := depths[neighbor]; seen {
					continue
				}
				depths[neighbor] = currentDepth + 1
				reachable = append(reachable, neighbor)
			}
		}
		for _, id := range reachable {
			switch depths[id] {
			case filter.Depth + 1:
				depthFrontier[id] = true
			}
			if depths[id] <= filter.Depth {
				candidateOrder = append(candidateOrder, id)
			}
		}
	}

	candidate := make(map[string]bool, len(candidateOrder))
	for _, id := range candidateOrder {
		candidate[id] = true
	}
	kept := make([]string, 0, min(filter.Limit, len(candidateOrder)))
	for _, id := range candidateOrder {
		if filter.Status != "" && byID[id].Status != filter.Status && id != filter.CenterTaskID {
			continue
		}
		if len(kept) == filter.Limit {
			continue
		}
		kept = append(kept, id)
	}
	selected := make(map[string]bool, len(kept))
	for _, id := range kept {
		selected[id] = true
	}

	page := TaskGraphPage{
		Protocol:         nativeTaskGraphProtocol,
		ProjectID:        q.Project,
		AuthorityVersion: snapshot.Authority.AuthorityVersion,
		GraphRevision:    snapshot.Authority.GraphRevision,
		DatabaseRevision: snapshot.DatabaseRevision,
		DatabaseID:       snapshot.DatabaseID,
		SampledAt:        snapshot.SampledAt,
		Nodes:            []TaskGraphNode{},
		Edges:            []TaskGraphEdge{},
		Filters:          filter,
		Boundary: TaskGraphBoundary{
			MaxDepth: filter.Depth,
			MaxNodes: filter.Limit,
			Reasons:  []string{},
			Edges:    []TaskGraphBoundaryEdge{},
		},
	}
	for _, id := range kept {
		task := byID[id]
		ready, blockers := taskDependencyState(task, snapshot)
		node := TaskGraphNode{TaskID: id, Title: task.Title, Status: task.Status, Priority: task.Priority, DependencyReady: ready, Blockers: blockers, StartupBlockers: nativeStartupBlockers(task, snapshot)}
		if delivery, ok := snapshot.Deliveries[id]; ok {
			copy := delivery
			node.Delivery = &copy
		}
		page.Nodes = append(page.Nodes, node)
	}

	omittedIDs := map[string]bool{}
	reasons := map[string]bool{}
	for _, id := range candidateOrder {
		if !selected[id] {
			if filter.Status != "" && byID[id].Status != filter.Status && id != filter.CenterTaskID {
				omittedIDs[id] = true
				reasons["status_filter"] = true
			} else {
				omittedIDs[id] = true
				reasons["node_limit"] = true
			}
		}
	}
	for id, depth := range depths {
		if depth > filter.Depth {
			omittedIDs[id] = true
			reasons["depth_limit"] = true
		}
	}
	for _, edge := range snapshot.Edges {
		inCandidate := candidate[edge.PredecessorID] && candidate[edge.SuccessorID]
		frontierEdge := false
		if filter.CenterTaskID != "" {
			if depthFrontier[edge.PredecessorID] && depths[edge.SuccessorID] == filter.Depth {
				frontierEdge = filter.Direction == "upstream" || filter.Direction == "both"
			}
			if depthFrontier[edge.SuccessorID] && depths[edge.PredecessorID] == filter.Depth {
				frontierEdge = frontierEdge || filter.Direction == "downstream" || filter.Direction == "both"
			}
		}
		if !inCandidate && !frontierEdge {
			continue
		}
		predSelected, succSelected := selected[edge.PredecessorID], selected[edge.SuccessorID]
		if predSelected && succSelected {
			page.Edges = append(page.Edges, TaskGraphEdge{Predecessor: edge.PredecessorID, Successor: edge.SuccessorID})
			continue
		}
		if !predSelected && !succSelected {
			continue
		}
		omittedID := edge.PredecessorID
		if predSelected {
			omittedID = edge.SuccessorID
		}
		reason := "node_limit"
		if frontierEdge || depthFrontier[omittedID] {
			reason = "depth_limit"
		} else if filter.Status != "" && candidate[omittedID] && byID[omittedID].Status != filter.Status && omittedID != filter.CenterTaskID {
			reason = "status_filter"
		}
		if len(page.Boundary.Edges) < maxTaskGraphBoundaryEdges {
			page.Boundary.Edges = append(page.Boundary.Edges, TaskGraphBoundaryEdge{Predecessor: edge.PredecessorID, Successor: edge.SuccessorID, Reason: reason})
		} else {
			page.Boundary.OmittedBoundaryEdgeCount++
			reasons["boundary_edge_limit"] = true
		}
		reasons[reason] = true
	}
	for reason := range reasons {
		page.Boundary.Reasons = append(page.Boundary.Reasons, reason)
	}
	sort.Strings(page.Boundary.Reasons)
	page.Boundary.OmittedNodeCount = len(omittedIDs)
	page.Boundary.Truncated = len(omittedIDs) > 0 || len(page.Boundary.Edges) > 0
	return page, nil
}

func graphNeighbors(id, direction string, incoming, outgoing map[string][]string) []string {
	var neighbors []string
	switch direction {
	case "upstream":
		neighbors = append(neighbors, incoming[id]...)
	case "downstream":
		neighbors = append(neighbors, outgoing[id]...)
	case "both":
		neighbors = append(neighbors, incoming[id]...)
		neighbors = append(neighbors, outgoing[id]...)
		sortTaskIDs(neighbors)
		neighbors = compactStrings(neighbors)
	}
	return neighbors
}

func sortTaskIDs(ids []string) {
	sort.Slice(ids, func(i, j int) bool { return taskIDLess(ids[i], ids[j]) })
}

func taskIDLess(a, b string) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

type TaskGraphExportReceipt struct {
	Output           string            `json:"output"`
	ProjectID        string            `json:"project_id"`
	NodeCount        int               `json:"node_count"`
	GraphRevision    uint64            `json:"graph_revision"`
	DatabaseID       string            `json:"database_id"`
	DatabaseRevision uint64            `json:"database_revision"`
	SampledAt        string            `json:"sampled_at"`
	Overwritten      bool              `json:"overwritten"`
	Boundary         TaskGraphBoundary `json:"boundary"`
}

// WriteTaskGraphExport atomically publishes a graph in the destination
// directory. A destination without overwrite uses a hard-link publication to
// make the no-replace check race-safe; the existing file is never removed first.
func WriteTaskGraphExport(path string, graph TaskGraphPage, overwrite bool) (TaskGraphExportReceipt, error) {
	if graph.Protocol != nativeTaskGraphProtocol || graph.ProjectID == "" || graph.Filters.ProjectID != graph.ProjectID {
		return TaskGraphExportReceipt{}, nativeErr("invalid_export", "export requires a native task graph query result")
	}
	if len(graph.Nodes) > maxTaskGraphNodes || graph.Boundary.Truncated && !graph.Filters.Filtered() {
		return TaskGraphExportReceipt{}, nativeErr("export_limit_exceeded", "complete task graph exceeds the export node limit")
	}
	if strings.TrimSpace(path) == "" {
		return TaskGraphExportReceipt{}, nativeErr("invalid_output", "output path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return TaskGraphExportReceipt{}, err
	}
	existed := false
	if _, statErr := os.Lstat(absolute); statErr == nil {
		existed = true
		if !overwrite {
			return TaskGraphExportReceipt{}, nativeErr("output_exists", "output already exists; pass -overwrite to replace it")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return TaskGraphExportReceipt{}, statErr
	}
	body, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return TaskGraphExportReceipt{}, err
	}
	body = append(body, '\n')
	dir := filepath.Dir(absolute)
	base := filepath.Base(absolute)
	temp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return TaskGraphExportReceipt{}, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err = temp.Write(body); err != nil {
		_ = temp.Close()
		return TaskGraphExportReceipt{}, err
	}
	if err = temp.Sync(); err != nil {
		_ = temp.Close()
		return TaskGraphExportReceipt{}, err
	}
	if err = temp.Close(); err != nil {
		return TaskGraphExportReceipt{}, err
	}
	if overwrite {
		err = os.Rename(tempName, absolute)
	} else {
		err = os.Link(tempName, absolute)
		if err == nil {
			_ = os.Remove(tempName)
		}
	}
	if err != nil {
		if !overwrite && !errors.Is(err, os.ErrNotExist) {
			if _, statErr := os.Lstat(absolute); statErr == nil {
				return TaskGraphExportReceipt{}, nativeErr("output_exists", "output appeared before publication; existing file was preserved")
			}
		}
		return TaskGraphExportReceipt{}, err
	}
	return TaskGraphExportReceipt{
		Output: absolute, ProjectID: graph.ProjectID, NodeCount: len(graph.Nodes),
		GraphRevision: graph.GraphRevision, DatabaseID: graph.DatabaseID,
		DatabaseRevision: graph.DatabaseRevision, SampledAt: graph.SampledAt.UTC().Format(time.RFC3339Nano),
		Overwritten: existed, Boundary: graph.Boundary,
	}, nil
}
