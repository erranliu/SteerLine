package steerline

import "github.com/erranliu/SteerLine/internal/steerline/task"

func nativeDependencyState(status string, delivered bool, predecessors []TaskDependencyStatus) (bool, []TaskDependencyStatus, []TaskDependencyStatus) {
	return task.DependencyState(status, delivered, predecessors)
}

func nativeDeliveryInconsistentStatus(status string, delivered bool) bool {
	return task.DeliveryInconsistent(status, delivered)
}

func nativeStartupBlockersFor(status string, delivered, paused bool, mode string, slotBusy bool) []string {
	return task.StartupBlockers(status, delivered, paused, mode, slotBusy)
}
