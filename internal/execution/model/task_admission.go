package model

func StartupBlockers(status string, delivered bool, mode string, slotBusy bool) []string {
	blockers := []string{}
	if (status == "closed") != delivered {
		blockers = append(blockers, "inconsistent_delivery")
	}
	if status != "open" {
		blockers = append(blockers, "task_not_open")
	}
	if slotBusy && mode == "auto" {
		blockers = append(blockers, "execution_slot_busy")
	}
	return blockers
}
