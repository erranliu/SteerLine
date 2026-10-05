package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

const (
	TaskGraphProtocol         = "native-task-graph/v1"
	MaxTaskGraphNodes         = 500
	MaxTaskGraphDepth         = 10
	MaxTaskGraphBoundaryEdges = 500
)

type GraphNode struct {
	TaskID          string              `json:"task_id"`
	Title           string              `json:"title"`
	Status          string              `json:"status"`
	Priority        int                 `json:"priority"`
	DependencyReady bool                `json:"dependency_ready"`
	Blockers        []DependencySummary `json:"blockers"`
	StartupBlockers []string            `json:"startup_blockers"`
	Delivery        *DeliverySummary    `json:"delivery,omitempty"`
}

type GraphEdge struct {
	Predecessor string `json:"predecessor"`
	Successor   string `json:"successor"`
}

type GraphFilter struct {
	ProjectID    string `json:"project_id"`
	Status       string `json:"status,omitempty"`
	CenterTaskID string `json:"center_task_id,omitempty"`
	Direction    string `json:"direction,omitempty"`
	Depth        int    `json:"depth"`
	Limit        int    `json:"limit"`
}

func (f GraphFilter) Filtered() bool { return f.Status != "" || f.CenterTaskID != "" }

type GraphBoundaryEdge struct {
	Predecessor string `json:"predecessor"`
	Successor   string `json:"successor"`
	Reason      string `json:"reason"`
}

type GraphBoundary struct {
	Truncated                bool                `json:"truncated"`
	MaxDepth                 int                 `json:"max_depth"`
	MaxNodes                 int                 `json:"max_nodes"`
	OmittedNodeCount         int                 `json:"omitted_node_count"`
	OmittedBoundaryEdgeCount int                 `json:"omitted_boundary_edge_count"`
	Reasons                  []string            `json:"reasons"`
	Edges                    []GraphBoundaryEdge `json:"edges"`
}

type GraphPage struct {
	Protocol         string        `json:"protocol"`
	ProjectID        string        `json:"project_id"`
	AuthorityVersion uint64        `json:"authority_version"`
	GraphRevision    uint64        `json:"graph_revision"`
	DatabaseRevision uint64        `json:"database_revision"`
	DatabaseID       string        `json:"database_id"`
	SampledAt        time.Time     `json:"sampled_at"`
	Nodes            []GraphNode   `json:"nodes"`
	Edges            []GraphEdge   `json:"edges"`
	NextCursor       string        `json:"next_cursor,omitempty"`
	HasMore          bool          `json:"has_more,omitempty"`
	Filters          GraphFilter   `json:"filters"`
	Boundary         GraphBoundary `json:"boundary"`
}

// MarshalJSON preserves source text and escapes bidi controls in output.
func (p GraphPage) MarshalJSON() ([]byte, error) {
	type plain GraphPage
	body, err := json.Marshal(plain(p))
	if err != nil {
		return nil, err
	}
	return EscapeBidiJSON(body), nil
}

// EscapeBidiJSON preserves source text while preventing terminal-direction
// controls from being interpreted when serialized pages are displayed.
func EscapeBidiJSON(body []byte) []byte {
	for _, control := range []rune{'\u061c', '\u200e', '\u200f', '\u202a', '\u202b', '\u202c', '\u202d', '\u202e', '\u2066', '\u2067', '\u2068', '\u2069'} {
		body = bytes.ReplaceAll(body, []byte(string(control)), []byte(fmt.Sprintf("\\u%04x", control)))
	}
	return body
}
