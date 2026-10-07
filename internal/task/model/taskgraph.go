// Package taskgraph contains deterministic in-memory rules for task DAGs.
// Callers provide a verified snapshot and keep storage, readiness qualification,
// and side effects in their own authority boundary.
package model

import (
	"sort"
	"time"
)

// Node contains only the values used by ordering rules.
type Node struct {
	ID        string
	Priority  int
	CreatedAt time.Time
}

// Edge points from a predecessor to its successor.
type Edge struct {
	Predecessor string
	Successor   string
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
