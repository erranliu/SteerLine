package query

import (
	"encoding/json"
	"math/big"
	"time"

	refactoring "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// Refactoring exposes the action's prompt snapshots without exposing Runner requests.
func Refactoring(r refactoring.Record, now time.Time) map[string]any {
	amount, complete := r.CostTotal()
	ops := make([]map[string]any, 0, len(r.Operations))
	var decisionInput json.RawMessage
	for _, o := range r.Operations {
		workflow := o.Content(r).WorkflowID
		ops = append(ops, map[string]any{"id": o.ID, "purpose": o.Purpose, "round": o.Round, "state": o.State, "feedback": o.Feedback, "result": o.Result, "cost": o.Cost, "created_at": o.CreatedAt, "prompt": o.Prompt, "run_id": o.RunID, "workflow": workflow})
		if workflow == "generate" {
			decisionInput = o.Request.Input
		}
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
	out := RefactoringSummary(r, now)
	out["cost_breakdown"] = breakdown
	out["input"] = r.Input
	out["direction_prompt"] = r.Frozen.Direction.Prompt
	if rules := refactoringDecisionRules(decisionInput); len(rules) > 0 {
		out["decision_rules"] = rules
	}
	out["source_commit"] = r.Frozen.Repository.BaseRevision
	out["source_branch"] = r.Frozen.Repository.BaseBranch
	out["startup_failed"] = r.Phase == "blocked" && r.Frozen.Repository.BaseRevision == ""
	out["question"] = r.Question
	out["operations"] = ops
	out["reports"] = r.Reports
	out["pr"] = r.PR
	out["amount"] = amount
	out["cost_complete"] = complete
	out["untracked"] = []string{"subagents", "tool_internal_models", "unobserved_provider_calls"}
	return out
}

func refactoringDecisionRules(input json.RawMessage) map[string]string {
	var request runnercontract.GenerateRunInput
	if len(input) == 0 || json.Unmarshal(input, &request) != nil {
		return nil
	}
	rules := make(map[string]string, 2)
	for _, material := range request.Generate.Materials {
		if material.Kind != "file" || material.Content == nil {
			continue
		}
		switch material.Path {
		case "maintenance/rules.md":
			rules["rules"] = *material.Content
		case "maintenance/strategy.md":
			rules["strategy"] = *material.Content
		}
	}
	return rules
}

func RefactoringSummary(r refactoring.Record, now time.Time) map[string]any {
	wallEnd := now
	if r.FinishedAt != nil {
		wallEnd = *r.FinishedAt
	}
	phase := r.DisplayPhase()
	wait := ""
	switch r.WaitReason {
	case refactoring.WaitSharedCapacity:
		wait = "等待共享生成容量"
	case refactoring.WaitRunnerCapacity:
		wait = "等待 Runner 容量"
	case refactoring.WaitEnvironment:
		wait = "运行环境资格待核实"
	default:
		if phase == "queued" {
			wait = "等待项目推进名额"
		}
	}
	direction := r.Frozen.Direction.Name
	if direction == "" {
		direction = r.Input.Direction
	}
	return map[string]any{
		"id": r.ID, "revision": r.Revision, "direction_name": direction,
		"environment_name": r.Frozen.EnvironmentName, "profile_name": r.Frozen.ProfileName,
		"phase": phase, "round": r.Round, "slot": r.Slot, "wait_reason": wait,
		"summary": r.Summary, "diagnostic": r.Diagnostic, "enqueued_at": r.EnqueuedAt,
		"elapsed_seconds": r.Elapsed(now).Seconds(), "wall_seconds": wallEnd.Sub(r.CreatedAt).Seconds(),
		"created_at": r.CreatedAt, "updated_at": r.UpdatedAt, "active": r.Active(),
		"can_finish": r.CanFinish(), "can_stop": r.Active() && !r.StopRequested,
		"can_review":   r.CanReview(),
		"can_conflict": r.Phase == "delivered" && r.PR != nil && !r.PR.Merged && r.PR.State == "open",
	}
}
