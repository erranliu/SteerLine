package query

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

// This read shape deliberately excludes input bodies and execution objects.
func projectAdvisoryActivity(a api.AsyncActivity, f ports.AsyncActivityFact) (api.AsyncActivity, error) {
	var r struct {
		Project, ID, State, Diagnostic string
		Revision                       uint64
		Source                         struct {
			CandidateID string `json:"candidate_id"`
			TaskID      string `json:"task_id"`
		}
		SourceActive bool `json:"source_active"`
		Settled      bool
		CreatedAt    time.Time  `json:"created_at"`
		FinishedAt   *time.Time `json:"finished_at"`
		ObservedAt   time.Time  `json:"observed_at"`
	}
	if err := json.Unmarshal(f.Payload, &r); err != nil {
		return a, err
	}
	if r.Project != a.ProjectID || r.ID != f.ID {
		return a, errors.New("advisory activity identity differs")
	}
	a.Kind = "prompt_suggestion"
	a.Title = "生成后续对话提示词建议"
	a.Revision = r.Revision
	a.CreatedAt = activityTime(r.CreatedAt)
	a.ObservedAt = activityTime(r.ObservedAt)
	a.RequestIDs = []string{r.ID}
	a.Diagnostic = r.Diagnostic
	a.Details = api.AsyncActivityDetails{Kind: "advisory", Target: r.ID, TaskID: r.Source.TaskID, CandidateID: r.Source.CandidateID}
	switch r.State {
	case "queued":
		a.State = "queued"
		a.Phase = "等待准备上下文"
	case "preparing":
		a.State = "running"
		a.Phase = "准备明确上下文"
	case "ready":
		a.State = "succeeded"
		a.Phase = "提示词建议已生成"
		a.NeedsAttention = r.SourceActive
	case "blocked", "failed":
		a.State = "failed"
		a.Phase = "提示词生成受阻"
		a.NeedsAttention = r.SourceActive
	case "cancelled":
		a.State = "cancelled"
		a.Phase = "本次提示词生成已取消"
	default:
		a.State = "waiting"
		a.Phase = "生成状态待核实"
	}
	if !r.Settled {
		a.State = "waiting"
		a.Phase = "等待原生成资源结算"
	} else if activityTerminal(a.State) {
		a.FinishedAt = r.FinishedAt
		a.BusinessFinishedAt = r.FinishedAt
	}
	return a, nil
}
