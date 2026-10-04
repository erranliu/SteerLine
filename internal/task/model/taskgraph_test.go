package model

import (
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestCycleIsDeterministicAndIncludesRepeatedEndpoint(t *testing.T) {
	cases := []struct {
		name  string
		edges []Edge
		want  []string
	}{
		{
			name: "smallest numeric cycle first",
			edges: []Edge{
				{Predecessor: "10", Successor: "11"},
				{Predecessor: "4", Successor: "10"},
				{Predecessor: "3", Successor: "2"},
				{Predecessor: "2", Successor: "3"},
			},
			want: []string{"2", "3", "2"},
		},
		{
			name:  "self cycle",
			edges: []Edge{{Predecessor: "7", Successor: "7"}},
			want:  []string{"7", "7"},
		},
		{name: "acyclic disconnected graph", edges: []Edge{{Predecessor: "10", Successor: "11"}, {Predecessor: "2", Successor: "3"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Cycle(tc.edges)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Cycle(%v) = %v, want %v", tc.edges, got, tc.want)
			}
			for left, right := 0, len(tc.edges)-1; left < right; left, right = left+1, right-1 {
				tc.edges[left], tc.edges[right] = tc.edges[right], tc.edges[left]
			}
			if reversed := Cycle(tc.edges); !reflect.DeepEqual(reversed, tc.want) {
				t.Fatalf("Cycle(reversed edges) = %v, want %v", reversed, tc.want)
			}
		})
	}
}

func TestStableTaskOrderUsesPriorityTimeAndNumericID(t *testing.T) {
	base := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	nodes := []Node{
		{ID: "9", Priority: 2, CreatedAt: base.Add(time.Second)},
		{ID: "10", Priority: 1, CreatedAt: base},
		{ID: "11", Priority: 1, CreatedAt: base},
		{ID: "12", Priority: 0, CreatedAt: base},
		{ID: "2", Priority: 1, CreatedAt: base},
	}
	edges := []Edge{{Predecessor: "9", Successor: "12"}}
	want := []string{"2", "10", "11", "9", "12"}
	if got := StableTaskOrder(nodes, edges); !reflect.DeepEqual(got, want) {
		t.Fatalf("StableTaskOrder() = %v, want %v", got, want)
	}
}

func TestProjectNeighborhoodKeepsBranchesMergesAndDisconnects(t *testing.T) {
	nodes := []Node{{ID: "6"}, {ID: "5"}, {ID: "4"}, {ID: "3"}, {ID: "2"}, {ID: "1"}}
	edges := []Edge{
		{Predecessor: "3", Successor: "5"},
		{Predecessor: "2", Successor: "3"},
		{Predecessor: "3", Successor: "4"},
		{Predecessor: "1", Successor: "3"},
	}
	filter := Filter{CenterID: "3", Direction: "both", Depth: 1, Limit: 500, MaxBoundaryEdges: 500}
	want := Projection{
		NodeIDs: []string{"3", "1", "2", "4", "5"},
		Edges: []Edge{
			{Predecessor: "1", Successor: "3"},
			{Predecessor: "2", Successor: "3"},
			{Predecessor: "3", Successor: "4"},
			{Predecessor: "3", Successor: "5"},
		},
		BoundaryEdges: []BoundaryEdge{},
		Reasons:       []string{},
	}
	got := Project(nodes, edges, filter)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Project() = %+v, want %+v", got, want)
	}
	for _, tc := range []struct {
		direction string
		nodeIDs   []string
		edges     []Edge
	}{
		{
			direction: "upstream",
			nodeIDs:   []string{"3", "1", "2"},
			edges:     []Edge{{Predecessor: "1", Successor: "3"}, {Predecessor: "2", Successor: "3"}},
		},
		{
			direction: "downstream",
			nodeIDs:   []string{"3", "4", "5"},
			edges:     []Edge{{Predecessor: "3", Successor: "4"}, {Predecessor: "3", Successor: "5"}},
		},
	} {
		filter.Direction = tc.direction
		got := Project(nodes, edges, filter)
		if !reflect.DeepEqual(got.NodeIDs, tc.nodeIDs) || !reflect.DeepEqual(got.Edges, tc.edges) {
			t.Fatalf("Project(direction=%q) = nodes:%v edges:%v, want nodes:%v edges:%v", tc.direction, got.NodeIDs, got.Edges, tc.nodeIDs, tc.edges)
		}
	}

	reversedNodes := append([]Node(nil), nodes...)
	reversedEdges := append([]Edge(nil), edges...)
	for left, right := 0, len(reversedNodes)-1; left < right; left, right = left+1, right-1 {
		reversedNodes[left], reversedNodes[right] = reversedNodes[right], reversedNodes[left]
	}
	for left, right := 0, len(reversedEdges)-1; left < right; left, right = left+1, right-1 {
		reversedEdges[left], reversedEdges[right] = reversedEdges[right], reversedEdges[left]
	}
	filter.Direction = "both"
	if reversed := Project(reversedNodes, reversedEdges, filter); !reflect.DeepEqual(reversed, want) {
		t.Fatalf("Project() changed with input order: %+v, want %+v", reversed, want)
	}
}

