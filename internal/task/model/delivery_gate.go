package model

// DependencyClaimError maps an unmet predecessor to its stable command error.
func DependencyClaimError(blockers []TaskDependencyStatus) error {
	for _, blocker := range blockers {
		switch blocker.Blocker {
		case "discarded_predecessor":
			return newError("discarded_predecessor", "a discarded predecessor blocks this task")
		case "inconsistent_delivery":
			return newError("inconsistent_delivery", "a predecessor's state conflicts with its delivery record")
		}
	}
	return newError("task_not_ready", "one or more predecessors lack verified delivery")
}
