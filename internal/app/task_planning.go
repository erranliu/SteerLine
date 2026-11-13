package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	roadmaps "github.com/erranliu/SteerLine/internal/roadmap"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

const taskPlanningGenerate = "roadmap_task_generate"

func finishTaskCardPlanning(task *roadmap.PlanningTask, receipt *runnercontract.Command) (bool, error) {
	run := task.Run
	if run.Status == "blocked" && run.ResumeRequired && run.Controls.CanResume {
		task.Operation = nil
		task.Status = run.Status
		return false, fmt.Errorf("原规划会话受阻：%s", run.Diagnostic)
	}
	if receipt.ActionStatus == "failed" || receipt.ReceiptStatus == "rejected" || run.Closed || run.StopStatus == "stopped" {
		task.Operation = nil
		if task.Body == "" && run.Closed {
			task.Status = "closed"
		}
		if receipt.Error != nil {
			return false, receipt.Error
		}
		return false, errors.New("任务卡规划轮次未完成")
	}
	if !run.Controls.CanMessage || run.Execution.TurnID == "" || run.Execution.TurnID == task.Operation.Previous {
		return false, nil
	}
	previousFile := task.Operation.Digest
	task.Operation = nil
	task.Status = "ready"
	if run.Execution.NativeOutcome != "succeeded" {
		return false, errors.New("任务卡规划轮次未成功，原任务卡仍可查看")
	}
	raw, err := roadmaps.ReadRegular(run.Execution.Workspace, task.Path)
	if err != nil || !validTaskCard(string(raw)) || (string(raw) != task.Body && roadmap.Digest(raw) == previousFile) {
		return false, errors.New("本轮未生成完整任务卡，请补充后重新规划")
	}
	if task.Body != string(raw) {
		task.Body = string(raw)
		task.CardVersion++
	}
	task.Diagnostic = ""
	return true, nil
}

func validTaskCard(body string) bool {
	var card roadmap.TaskCard
	return decodeStrictJSON([]byte(body), &card) == nil && strings.TrimSpace(card.Goal) != "" &&
		strings.TrimSpace(card.Scope) != "" && strings.TrimSpace(card.Acceptance) != ""
}

// confirmTaskCard supplies the same confirmed input as Roadmap parsing, from
// the fields and formal predecessors that the user reviewed in the task card.
func (p *roadmapPort) confirmTaskCard(ctx context.Context, in httpcontract.CommandEnvelope) (roadmap.Record, error) {
	var card roadmap.TaskCard
	if err := decodeStrictJSON(in.Payload, &card); err != nil {
		return roadmap.Record{}, err
	}
	task, err := p.store.PlanningTask(ctx, p.config.Identity(), in.TargetID)
	if err != nil {
		return roadmap.Record{}, err
	}
	if task.Kind != roadmap.SingleTaskPlanning || !validTaskCard(task.Body) || task.Revision != in.ExpectedVersion || task.Operation != nil || task.Conflict != nil || task.Status != "ready" && task.Status != "closed" {
		return roadmap.Record{}, errors.New("任务卡已变化或规划操作尚未结算，请刷新后确认")
	}
	baseline, err := p.config.runtimeMaintenance.check(ctx)
	if err != nil {
		return roadmap.Record{}, err
	}
	if baseline.Rewritten {
		return roadmap.Record{}, errors.New("目标分支历史已改写，请先核对项目基线")
	}
	graph, err := p.graph(ctx)
	if err != nil {
		return roadmap.Record{}, err
	}
	rows, err := p.store.Roadmaps(ctx, p.config.Identity())
	if err != nil {
		return roadmap.Record{}, err
	}
	for _, old := range rows {
		if old.Source.PlanningID == task.ID && (!old.ObservationComplete() || old.Publication != nil) {
			return roadmap.Record{}, errors.New("该任务已有未结算的生成或文档发布，请核对原流程")
		}
	}
	latest, err := p.store.PlanningTask(ctx, task.Project, task.ID)
	if err != nil {
		return roadmap.Record{}, err
	}
	if latest.Revision != task.Revision {
		return roadmap.Record{}, errors.New("任务卡已变化，请刷新后确认")
	}
	parent := roadmaps.DocumentConfirmationRequest(in.RequestID)
	r, err := taskCardRecord(task, card, graph, baseline.Commit, parent)
	if err != nil {
		return r, err
	}
	return r, p.store.SaveRoadmap(ctx, r, 0)
}

