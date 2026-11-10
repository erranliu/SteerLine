package query

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func projectRoadmapActivity(a api.AsyncActivity, f ports.AsyncActivityFact, instance string, settled, current bool) (api.AsyncActivity, error) {
	var r struct {
		Project, ID, Purpose, State, Diagnostic, Submission string
		Revision                                            uint64
		CreatedAt                                           time.Time `json:"created_at"`
		RequestID                                           string    `json:"request_id"`
		RunID                                               string    `json:"run_id"`
		Cancelled                                           bool
		Source                                              struct{ Path string }
		Projection                                          *struct {
			Status         string
			UpdatedAt      time.Time `json:"updated_at"`
			ResumeRequired bool      `json:"resume_required"`
			Execution      struct {
				AttemptID string `json:"attempt_id"`
			}
		}
		Operation   *struct{ Status, Instance, ID, Action string }
		Publication *struct{ State, Diagnostic string }
	}
	if err := json.Unmarshal(f.Payload, &r); err != nil {
		return a, err
	}
	if r.Project != a.ProjectID || r.ID != f.ID {
		return a, errors.New("roadmap activity identity differs")
	}
	a.Kind, a.Title = "roadmap_parse", "解析 Roadmap 草稿 · "+r.Source.Path
	if r.Purpose == "documents" {
		a.Kind, a.Title = "roadmap_documents", "生成 Spec / Plan / 验收文档 · "+r.Source.Path
	}
	a.Revision, a.CreatedAt = r.Revision, activityTime(r.CreatedAt)
	a.RequestIDs = []string{r.RequestID}
	a.Details = api.AsyncActivityDetails{Kind: "roadmap", Target: r.ID}
	a.Execution = api.ExecutionReference{RequestID: r.RequestID, RunID: r.RunID}
	if r.Projection != nil {
		a.Execution.AttemptID = r.Projection.Execution.AttemptID
	}
	if r.Operation != nil && r.Operation.ID != "" {
		a.Execution.RequestID, a.Execution.Action = r.Operation.ID, r.Operation.Action
		// Publication/admission are local operations, not a new Runner execution.
		a.Execution.RunID, a.Execution.AttemptID = "", ""
	}
	a.Diagnostic = r.Diagnostic
	switch r.State {
	case "queued":
		a.State, a.Phase = "queued", "等待内容生成"
	case "accepted":
		a.State, a.Phase = "waiting", "内容生成执行状态待核实"
		if r.Projection != nil {
			switch r.Projection.Status {
			case "running":
				a.State, a.Phase = "running", "解析任务与依赖"
				if r.Purpose == "documents" {
					a.Phase = "生成 Spec / Plan / 验收文档"
				}
			case "queued":
				a.State, a.Phase = "queued", "内容生成排队中"
			case "blocked", "waiting_input":
				a.State, a.Phase, a.NeedsAttention = "waiting", "内容生成受阻，需人工处理", current
			case "waiting_external":
				a.State, a.Phase = "waiting", "等待内容生成外部结果"
			}
			if r.Projection.ResumeRequired {
				a.State, a.Phase, a.NeedsAttention = "waiting", "内容生成需要显式恢复", current
			}
		}
	case "review":
		a.State, a.Phase, a.NeedsAttention = "succeeded", "解析完成，待确认任务与依赖", current
	case "confirmed":
		a.State, a.Phase = "succeeded", "任务与依赖已确认"
	case "documents-review":
		a.State, a.Phase, a.NeedsAttention = "succeeded", "文档生成完成，待审阅发布", current
	case "blocked", "documents-blocked":
		a.Phase, a.NeedsAttention = "生成结果存在待澄清问题", current
		if !current && settled {
			a.State, a.Phase = "failed", "原生成已结束，结果含待澄清问题"
		}
	case "failed", "invalid", "rejected":
		a.State, a.Phase, a.NeedsAttention = "failed", "内容生成失败", current
	case "cancelled":
		a.State, a.Phase = "cancelled", "内容生成已取消"
	case "publishing":
		a.Phase, a.NeedsAttention = "文档发布待核实或恢复", true
		if r.Operation != nil && r.Operation.Status == "pending" && r.Operation.Instance == instance {
			a.State, a.Phase, a.NeedsAttention = "running", "发布 Roadmap 文档", false
		}
	case "awaiting-merge":
		a.Phase = "等待文档 PR 合并"
	case "awaiting-admission":
		a.Phase, a.NeedsAttention = "文档已合并，等待入图", true
	case "admission-unknown":
		a.Phase, a.NeedsAttention = "原入图结果待核实", true
		if r.Operation != nil && r.Operation.Status == "pending" && r.Operation.Instance == instance {
			a.State, a.Phase, a.NeedsAttention = "running", "Roadmap 正在入图", false
		}
	case "admitted":
		a.State, a.Phase = "succeeded", "Roadmap 已入图"
	default:
		a.Phase, a.NeedsAttention = "内容生成提交结果待核实", current
	}
	if r.Publication != nil && r.Publication.Diagnostic != "" {
		a.Diagnostic = strings.TrimSpace(a.Diagnostic + " · " + r.Publication.Diagnostic)
	}
	if r.Cancelled {
		a.State, a.Phase, a.NeedsAttention = "cancelled", "内容生成已取消", false
	}
	if activityTerminal(a.State) && !settled {
		a.Summary, a.State, a.Phase = a.Phase, "waiting", "原内容生成仍在运行或收尾待核实"
	}
	// A Runner timestamp describes content generation only. Later publication
	// and admission have no authoritative completion timestamp in this record.
	if activityTerminal(a.State) && r.Publication == nil && r.Projection != nil {
		a.FinishedAt = activityTime(r.Projection.UpdatedAt)
	}
	return a, nil
}
