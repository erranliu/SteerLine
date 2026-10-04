// Package taskview builds task graph read models from one caller-provided
// snapshot. It owns the graph query protocol, not storage or task mutations.
package taskview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

const (
	Protocol         = "native-task-graph/v1"
	MaxNodes         = 500
	MaxDepth         = 10
	MaxBoundaryEdges = 500
)

type Node struct {
	TaskID          string                           `json:"task_id"`
	Title           string                           `json:"title"`
	Status          string                           `json:"status"`
	Priority        int                              `json:"priority"`
	DependencyReady bool                             `json:"dependency_ready"`
	Blockers        []taskmodel.TaskDependencyStatus `json:"blockers"`
	StartupBlockers []string                         `json:"startup_blockers"`
	Delivery        *task.TaskDeliveryRecord         `json:"delivery,omitempty"`
}

type Edge struct {
	Predecessor string `json:"predecessor"`
	Successor   string `json:"successor"`
}

type Filter struct {
	ProjectID    string `json:"project_id"`
	Status       string `json:"status,omitempty"`
	CenterTaskID string `json:"center_task_id,omitempty"`
	Direction    string `json:"direction,omitempty"`
	Depth        int    `json:"depth"`
	Limit        int    `json:"limit"`
}

func (f Filter) Filtered() bool { return f.Status != "" || f.CenterTaskID != "" }

type BoundaryEdge struct {
	Predecessor string `json:"predecessor"`
	Successor   string `json:"successor"`
	Reason      string `json:"reason"`
}

type Boundary struct {
	Truncated                bool           `json:"truncated"`
	MaxDepth                 int            `json:"max_depth"`
	MaxNodes                 int            `json:"max_nodes"`
	OmittedNodeCount         int            `json:"omitted_node_count"`
	OmittedBoundaryEdgeCount int            `json:"omitted_boundary_edge_count"`
	Reasons                  []string       `json:"reasons"`
	Edges                    []BoundaryEdge `json:"edges"`
}

type Page struct {
	Protocol         string    `json:"protocol"`
	ProjectID        string    `json:"project_id"`
	AuthorityVersion uint64    `json:"authority_version"`
	GraphRevision    uint64    `json:"graph_revision"`
	DatabaseRevision uint64    `json:"database_revision"`
	DatabaseID       string    `json:"database_id"`
	SampledAt        time.Time `json:"sampled_at"`
	Nodes            []Node    `json:"nodes"`
	Edges            []Edge    `json:"edges"`
	NextCursor       string    `json:"next_cursor,omitempty"`
	HasMore          bool      `json:"has_more,omitempty"`
	Filters          Filter    `json:"filters"`
	Boundary         Boundary  `json:"boundary"`
}

// MarshalJSON preserves source text and escapes bidi controls in output.
func (p Page) MarshalJSON() ([]byte, error) {
	type plain Page
	body, err := json.Marshal(plain(p))
	if err != nil {
		return nil, err
	}
	return EscapeBidiJSON(body), nil
}

// EscapeBidiJSON is also used by the task detail page in the center package.
func EscapeBidiJSON(body []byte) []byte {
	for _, control := range []rune{'\u061c', '\u200e', '\u200f', '\u202a', '\u202b', '\u202c', '\u202d', '\u202e', '\u2066', '\u2067', '\u2068', '\u2069'} {
		body = bytes.ReplaceAll(body, []byte(string(control)), []byte(fmt.Sprintf("\\u%04x", control)))
	}
	return body
}
