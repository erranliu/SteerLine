package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/taskgraph"
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

	for _, edge := range snapshot.Edges {
		if _, ok := byID[edge.PredecessorID]; !ok {
			return TaskGraphPage{}, nativeErr("contract_mismatch", "graph edge has no predecessor task")
		}
		if _, ok := byID[edge.SuccessorID]; !ok {
			return TaskGraphPage{}, nativeErr("contract_mismatch", "graph edge has no successor task")
		}
	}
	projection := taskgraph.Project(taskgraphNodes(snapshot.Tasks), taskgraphEdges(snapshot.Edges), taskgraph.Filter{
		Status: filter.Status, CenterID: filter.CenterTaskID, Direction: filter.Direction,
		Depth: filter.Depth, Limit: filter.Limit, MaxBoundaryEdges: maxTaskGraphBoundaryEdges,
	})

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
	for _, id := range projection.NodeIDs {
		task := byID[id]
		ready, blockers := taskDependencyState(task, snapshot)
		node := TaskGraphNode{TaskID: id, Title: task.Title, Status: task.Status, Priority: task.Priority, DependencyReady: ready, Blockers: blockers, StartupBlockers: nativeStartupBlockers(task, snapshot)}
		if delivery, ok := snapshot.Deliveries[id]; ok {
			copy := delivery
			node.Delivery = &copy
		}
		page.Nodes = append(page.Nodes, node)
	}
	for _, edge := range projection.Edges {
		page.Edges = append(page.Edges, TaskGraphEdge{Predecessor: edge.Predecessor, Successor: edge.Successor})
	}
	for _, edge := range projection.BoundaryEdges {
		page.Boundary.Edges = append(page.Boundary.Edges, TaskGraphBoundaryEdge{Predecessor: edge.Predecessor, Successor: edge.Successor, Reason: edge.Reason})
	}
	page.Boundary.Reasons = projection.Reasons
	page.Boundary.OmittedNodeCount = projection.OmittedNodeCount
	page.Boundary.OmittedBoundaryEdgeCount = projection.OmittedBoundaryEdgeCount
	page.Boundary.Truncated = projection.Truncated
	return page, nil
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
