package app

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	roadmapservice "github.com/erranliu/SteerLine/internal/roadmap"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type roadmapTaskAdmission interface {
	submit(ctx context.Context, sourceID, requestID string, batch taskmodel.TaskBatchInput) (taskmodel.TaskBatchReceipt, error)
	receipt(ctx context.Context, requestID string) (taskmodel.TaskBatchReceipt, error)
}

type roadmapMergeHistory interface {
	refreshBaseline(context.Context) (string, error)
	verifyAncestor(ctx context.Context, ancestor, descendant string) error
}

func (p *roadmapPort) admissionBatch(ctx context.Context, r roadmap.Record) (taskmodel.TaskBatchInput, error) {
	batch := taskmodel.TaskBatchInput{SchemaVersion: taskmodel.BatchSchemaVersion, ProjectID: p.config.Identity(), Tasks: []taskmodel.TaskBatchTask{}, Dependencies: []taskmodel.TaskBatchEdge{}}
	if e := roadmap.ValidateDocuments(r); e != nil {
		return batch, e
	}
	if r.Publication == nil || r.Publication.State != "merged" || !taskmodel.ValidCommit(r.Publication.MergedCommit) {
		return batch, errors.New("文档合并尚未核实")
	}
	if e := p.verifyMerged(ctx, &r); e != nil {
		return batch, e
	}
	target, e := p.config.TaskRepositoryTarget(ctx)
	if e != nil {
		return batch, e
	}
	documents := map[string]roadmap.Documents{}
	for _, d := range r.Documents {
		documents[d.Key] = d
	}
	analysis, e := roadmap.ValidateAnalysis(*r.Analysis, r.Source, r.Existing)
	if e != nil {
		return batch, e
	}
	ordered, e := roadmap.Order(analysis.Tasks)
	if e != nil {
		return batch, e
	}
	known := map[string]string{}
	for _, n := range ordered {
		if n.ExistingID != "" {
			known[n.Key] = n.ExistingID
		}
	}
	reference := func(d roadmap.Document) taskmodel.FileReference {
		return taskmodel.FileReference{Path: d.Path, Commit: r.Publication.MergedCommit, SHA256: roadmap.Digest([]byte(d.Body))}
	}
	for _, n := range ordered {
		if n.ExistingID != "" {
			continue
		}
		d := documents[n.Key]
		contract := taskmodel.TaskContract{ContractID: roadmap.ValueDigest([]string{r.Source.ID, n.Key, r.Publication.MergedCommit})[:32], Repository: target, Remote: p.config.Remote, Branch: p.config.Branch, SpecRef: reference(d.Spec), PlanRef: reference(d.Plan), CommonRefs: []taskmodel.FileReference{}, AcceptanceRefs: []taskmodel.FileReference{}}
		for _, a := range d.Acceptance {
			contract.AcceptanceRefs = append(contract.AcceptanceRefs, reference(a))
		}
		contract.Digest, e = taskmodel.ContractDigest(contract)
		if e != nil {
			return batch, e
		}
		batch.Tasks = append(batch.Tasks, taskmodel.TaskBatchTask{LocalKey: n.Key, Title: n.Title, Type: "task", Contract: contract})
		formal := append([]string(nil), n.Proposed.Formal...)
		for _, key := range n.Proposed.Local {
			if id := known[key]; id != "" {
				formal = append(formal, id)
			} else {
				batch.Dependencies = append(batch.Dependencies, taskmodel.TaskBatchEdge{Predecessor: taskmodel.TaskBatchReference{LocalKey: key}, Successor: taskmodel.TaskBatchReference{LocalKey: n.Key}})
			}
		}
		for _, id := range formal {
			number, e := strconv.ParseInt(id, 10, 64)
			if e != nil {
				return batch, e
			}
			batch.Dependencies = append(batch.Dependencies, taskmodel.TaskBatchEdge{Predecessor: taskmodel.TaskBatchReference{TaskID: &number}, Successor: taskmodel.TaskBatchReference{LocalKey: n.Key}})
		}
	}
	if len(batch.Tasks) == 0 {
		return batch, errors.New("没有待入图的新任务")
	}
	return batch, nil
}
func (p *roadmapPort) admit(ctx context.Context, r roadmap.Record, request string) (roadmap.Record, error) {
	if r.State != "awaiting-admission" && r.State != "admission-unknown" {
		return r, errors.New("请先发布并核实文档合并")
	}
	if r.AdmissionRequest != "" && r.AdmissionRequest != request {
		return r, errors.New("已有原入图请求，请先查询原结果")
	}
	if r.AdmissionRequest == "" {
		batch, e := p.admissionBatch(ctx, r)
		if e != nil {
			return r, e
		}
		if e = p.verifyAdmissionGraph(ctx, r); e != nil {
			return r, e
		}
		r.AdmissionBatch, e = json.Marshal(batch)
		if e != nil {
			return r, e
		}
		r.AdmissionRequest = request
		r.State = "admission-unknown"
		r, e = p.serviceSave(ctx, r)
		if e != nil {
			return r, e
		}
	}
	var batch taskmodel.TaskBatchInput
	if e := json.Unmarshal(r.AdmissionBatch, &batch); e != nil {
		return r, e
	}
	receipt, e := p.taskAdmission.submit(ctx, r.Source.ID, r.AdmissionRequest, batch)
	if e != nil {
		return r, e
	}
	r.Mapping = receipt.TaskIDs
	r.State = "admitted"
	r.Diagnostic = ""
	return p.serviceSave(context.WithoutCancel(ctx), r)
}
func (p *roadmapPort) serviceSave(ctx context.Context, r roadmap.Record) (roadmap.Record, error) {
	expected := r.Revision
	r.Revision++
	return r, p.store.SaveRoadmap(ctx, r, expected)
}
func (p *roadmapPort) verifyMerged(ctx context.Context, r *roadmap.Record) error {
	if r.Publication == nil || r.Publication.URL == "" {
		return errors.New("没有绑定的文档 PR")
	}
	fact, e := p.prFact(ctx, r.Publication.URL)
	if e != nil {
		return e
	}
	if !fact.Merged || fact.Base.Ref != p.config.Branch || fact.Head.Ref != r.Publication.Branch || fact.Head.SHA != r.Publication.Commit || fact.MergeCommit == "" {
		return errors.New("文档 PR 尚未合并或绑定身份不符")
	}
	baseline, e := p.mergeHistory.refreshBaseline(ctx)
	if e != nil {
		return e
	}
	if e = p.mergeHistory.verifyAncestor(ctx, fact.MergeCommit, baseline); e != nil {
		return errors.New("文档合并未进入项目目标主分支")
	}
	files, e := p.publicationFiles(ctx, *r)
	if e != nil {
		return e
	}
	for name, body := range files {
		got, e := p.materials.readCommittedMaterial(ctx, fact.MergeCommit, name)
		if body == "" && strings.HasSuffix(name, ".manual-checks.json") && isMissingRoadmapFile(e) {
			continue
		}
		if e != nil {
			return e
		}
		if string(got) != body {
			return errors.New("合并后的文档内容与审阅结果不同：" + name)
		}
	}
	catalogue, _, e := p.testCatalogueAt(ctx, fact.MergeCommit)
	if e != nil {
		return e
	}
	if e = roadmapservice.ValidateTestIDs(*r, catalogue); e != nil {
		return e
	}
	r.Publication.MergedCommit = fact.MergeCommit
	r.Publication.State = "merged"
	r.State = "awaiting-admission"
	return nil
}
func (p *roadmapPort) reconcileAdmission(ctx context.Context, r roadmap.Record) (roadmap.Record, error) {
	if r.AdmissionRequest == "" {
		return r, errors.New("没有原入图请求")
	}
	receipt, e := p.taskAdmission.receipt(ctx, r.AdmissionRequest)
	if e != nil {
		return r, e
	}
	r.Mapping = receipt.TaskIDs
	r.State = "admitted"
	r.Diagnostic = strings.TrimSpace(r.Diagnostic)
	return p.serviceSave(ctx, r)
}

