package query

import (
	refactoring "github.com/erranliu/SteerLine/internal/refactoring/model"
	"math/big"
	"time"
)

// View exposes the action's direction snapshot and omits internal execution settings.
func Refactoring(r refactoring.Record, now time.Time) map[string]any {
	amount, complete := r.CostTotal()
	ops := make([]map[string]any, 0, len(r.Operations))
	for _, o := range r.Operations {
		ops = append(ops, map[string]any{"id": o.ID, "purpose": o.Purpose, "round": o.Round, "state": o.State, "feedback": o.Feedback, "result": o.Result, "cost": o.Cost, "created_at": o.CreatedAt, "prompt": o.Prompt, "run_id": o.RunID})
	}
	breakdown := map[string]any{}
	for _, closing := range []bool{false, true} {
		sum := new(big.Rat)
		known := false
		full := true
		for _, o := range r.Operations {
			if o.Closing != closing || o.State == "cancelled" || o.State == "rejected" {
				continue
			}
			full = full && o.Cost.Complete
			if o.Cost.Amount != nil {
				n, ok := new(big.Rat).SetString(*o.Cost.Amount)
				if ok {
					sum.Add(sum, n)
					known = true
				}
			}
		}
		var value *string
		if known {
			v := sum.FloatString(18)
			value = &v
		}
		key := "progress"
		if closing {
			key = "closing"
		}
		breakdown[key] = map[string]any{"amount": value, "complete": full}
	}
	wallEnd := now
	if r.FinishedAt != nil {
		wallEnd = *r.FinishedAt
	}
	wallSeconds := wallEnd.Sub(r.CreatedAt).Seconds()
	return map[string]any{"cost_breakdown": breakdown, "wall_seconds": wallSeconds, "id": r.ID, "revision": r.Revision, "input": r.Input, "direction_name": r.Frozen.Direction.Name, "direction_prompt": r.Frozen.Direction.Prompt, "phase": r.Phase, "round": r.Round, "source_commit": r.Frozen.Repository.BaseRevision, "source_branch": r.Frozen.Repository.BaseBranch, "summary": r.Summary, "diagnostic": r.Diagnostic, "question": r.Question, "operations": ops, "reports": r.Reports, "pr": r.PR, "elapsed_seconds": r.Elapsed(now).Seconds(), "amount": amount, "cost_complete": complete, "untracked": []string{"subagents", "tool_internal_models", "unobserved_provider_calls"}, "created_at": r.CreatedAt, "updated_at": r.UpdatedAt, "active": r.Active(), "can_finish": r.Phase == "stopped" || r.Phase == "blocked" || r.Active() && !r.FinishRequested && !r.StopRequested, "can_stop": r.Active() && !r.StopRequested, "can_review": r.CanReview(), "can_conflict": r.Phase == "delivered" && r.PR != nil && !r.PR.Merged && r.PR.State == "open"}
}
