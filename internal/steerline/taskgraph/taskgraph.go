// Package taskgraph contains deterministic in-memory rules for task DAGs.
// Callers provide a verified snapshot and keep storage, readiness qualification,
// and side effects in their own authority boundary.
package taskgraph

import (
	"sort"
	"time"
)

// Node contains only the values used by ordering and graph projection rules.
type Node struct {
	ID        string
	Status    string
	Priority  int
	CreatedAt time.Time
}

// Edge points from a predecessor to its successor.
type Edge struct {
	Predecessor string
	Successor   string
}

// Filter contains validated query values. Limit and MaxBoundaryEdges are
// supplied by the caller so this package does not own protocol limits.
type Filter struct {
	Status           string
	CenterID         string
	Direction        string
	Depth            int
	Limit            int
	MaxBoundaryEdges int
}

// BoundaryEdge describes an edge crossing the selected projection boundary.
type BoundaryEdge struct {
	Predecessor string
	Successor   string
	Reason      string
}

// Projection is a deterministic selection over one caller-provided snapshot.
type Projection struct {
	NodeIDs                  []string
	Edges                    []Edge
	BoundaryEdges            []BoundaryEdge
	OmittedNodeCount         int
	OmittedBoundaryEdgeCount int
	Reasons                  []string
	Truncated                bool
}

// Cycle returns the first cycle found by numeric task-ID order, including the
// repeated endpoint. The result does not depend on input edge order.
func Cycle(edges []Edge) []string {
	adjacency := make(map[string][]string)
	for _, edge := range edges {
		adjacency[edge.Predecessor] = append(adjacency[edge.Predecessor], edge.Successor)
		if _, ok := adjacency[edge.Successor]; !ok {
			adjacency[edge.Successor] = nil
		}
	}
	for id := range adjacency {
		sortTaskIDs(adjacency[id])
		adjacency[id] = compactStrings(adjacency[id])
	}

	state := make(map[string]uint8, len(adjacency))
	stack := make([]string, 0, len(adjacency))
	positions := make(map[string]int, len(adjacency))
	var found []string
	var visit func(string) bool
	visit = func(id string) bool {
		state[id] = 1
		positions[id] = len(stack)
		stack = append(stack, id)
		for _, next := range adjacency[id] {
			switch state[next] {
			case 0:
				if visit(next) {
					return true
				}
			case 1:
				found = append(append([]string(nil), stack[positions[next]:]...), next)
				return true
			}
		}
		stack = stack[:len(stack)-1]
		delete(positions, id)
		state[id] = 2
		return false
	}

	nodes := make([]string, 0, len(adjacency))
	for id := range adjacency {
		nodes = append(nodes, id)
	}
	sortTaskIDs(nodes)
	for _, id := range nodes {
		if state[id] == 0 && visit(id) {
			return found
		}
	}
	return nil
}

// StableTaskOrder returns a deterministic topological order. Among currently
// unblocked nodes it orders by priority, creation time, then numeric task ID.
func StableTaskOrder(nodes []Node, edges []Edge) []string {
	byID := make(map[string]Node, len(nodes))
	indegree := make(map[string]int, len(nodes))
	children := make(map[string][]string)
	for _, node := range nodes {
		byID[node.ID] = node
		indegree[node.ID] = 0
	}
	for _, edge := range edges {
		if _, ok := byID[edge.Predecessor]; !ok {
			continue
		}
		if _, ok := byID[edge.Successor]; !ok {
			continue
		}
		indegree[edge.Successor]++
		children[edge.Predecessor] = append(children[edge.Predecessor], edge.Successor)
	}
	less := func(a, b string) bool {
		x, y := byID[a], byID[b]
		if x.Priority != y.Priority {
			return x.Priority < y.Priority
		}
		if !x.CreatedAt.Equal(y.CreatedAt) {
			return x.CreatedAt.Before(y.CreatedAt)
		}
		return taskIDLess(a, b)
	}
	ready := make([]string, 0, len(nodes))
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })
	order := make([]string, 0, len(nodes))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		for _, child := range children[id] {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
				sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })
			}
		}
	}
	return order
}