func TestProjectSortsNumericIDsAndReportsNodeLimit(t *testing.T) {
	nodes := []Node{{ID: "10"}, {ID: "2"}, {ID: "1"}}
	got := Project(nodes, nil, Filter{Limit: 2, MaxBoundaryEdges: 500})
	want := Projection{
		NodeIDs:                  []string{"1", "2"},
		Edges:                    []Edge{},
		BoundaryEdges:            []BoundaryEdge{},
		OmittedNodeCount:         1,
		Reasons:                  []string{"node_limit"},
		Truncated:                true,
		OmittedBoundaryEdgeCount: 0,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Project() = %+v, want %+v", got, want)
	}
}

func TestProjectStatusFilterRetainsAnchorAndMarksOmittedBoundary(t *testing.T) {
	nodes := []Node{
		{ID: "1", Status: "discarded"},
		{ID: "2", Status: "open"},
		{ID: "3", Status: "closed"},
	}
	edges := []Edge{{Predecessor: "1", Successor: "2"}, {Predecessor: "2", Successor: "3"}}
	filter := Filter{Status: "open", CenterID: "1", Direction: "downstream", Depth: 1, Limit: 1, MaxBoundaryEdges: 500}
	got := Project(nodes, edges, filter)
	want := Projection{
		NodeIDs:          []string{"1"},
		Edges:            []Edge{},
		BoundaryEdges:    []BoundaryEdge{{Predecessor: "1", Successor: "2", Reason: "node_limit"}},
		OmittedNodeCount: 2,
		Reasons:          []string{"depth_limit", "node_limit"},
		Truncated:        true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Project() = %+v, want %+v", got, want)
	}

	filter.Limit = 2
	got = Project(nodes, edges, filter)
	if !reflect.DeepEqual(got.NodeIDs, []string{"1", "2"}) || len(got.BoundaryEdges) != 1 || got.BoundaryEdges[0].Reason != "depth_limit" {
		t.Fatalf("anchor or depth boundary changed: %+v", got)
	}
}

func TestProjectDepthLimitAndBoundaryEdgeCap(t *testing.T) {
	t.Run("depth", func(t *testing.T) {
		nodes := make([]Node, 12)
		edges := make([]Edge, 11)
		for index := range nodes {
			id := strconv.Itoa(index + 1)
			nodes[index] = Node{ID: id}
			if index > 0 {
				edges[index-1] = Edge{Predecessor: strconv.Itoa(index), Successor: id}
			}
		}
		got := Project(nodes, edges, Filter{CenterID: "1", Direction: "downstream", Depth: 10, Limit: 500, MaxBoundaryEdges: 500})
		if len(got.NodeIDs) != 11 || got.NodeIDs[10] != "11" || got.OmittedNodeCount != 1 || !reflect.DeepEqual(got.Reasons, []string{"depth_limit"}) || !reflect.DeepEqual(got.BoundaryEdges, []BoundaryEdge{{Predecessor: "11", Successor: "12", Reason: "depth_limit"}}) {
			t.Fatalf("depth projection = %+v", got)
		}
	})
	t.Run("500 edge cap", func(t *testing.T) {
		const nodeCount = 502
		nodes := make([]Node, nodeCount)
		edges := make([]Edge, nodeCount-1)
		for index := range nodes {
			id := strconv.Itoa(index + 1)
			nodes[index] = Node{ID: id}
			if index > 0 {
				edges[index-1] = Edge{Predecessor: "1", Successor: id}
			}
		}
		got := Project(nodes, edges, Filter{Limit: 1, MaxBoundaryEdges: 500})
		if len(got.NodeIDs) != 1 || got.NodeIDs[0] != "1" || got.OmittedNodeCount != 501 || len(got.BoundaryEdges) != 500 || got.OmittedBoundaryEdgeCount != 1 || !reflect.DeepEqual(got.Reasons, []string{"boundary_edge_limit", "node_limit"}) || !got.Truncated {
			t.Fatalf("capped boundary = nodes:%d omitted:%d edges:%d omitted_edges:%d reasons:%v", len(got.NodeIDs), got.OmittedNodeCount, len(got.BoundaryEdges), got.OmittedBoundaryEdgeCount, got.Reasons)
		}
	})
}
