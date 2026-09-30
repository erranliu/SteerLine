package task

import "github.com/erranliu/SteerLine/internal/steerline/taskgraph"

// DependencyState evaluates task prerequisites from one caller-provided snapshot.
func DependencyState(status string, delivered bool, predecessors []TaskDependencyStatus) (bool, []TaskDependencyStatus, []TaskDependencyStatus) {
	ready := status == "open" && !DeliveryInconsistent(status, delivered)
	all := make([]TaskDependencyStatus, 0, len(predecessors))
	unmet := make([]TaskDependencyStatus, 0)
	for _, predecessor := range predecessors {
		predecessor.Blocker = ""
		switch {
		case predecessor.Status == "":
			predecessor.Blocker = "inconsistent_delivery"
		case predecessor.Status == "discarded":
			predecessor.Blocker = "discarded_predecessor"
		case predecessor.Status == "closed" && !predecessor.Delivered, predecessor.Delivered && predecessor.Status != "closed":
			predecessor.Blocker = "inconsistent_delivery"
		case !predecessor.Delivered:
			predecessor.Blocker = "predecessor_not_delivered"
		}
		all = append(all, predecessor)
		if predecessor.Blocker != "" {
			unmet = append(unmet, predecessor)
			ready = false
		}
	}
	return ready, all, unmet
}

func DeliveryInconsistent(status string, delivered bool) bool {
	return (status == "closed") != delivered
}

func StartupBlockers(status string, delivered, paused bool, mode string, slotBusy bool) []string {
	blockers := []string{}
	if DeliveryInconsistent(status, delivered) {
		blockers = append(blockers, "inconsistent_delivery")
	}
	if status != "open" {
		blockers = append(blockers, "task_not_open")
	}
	if paused {
		blockers = append(blockers, "dispatch_paused")
	}
	if slotBusy && mode == "auto" {
		blockers = append(blockers, "execution_slot_busy")
	}
	return blockers
}

func DependencyStateForTask(record TaskRecord, graph Snapshot) (bool, []TaskDependencyStatus) {
	predecessors := []TaskDependencyStatus{}
	for _, edge := range graph.Edges {
		if edge.SuccessorID != record.TaskID {
			continue
		}
		pred, ok := graph.ByID[edge.PredecessorID]
		if !ok {
			predecessors = append(predecessors, TaskDependencyStatus{TaskID: edge.PredecessorID})
			continue
		}
		_, delivered := graph.Deliveries[edge.PredecessorID]
		predecessors = append(predecessors, TaskDependencyStatus{TaskID: pred.TaskID, Status: pred.Status, Delivered: delivered})
	}
	_, delivered := graph.Deliveries[record.TaskID]
	ready, _, blockers := DependencyState(record.Status, delivered, predecessors)
	return ready, blockers
}

// ReadyTasks returns dependency-ready tasks in stable dispatch order.
func ReadyTasks(graph Snapshot) []TaskRecord {
	ready := []TaskRecord{}
	for _, record := range graph.Tasks {
		if record.Status != "open" {
			continue
		}
		if allowed, _ := DependencyStateForTask(record, graph); allowed {
			ready = append(ready, record)
		}
	}
	nodes := make([]taskgraph.Node, 0, len(ready))
	for _, record := range ready {
		nodes = append(nodes, taskgraph.Node{ID: record.TaskID, Status: record.Status, Priority: record.Priority, CreatedAt: record.CreatedAt})
	}
	edges := make([]taskgraph.Edge, 0, len(graph.Edges))
	for _, edge := range graph.Edges {
		edges = append(edges, taskgraph.Edge{Predecessor: edge.PredecessorID, Successor: edge.SuccessorID})
	}
	orderedIDs := taskgraph.StableTaskOrder(nodes, edges)
	byID := make(map[string]TaskRecord, len(ready))
	for _, record := range ready {
		byID[record.TaskID] = record
	}
	ordered := make([]TaskRecord, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		ordered = append(ordered, byID[id])
	}
	return ordered
}
