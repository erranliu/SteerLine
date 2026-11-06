package taskrunner

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
)

func conflictInput(c runnerbridge.Candidate) (json.RawMessage, error) {
	r := c.Projection
	if c.Participation != "active" || c.StopRequested || r == nil || c.RunID == "" || r.Status != "pr_ready" || !r.Controls.CanMessage || r.PR == nil || r.PR.URL == "" || r.PR.Merged || r.PR.State == "closed" || r.PR.State == "closed_unmerged" || r.PR.State == "merged" {
		return nil, failure("candidate_inactive", "原候选当前不能接收冲突解决指令")
	}
	text, err := prompts.PRConflictResolution(r.PR.URL, r.PR.BaseBranch)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Revision uint64 `json:"expected_revision"`
		Text     string `json:"text"`
	}{r.Revision, text})
}
