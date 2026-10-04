package task

import (
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func StartupBlockers(status string, delivered, paused bool, mode string, slotBusy bool) []string {
	blockers := []string{}
	if taskmodel.DeliveryInconsistent(status, delivered) {
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