func (p *roadmapPort) resumeAdmission(ctx context.Context, r roadmap.Record) (roadmap.Record, error) {
	observed, err := p.reconcileAdmission(ctx, r)
	if err == nil {
		return observed, nil
	}
	var commandErr *taskmodel.Error
	if !errors.As(err, &commandErr) || commandErr.Code != "command_not_found" {
		return r, err
	}
	// The original Task request does not exist. Recheck its fixed prerequisites
	// before explicitly resuming the same idempotent transaction.
	if err = p.verifyAdmissionGraph(ctx, r); err != nil {
		return r, err
	}
	if err = p.verifyMerged(ctx, &r); err != nil {
		return r, err
	}
	r.State = "admission-unknown"
	return p.admit(ctx, r, r.AdmissionRequest)
}

func (p *roadmapPort) verifyAdmissionGraph(ctx context.Context, r roadmap.Record) error {
	if r.Source.PlanningID != "" {
		// A single task has only explicit edges from existing tasks. Unrelated
		// additions between publication and admission do not change those edges.
		graph, err := p.store.TaskGraphSnapshot(ctx, p.config.Identity())
		if err != nil {
			return err
		}
		tasks := make([]roadmap.Existing, 0, len(graph.Tasks))
		for _, task := range graph.Tasks {
			tasks = append(tasks, roadmap.Existing{ID: task.TaskID, Status: task.Status})
		}
		_, err = roadmap.ValidateAnalysis(*r.Analysis, r.Source, tasks)
		return err
	}
	graph, err := p.service.Graph(ctx)
	if err != nil {
		return err
	}
	if roadmap.GraphDigest(graph) != r.GraphDigest {
		return errors.New("已有 DAG 或任务材料已变化，请重新核对规划")
	}
	return nil
}
