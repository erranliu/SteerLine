package steerline

import (
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/task"
)

func nativeStartupBlockers(record TaskRecord, graph taskGraphSnapshot) []string {
	_, delivered := graph.Deliveries[record.TaskID]
	return task.StartupBlockers(record.Status, delivered, graph.Authority.DispatchPaused, graph.Authority.DispatchMode, graph.ExecutionSlotBusy)
}

func nativeDeliveryInconsistent(record TaskRecord, graph taskGraphSnapshot) bool {
	_, delivered := graph.Deliveries[record.TaskID]
	return task.DeliveryInconsistent(record.Status, delivered)
}

func taskDependencyState(record TaskRecord, graph taskGraphSnapshot) (bool, []TaskDependencyStatus) {
	return task.DependencyStateForTask(record, graph)
}

// ReadyNativeTaskSnapshot returns the center's dispatch-order candidates and
// the database version sampled with them. The result is never a claim permit.
type NativeReadyTaskSnapshot = contracts.NativeReadyTaskSnapshot
