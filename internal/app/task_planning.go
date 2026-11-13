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
)

const taskPlanningGenerate = "roadmap_task_generate"

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
	if task.Kind != roadmap.SingleTaskPlanning || task.Revision != in.ExpectedVersion || task.Operation != nil || task.Conflict != nil || task.Status != "ready" && task.Status != "closed" {
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
	source := roadmap.Source{ID: task.ID, PlanningID: task.ID, Path: task.Title, Body: body, Digest: roadmap.Digest([]byte(body)), Head: commit}
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
