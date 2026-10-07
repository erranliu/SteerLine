package taskview

import (
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func viewError(code, message string) error { return &taskmodel.Error{Code: code, Message: message} }

// Project uses one verified snapshot for graph shape, delivery and readiness.

func graphNode(record taskmodel.TaskRecord, snapshot storageports.TaskGraphSnapshot, taskGraph taskmodel.Snapshot) Node {
	_, delivered := snapshot.Deliveries[record.TaskID]
	ready, blockers := taskmodel.DependencyStateForTask(record, taskGraph)
	node := Node{
		TaskID: record.TaskID, Title: record.Title, Status: record.Status, Priority: record.Priority,
		DependencyReady: ready, Blockers: blockers,
		StartupBlockers: executionmodel.StartupBlockers(record.Status, delivered, snapshot.Authority.DispatchPaused, snapshot.Authority.DispatchMode, snapshot.ExecutionSlotBusy),
	}
	if delivery, ok := snapshot.Deliveries[record.TaskID]; ok {
		copy := delivery
		node.Delivery = &copy
	}
	return node
}
