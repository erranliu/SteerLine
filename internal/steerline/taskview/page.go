package taskview

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"

	"github.com/erranliu/SteerLine/internal/steerline/task"
)

type cursor struct {
	Version          int    `json:"v"`
	ProjectID        string `json:"p"`
	AuthorityVersion uint64 `json:"a"`
	GraphRevision    uint64 `json:"g"`
	DatabaseRevision uint64 `json:"d"`
	DatabaseID       string `json:"i"`
	Offset           int    `json:"o"`
}

// PageRequest is validated before the center reads a graph snapshot.
type PageRequest struct {
	project  string
	limit    int
	cursor   string
	expected cursor
}

func NormalizePage(project, encodedCursor string, limit int) (PageRequest, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > MaxNodes {
		limit = MaxNodes
	}
	request := PageRequest{project: project, limit: limit, cursor: encodedCursor}
	if encodedCursor == "" {
		return request, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(encodedCursor)
	if err != nil {
		return PageRequest{}, viewError("cursor_invalidated", "cursor encoding is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request.expected) != nil || decoder.Decode(new(any)) != io.EOF || request.expected.Version != 1 || request.expected.ProjectID != project || request.expected.Offset < 0 || request.expected.DatabaseRevision == 0 || len(request.expected.DatabaseID) != 32 {
		return PageRequest{}, viewError("cursor_invalidated", "cursor identity or sampling revision is invalid")
	}
	return request, nil
}

// Paginate preserves the version-bound cursor and boundary semantics.
func Paginate(snapshot task.Snapshot, request PageRequest) (Page, error) {
	expected := request.expected
	if request.cursor != "" && (expected.AuthorityVersion != snapshot.Authority.AuthorityVersion || expected.GraphRevision != snapshot.Authority.GraphRevision || expected.DatabaseRevision != snapshot.DatabaseRevision || expected.DatabaseID != snapshot.DatabaseID) {
		return Page{}, viewError("cursor_invalidated", "database or project graph changed between pages")
	}
	page := Page{
		Protocol: Protocol, ProjectID: request.project,
		AuthorityVersion: snapshot.Authority.AuthorityVersion, GraphRevision: snapshot.Authority.GraphRevision,
		DatabaseRevision: snapshot.DatabaseRevision, DatabaseID: snapshot.DatabaseID, SampledAt: snapshot.SampledAt,
		Nodes: []Node{}, Edges: []Edge{},
		Filters:  Filter{ProjectID: request.project, Depth: 1, Limit: request.limit},
		Boundary: Boundary{MaxDepth: 1, MaxNodes: request.limit, Reasons: []string{}, Edges: []BoundaryEdge{}},
	}
	offset := expected.Offset
	if offset > len(snapshot.Tasks) || offset == len(snapshot.Tasks) && offset > 0 {
		return page, viewError("cursor_invalidated", "cursor offset is outside the current snapshot")
	}
	end := offset + request.limit
	if end > len(snapshot.Tasks) {
		end = len(snapshot.Tasks)
	}
	page.HasMore = end < len(snapshot.Tasks)
	selected := map[string]bool{}
	taskGraph := snapshot.TaskGraph()
	for _, record := range snapshot.Tasks[offset:end] {
		selected[record.TaskID] = true
		page.Nodes = append(page.Nodes, graphNode(record, snapshot, taskGraph))
	}
	for _, edge := range snapshot.Edges {
		pred, succ := selected[edge.PredecessorID], selected[edge.SuccessorID]
		if pred && succ {
			page.Edges = append(page.Edges, Edge{Predecessor: edge.PredecessorID, Successor: edge.SuccessorID})
		} else if pred || succ {
			if len(page.Boundary.Edges) < MaxBoundaryEdges {
				page.Boundary.Edges = append(page.Boundary.Edges, BoundaryEdge{Predecessor: edge.PredecessorID, Successor: edge.SuccessorID, Reason: "page_limit"})
			} else {
				page.Boundary.OmittedBoundaryEdgeCount++
			}
		}
	}
	page.Boundary.OmittedNodeCount = len(snapshot.Tasks) - len(page.Nodes)
	page.Boundary.Truncated = page.Boundary.OmittedNodeCount > 0
	if page.Boundary.Truncated {
		page.Boundary.Reasons = append(page.Boundary.Reasons, "page_limit")
	}
	if page.Boundary.OmittedBoundaryEdgeCount > 0 {
		page.Boundary.Reasons = append(page.Boundary.Reasons, "boundary_edge_limit")
	}
	if page.HasMore {
		c := cursor{Version: 1, ProjectID: request.project, AuthorityVersion: page.AuthorityVersion, GraphRevision: page.GraphRevision, DatabaseRevision: page.DatabaseRevision, DatabaseID: page.DatabaseID, Offset: end}
		raw, _ := json.Marshal(c)
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return page, nil
}
