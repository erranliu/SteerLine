package steerline

import (
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func nativeDeliveryInconsistent(record taskmodel.TaskRecord, graph taskGraphSnapshot) bool {
	_, delivered := graph.Deliveries[record.TaskID]
	return taskmodel.DeliveryInconsistent(record.Status, delivered)
}

func taskDependencyState(record taskmodel.TaskRecord, graph taskGraphSnapshot) (bool, []taskmodel.TaskDependencyStatus) {
	return taskmodel.DependencyStateForTask(record, graph.TaskGraph())
}

// ReadyNativeTaskSnapshot returns the center's dispatch-order candidates and
// the database version sampled with them. The result is never a claim permit.
type NativeReadyTaskSnapshot = contracts.NativeReadyTaskSnapshot