// Project selects nodes and reports all omitted parts within the supplied
// snapshot. It does not decide whether a node is ready or otherwise qualified.
func Project(nodes []Node, edges []Edge, filter Filter) Projection {
	result := Projection{
		NodeIDs:       []string{},
		Edges:         []Edge{},
		BoundaryEdges: []BoundaryEdge{},
		Reasons:       []string{},
	}
	byID := make(map[string]Node, len(nodes))
	allIDs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
		allIDs = append(allIDs, node.ID)
	}
	allIDs = sortedUniqueTaskIDs(allIDs)
	if filter.CenterID != "" {
		if _, ok := byID[filter.CenterID]; !ok {
			return result
		}
	}

	incoming := make(map[string][]string, len(nodes))
	outgoing := make(map[string][]string, len(nodes))
	sortedEdges := append([]Edge(nil), edges...)
	sort.Slice(sortedEdges, func(i, j int) bool {
		if sortedEdges[i].Predecessor != sortedEdges[j].Predecessor {
			return taskIDLess(sortedEdges[i].Predecessor, sortedEdges[j].Predecessor)
		}
		return taskIDLess(sortedEdges[i].Successor, sortedEdges[j].Successor)
	})
	for _, edge := range sortedEdges {
		incoming[edge.Successor] = append(incoming[edge.Successor], edge.Predecessor)
		outgoing[edge.Predecessor] = append(outgoing[edge.Predecessor], edge.Successor)
	}

	depths := make(map[string]int)
	depthFrontier := make(map[string]bool)
	candidateOrder := make([]string, 0, len(nodes))
	if filter.CenterID == "" {
		candidateOrder = append(candidateOrder, allIDs...)
	} else {
		depths[filter.CenterID] = 0
		reachable := []string{filter.CenterID}
		for head := 0; head < len(reachable); head++ {
			id := reachable[head]
			for _, neighbor := range neighbors(id, filter.Direction, incoming, outgoing) {
				if _, seen := depths[neighbor]; seen {
					continue
				}
				depths[neighbor] = depths[id] + 1
				reachable = append(reachable, neighbor)
			}
		}
		for _, id := range reachable {
			if depths[id] == filter.Depth+1 {
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
	selected := make(map[string]bool, min(filter.Limit, len(candidateOrder)))
	for _, id := range candidateOrder {
		if filter.Status != "" && byID[id].Status != filter.Status && id != filter.CenterID {
			continue
		}
		if len(result.NodeIDs) == filter.Limit {
			continue
		}
		result.NodeIDs = append(result.NodeIDs, id)
		selected[id] = true
	}

	omittedIDs := make(map[string]bool)
	reasons := make(map[string]bool)
	for _, id := range candidateOrder {
		if selected[id] {
			continue
		}
		if filter.Status != "" && byID[id].Status != filter.Status && id != filter.CenterID {
			omittedIDs[id] = true
			reasons["status_filter"] = true
		} else {
			omittedIDs[id] = true
			reasons["node_limit"] = true
		}
	}
	for id, depth := range depths {
		if depth > filter.Depth {
			omittedIDs[id] = true
			reasons["depth_limit"] = true
		}
	}
	for _, edge := range sortedEdges {
		inCandidate := candidate[edge.Predecessor] && candidate[edge.Successor]
		frontierEdge := false
		if filter.CenterID != "" {
			if depthFrontier[edge.Predecessor] && depths[edge.Successor] == filter.Depth {
				frontierEdge = filter.Direction == "upstream" || filter.Direction == "both"
			}
			if depthFrontier[edge.Successor] && depths[edge.Predecessor] == filter.Depth {
				frontierEdge = frontierEdge || filter.Direction == "downstream" || filter.Direction == "both"
			}
		}
		if !inCandidate && !frontierEdge {
			continue
		}
		predSelected, succSelected := selected[edge.Predecessor], selected[edge.Successor]
		if predSelected && succSelected {
			result.Edges = append(result.Edges, edge)
			continue
		}
		if !predSelected && !succSelected {
			continue
		}
		omittedID := edge.Predecessor
		if predSelected {
			omittedID = edge.Successor
		}
		reason := "node_limit"
		if frontierEdge || depthFrontier[omittedID] {
			reason = "depth_limit"
		} else if filter.Status != "" && candidate[omittedID] && byID[omittedID].Status != filter.Status && omittedID != filter.CenterID {
			reason = "status_filter"
		}
		if len(result.BoundaryEdges) < filter.MaxBoundaryEdges {
			result.BoundaryEdges = append(result.BoundaryEdges, BoundaryEdge{Predecessor: edge.Predecessor, Successor: edge.Successor, Reason: reason})
		} else {
			result.OmittedBoundaryEdgeCount++
			reasons["boundary_edge_limit"] = true
		}
		reasons[reason] = true
	}
	for reason := range reasons {
		result.Reasons = append(result.Reasons, reason)
	}
	sort.Strings(result.Reasons)
	result.OmittedNodeCount = len(omittedIDs)
	result.Truncated = len(omittedIDs) > 0 || len(result.BoundaryEdges) > 0
	return result
}

func neighbors(id, direction string, incoming, outgoing map[string][]string) []string {
	var result []string
	switch direction {
	case "upstream":
		result = append(result, incoming[id]...)
	case "downstream":
		result = append(result, outgoing[id]...)
	case "both":
		result = append(result, incoming[id]...)
		result = append(result, outgoing[id]...)
		sortTaskIDs(result)
		result = compactStrings(result)
	}
	return result
}

func sortedUniqueTaskIDs(ids []string) []string {
	sortTaskIDs(ids)
	return compactStrings(ids)
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
