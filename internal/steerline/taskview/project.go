package taskview

import (
	"fmt"

	"github.com/erranliu/SteerLine/internal/steerline/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func viewError(code, message string) error { return &taskmodel.Error{Code: code, Message: message} }

// NormalizeFilter validates protocol limits before a snapshot is read.
func NormalizeFilter(project string, filter Filter) (Filter, error) {
	if filter.ProjectID != "" && filter.ProjectID != project {
		return Filter{}, viewError("authority_mismatch", "graph project_id must match the configured project")
	}
	filter.ProjectID = project
	if filter.Status != "" && !taskmodel.ValidStatus(filter.Status) {
		return Filter{}, viewError("invalid_filter", "status is not a supported native task status")
	}
	if filter.CenterTaskID != "" {
		if err := taskmodel.ValidateID(filter.CenterTaskID); err != nil {
			return Filter{}, err
		}
		if filter.Direction == "" {
			filter.Direction = "both"
		}
		if filter.Direction != "upstream" && filter.Direction != "downstream" && filter.Direction != "both" {
			return Filter{}, viewError("invalid_filter", "direction must be upstream, downstream or both")
		}
	} else if filter.Direction != "" {
		return Filter{}, viewError("invalid_filter", "direction requires a center task_id")
	}
	if filter.Depth < 0 {
		return Filter{}, viewError("invalid_filter", "depth cannot be negative")
	}
	if filter.CenterTaskID == "" {
		if filter.Depth > 1 {
			return Filter{}, viewError("invalid_filter", "depth requires a center task_id")
		}
		filter.Depth = 0
	} else {
		if filter.Depth == 0 {
			filter.Depth = 1
		}
		if filter.Depth > MaxDepth {
			return Filter{}, viewError("invalid_filter", fmt.Sprintf("depth cannot exceed %d", MaxDepth))
		}
	}
	if filter.Limit < 0 {
		return Filter{}, viewError("invalid_filter", "node limit cannot be negative")
	}
	if filter.Limit == 0 {
		filter.Limit = MaxNodes
	}
	if filter.Limit > MaxNodes {
		return Filter{}, viewError("invalid_filter", fmt.Sprintf("node limit cannot exceed %d", MaxNodes))
	}
	return filter, nil
}

// Project uses one verified snapshot for graph shape, delivery and readiness.
func Project(snapshot task.Snapshot, filter Filter) (Page, error) {
	byID := snapshot.ByID
	if filter.CenterTaskID != "" {
		if _, ok := byID[filter.CenterTaskID]; !ok {
			return Page{}, viewError("task_not_found", "graph center task was not found")
		}
	}
	for _, edge := range snapshot.Edges {
		if _, ok := byID[edge.PredecessorID]; !ok {
			return Page{}, viewError("contract_mismatch", "graph edge has no predecessor task")
		}
		if _, ok := byID[edge.SuccessorID]; !ok {
			return Page{}, viewError("contract_mismatch", "graph edge has no successor task")
		}
	}
	projection := taskmodel.Project(graphNodes(snapshot.Tasks), graphEdges(snapshot.Edges), taskmodel.Filter{
		Status: filter.Status, CenterID: filter.CenterTaskID, Direction: filter.Direction,
		Depth: filter.Depth, Limit: filter.Limit, MaxBoundaryEdges: MaxBoundaryEdges,
	})
	page := Page{
		Protocol: Protocol, ProjectID: filter.ProjectID,
		AuthorityVersion: snapshot.Authority.AuthorityVersion,
		GraphRevision:    snapshot.Authority.GraphRevision,
		DatabaseRevision: snapshot.DatabaseRevision,
		DatabaseID:       snapshot.DatabaseID, SampledAt: snapshot.SampledAt,
		Nodes: []Node{}, Edges: []Edge{}, Filters: filter,
		Boundary: Boundary{MaxDepth: filter.Depth, MaxNodes: filter.Limit, Reasons: []string{}, Edges: []BoundaryEdge{}},
	}
	for _, id := range projection.NodeIDs {
		record := byID[id]
		page.Nodes = append(page.Nodes, graphNode(record, snapshot))
	}
	for _, edge := range projection.Edges {
		page.Edges = append(page.Edges, Edge{Predecessor: edge.Predecessor, Successor: edge.Successor})
	}
	for _, edge := range projection.BoundaryEdges {
		page.Boundary.Edges = append(page.Boundary.Edges, BoundaryEdge{Predecessor: edge.Predecessor, Successor: edge.Successor, Reason: edge.Reason})
	}
	page.Boundary.Reasons = projection.Reasons
	page.Boundary.OmittedNodeCount = projection.OmittedNodeCount
	page.Boundary.OmittedBoundaryEdgeCount = projection.OmittedBoundaryEdgeCount
	page.Boundary.Truncated = projection.Truncated
	return page, nil
}

func graphNode(record taskmodel.TaskRecord, snapshot task.Snapshot) Node {
	_, delivered := snapshot.Deliveries[record.TaskID]
	ready, blockers := taskmodel.DependencyStateForTask(record, snapshot.TaskGraph())
	node := Node{
		TaskID: record.TaskID, Title: record.Title, Status: record.Status, Priority: record.Priority,
		DependencyReady: ready, Blockers: blockers,
		StartupBlockers: task.StartupBlockers(record.Status, delivered, snapshot.Authority.DispatchPaused, snapshot.Authority.DispatchMode, snapshot.ExecutionSlotBusy),
	}
	if delivery, ok := snapshot.Deliveries[record.TaskID]; ok {
		copy := delivery
		node.Delivery = &copy
	}
	return node
}

func graphNodes(records []taskmodel.TaskRecord) []taskmodel.Node {
	nodes := make([]taskmodel.Node, 0, len(records))
	for _, record := range records {
		nodes = append(nodes, taskmodel.Node{ID: record.TaskID, Status: record.Status, Priority: record.Priority, CreatedAt: record.CreatedAt})
	}
	return nodes
}

func graphEdges(edges []taskmodel.TaskDependencyRecord) []taskmodel.Edge {
	converted := make([]taskmodel.Edge, 0, len(edges))
	for _, edge := range edges {
		converted = append(converted, taskmodel.Edge{Predecessor: edge.PredecessorID, Successor: edge.SuccessorID})
	}
	return converted
}
