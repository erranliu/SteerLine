package steerline

import (
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func nativeDeliveryInconsistent(record taskmodel.TaskRecord, graph storageports.TaskGraphSnapshot) bool {
	_, delivered := graph.Deliveries[record.TaskID]
	return taskmodel.DeliveryInconsistent(record.Status, delivered)
}

func taskDependencyState(record taskmodel.TaskRecord, graph storageports.TaskGraphSnapshot) (bool, []taskmodel.TaskDependencyStatus) {
	return taskmodel.DependencyStateForTask(record, graph.TaskGraph())
}
