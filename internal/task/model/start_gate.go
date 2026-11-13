package model

// StartDispatchError applies the dispatch policy to both explicit actions and
// automatic first starts.
func StartDispatchError(mode string, explicit, candidate bool) error {
	if candidate && explicit || !candidate && (explicit && mode == "manual" || !explicit && mode == "auto") {
		return nil
	}
	if candidate {
		return newError("wrong_mode", "new candidates require an explicit start")
	}
	if explicit {
		return newError("wrong_mode", "手动启动要求手动派发模式")
	}
	return newError("wrong_mode", "dispatch mode differs from the original start")
}

// CandidateStartError uses caller-owned observations. Only a verified delivery
// transaction can close the task.
func CandidateStartError(candidate bool, active int, merged bool) error {
	if candidate && merged {
		return newError("candidate_pr_merged", "已有候选 PR 合并")
	}
	if active >= 5 {
		return newError("candidate_limit_reached", "当前材料版本已有五个参与候选")
	}
	return nil
}
