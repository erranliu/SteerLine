package steerline

// nativeDependencyState is shared by the DAG and task-detail read models. It
// keeps dependency readiness separate from dispatch conditions such as pause.
func nativeDependencyState(status string, delivered bool, predecessors []TaskDependencyStatus) (bool, []TaskDependencyStatus, []TaskDependencyStatus) {
	ready := status == "open" && !nativeDeliveryInconsistentStatus(status, delivered)
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

func nativeDeliveryInconsistentStatus(status string, delivered bool) bool {
	return (status == "closed") != delivered
}

func nativeStartupBlockersFor(status string, delivered, paused bool, mode string, slotBusy bool) []string {
	blockers := []string{}
	if nativeDeliveryInconsistentStatus(status, delivered) {
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