func taskCardRecord(task roadmap.PlanningTask, card roadmap.TaskCard, graph []roadmap.Existing, commit, id string) (roadmap.Record, error) {
	body := fmt.Sprintf("# %s\n\n## 目标\n%s\n\n## 范围\n%s\n\n## 约束\n%s\n\n## 验收条件\n%s\n", task.Title, card.Goal, card.Scope, card.Constraints, card.Acceptance)
	if len(body) > 256<<10 {
		return roadmap.Record{}, errors.New("任务卡超过大小上限")
	}
	source := roadmap.Source{ID: task.ID, PlanningID: task.ID, CardVersion: task.CardVersion, Path: task.Title, Body: body, Digest: roadmap.Digest([]byte(body)), Head: commit}
	var generated roadmap.TaskCard
	if err := decodeStrictJSON([]byte(task.Body), &generated); err == nil {
		for _, field := range []struct{ name, generated, confirmed string }{
			{"goal", generated.Goal, card.Goal},
			{"scope", generated.Scope, card.Scope},
			{"constraints", generated.Constraints, card.Constraints},
			{"acceptance", generated.Acceptance, card.Acceptance},
		} {
			if field.generated != field.confirmed {
				if source.CardEdits == nil {
					source.CardEdits = make(map[string]string)
				}
				source.CardEdits[field.name] = field.confirmed
			}
		}
	}
	deps := roadmap.Dependencies{Local: []string{}, Formal: append([]string{}, card.Predecessors...)}
	slices.Sort(deps.Formal)
	if len(slices.Compact(slices.Clone(deps.Formal))) != len(deps.Formal) {
		return roadmap.Record{}, errors.New("DAG 前置不能重复")
	}
	analysis, err := roadmap.ValidateAnalysis(roadmap.Analysis{Coordination: "单个任务。前置为用户确认的已有正式任务。", Questions: []string{}, Tasks: []roadmap.Node{{
		Key: "T1", KeyOrigin: "suggested", Title: task.Title, Goal: card.Goal, Scope: card.Scope,
		Constraints: card.Constraints, Acceptance: card.Acceptance, Evidence: card.Goal,
		Original: deps, Proposed: deps, ProvidesTests: append([]string{}, card.ProvidesTests...),
	}}}, source, graph)
	if err != nil {
		return roadmap.Record{}, err
	}
	r := roadmap.Record{ID: id, Project: task.Project, Revision: 1, RequestID: id, Purpose: "task-planning", State: "confirmed", CreatedAt: time.Now().UTC(),
		Source: source, Existing: graph, GraphDigest: roadmap.GraphDigest(graph), Analysis: &analysis, ConfirmationRequest: id, Documents: []roadmap.Documents{}}
	if _, err = roadmap.TestProviders(r); err != nil {
		return roadmap.Record{}, err
	}
	r.ConfirmedDigest = roadmap.ConfirmationDigest(r)
	return r, nil
}

func (p *roadmapPort) verifyCurrentTaskCard(ctx context.Context, r roadmap.Record) error {
	if r.Source.PlanningID == "" {
		return nil
	}
	rows, err := p.store.Roadmaps(ctx, p.config.Identity())
	if err != nil {
		return err
	}
	for _, other := range rows {
		if other.Source.PlanningID == r.Source.PlanningID && other.Purpose == "task-planning" && other.CreatedAt.After(r.CreatedAt) && other.ID != r.ParentID {
			return errors.New("已有较新的确认任务卡，请使用当前规划")
		}
	}
	return nil
}

func (p *roadmapPort) generateTaskDocuments(ctx context.Context, in httpcontract.CommandEnvelope) (roadmap.Record, error) {
	if !roadmap.IDPattern.MatchString(in.TargetID) || !roadmap.IDPattern.MatchString(in.RequestID) {
		return roadmap.Record{}, errors.New("任务规划身份无效")
	}
	if !p.preparation(ctx).documentsReady() {
		return roadmap.Record{}, errors.New("生成规范尚未就绪，请在项目中准备并发布静态材料")
	}
	if p.config.runtimeNative != nil {
		p.config.runtimeNative.RefreshFailures()
	}
	r, err := p.confirmTaskCard(ctx, in)
	if err != nil {
		return r, err
	}
	return p.service.GenerateDocuments(ctx, r.ID, in.RequestID, r.Revision)
}

func taskDocumentTitle(r roadmap.Record) string {
	return "任务文档：" + strings.TrimSpace(r.Source.Path)
}
