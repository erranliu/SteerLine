package app

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

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
	ordered, e := roadmap.Order(r.Analysis.Tasks)
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
		graph, e := p.service.Graph(ctx)
		if e != nil {
			return r, e
		}
		// Only a stable confirmed dependency/material snapshot can reach the transaction.
		if roadmap.GraphDigest(graph) != r.GraphDigest {
			return r, errors.New("已有 DAG 或任务材料已变化，请重新核对规划")
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
	graph, e := p.store.TaskGraphSnapshot(ctx, p.config.Identity())
	if e != nil {
		return r, e
	}
	commands := NewTaskCommands(p.store, p.config)
	taskRequest := roadmap.Digest([]byte("roadmap-admission\n" + request))[:32]
	var receipt taskmodel.TaskBatchReceipt
	mutate := func(ctx context.Context) error {
		var e error
		receipt, e = commands.AdmitRoadmap(ctx, r.Source.ID, batch, taskRequest, graph.Authority.AuthorityVersion, "local-user")
		return e
	}
	if p.mutateTask != nil {
		e = p.mutateTask(ctx, mutate)
	} else {
		e = mutate(ctx)
	}
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
	observation, e := p.config.runtimeMaintenance.check(ctx)
	if e != nil {
		return e
	}
	if _, e = p.config.runtimeMaintenance.git(ctx, "merge-base", "--is-ancestor", fact.MergeCommit, observation.Commit); e != nil {
		return errors.New("文档合并未进入项目目标主分支")
	}
	files, e := p.publicationFiles(ctx, *r)
	if e != nil {
		return e
	}
	for name, body := range files {
		got, e := p.config.runtimeMaintenance.read(ctx, fact.MergeCommit, name)
		if e != nil {
			return e
		}
		if string(got) != body {
			return errors.New("合并后的文档内容与审阅结果不同：" + name)
		}
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
	commands := NewTaskCommands(p.store, p.config)
	id := roadmap.Digest([]byte("roadmap-admission\n" + r.AdmissionRequest))[:32]
	value, e := commands.ReadReceipt(ctx, id, "create_task_batch", p.config.Identity())
	if e != nil {
		return r, e
	}
	raw, _ := json.Marshal(value)
	var receipt taskmodel.TaskBatchReceipt
	if e = json.Unmarshal(raw, &receipt); e != nil {
		return r, e
	}
	if receipt.Status != "accepted" || receipt.RequestID != id || len(receipt.TaskIDs) == 0 {
		return r, errors.New("原入图回执未核实")
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
	graph, err := p.service.Graph(ctx)
	if err != nil {
		return r, err
	}
	if roadmap.GraphDigest(graph) != r.GraphDigest {
		return r, errors.New("原入图前置已变化，不能继续，请重新核对规划")
	}
	if err = p.verifyMerged(ctx, &r); err != nil {
		return r, err
	}
	r.State = "admission-unknown"
	return p.admit(ctx, r, r.AdmissionRequest)
}
