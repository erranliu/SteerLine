package app

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	pm "github.com/erranliu/SteerLine/internal/projectmaintenance"
	pmm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

type materialStore interface {
	Material(context.Context, string, string) (pmm.Material, error)
	SaveMaterial(context.Context, string, pmm.Material) error
	MaterialProfile(context.Context, string) string
	SetMaterialProfile(context.Context, string, string) error
	Candidate(context.Context, string, string) (runnerstate.Candidate, error)
}

func (m *projectMaintenance) materialInput(ctx context.Context, task taskmodel.TaskRecord, commit string) (pmm.Material, error) {
	result := pmm.Material{TaskID: task.TaskID, Commit: commit, Status: "invalid"}
	contract, e := m.store.TaskContract(ctx, m.config.Identity(), task)
	if e != nil {
		return result, e
	}
	result.Contract, e = m.resolved(ctx, contract, commit)
	if e != nil {
		return result, e
	}
	values := []string{}
	refs := append([]taskmodel.FileReference{result.Contract.SpecRef, result.Contract.PlanRef}, result.Contract.CommonRefs...)
	refs = append(refs, result.Contract.AcceptanceRefs...)
	for _, r := range refs {
		b, e := m.read(ctx, commit, r.Path)
		if e != nil {
			return result, e
		}
		values = append(values, r.Path, pmm.TextDigest(b))
	}
	sources, e := pm.GenerationSources(contract.PlanRef.Path, func(path string) ([]byte, error) { return m.read(ctx, commit, path) })
	if e != nil {
		return result, e
	}
	for _, source := range sources {
		values = append(values, source.Path, source.TextDigest)
	}
	result.Digest = pmm.Hash(values)
	result.Inputs = testmanagement.GenerationInputs(sources)
	result.Status = "pending"
	return result, nil
}

// refresh runs under the existing project actor. Queries never call it.
func (m *projectMaintenance) refresh(ctx context.Context) error {
	ctx = materialReads(ctx)
	store, ok := m.store.(materialStore)
	if !ok {
		return errors.New("project material store unavailable")
	}
	observed := m.current()
	if observed.Commit == "" || observed.Diagnostic != "" {
		return m.completeGenerationBatch(ctx)
	}
	graph, e := m.store.TaskGraphSnapshot(ctx, m.config.Identity())
	if e != nil {
		return e
	}
	tasks := append([]taskmodel.TaskRecord(nil), graph.Tasks...)
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].TaskID < tasks[j].TaskID })
	for _, task := range tasks {
		if task.Status != "open" && task.Status != "in_progress" {
			continue
		}
		previous, previousErr := store.Material(ctx, m.config.Identity(), task.TaskID)
		material := previous
		var inputErr error
		if previousErr != nil || previous.Commit != observed.Commit {
			material, inputErr = m.materialInput(ctx, task, observed.Commit)
		}
		if previousErr == nil && previous.Commit == observed.Commit && previous.Status == "invalid" && previous.Digest == "" {
			continue
		}
		if inputErr != nil {
			material.Diagnostic = inputErr.Error()
			if e = store.SaveMaterial(ctx, m.config.Identity(), material); e != nil {
				return e
			}
			continue
		}
		rows, e := m.store.Generations(ctx, m.config.Identity())
		if e != nil {
			return e
		}
		var chosen *testmanagement.Generation
		// Prefer a successful version, otherwise keep the newest original attempt.
		for i := range rows {
			g := &rows[i]
			if g.Command.Plan != material.Contract.PlanRef.Path || g.Inputs != material.Inputs {
				continue
			}
			if chosen == nil || g.State == "generated" || chosen.State != "generated" && g.CreatedAt.After(chosen.CreatedAt) {
				chosen = g
			}
		}
		if previous.Inputs == material.Inputs && previous.Status == "pending" && previous.GenerationID != "" {
			if _, e := m.store.Generation(ctx, m.config.Identity(), previous.GenerationID); e != nil {
				chosen = nil
			}
		}
		if chosen != nil {
			if (chosen.State == "failed" || chosen.State == "invalid") && !chosen.Cancelled && chosen.Projection != nil && chosen.Projection.StopStatus != "stopped" && m.tests != nil {
				_, _ = m.tests.service.Stop(ctx, chosen.ID, pmm.Hash([]string{chosen.ID, "settle-failed"})[:32])
			}
			material.GenerationID = chosen.ID
			material.Status = chosen.State
			if chosen.Cancelled {
				material.Status = "cancelled"
			}
			material.Diagnostic = chosen.Diagnostic
			if chosen.State == "generated" {
				definitions, err := testmanagement.Select(chosen.Result, chosen.Catalogue)
				if err == nil && (len(definitions) > 0 || testmanagement.AllowsNoTests(chosen.PlanText)) {
					material.Status = "ready"
				} else {
					material.Status = "invalid"
					material.Diagnostic = "测试安排无效或空安排缺少Plan说明"
				}
			}
		} else if previous.Inputs == material.Inputs && previous.Status == "failed" {
			material.GenerationID = previous.GenerationID
			material.Status = "failed"
			material.Diagnostic = previous.Diagnostic
		} else {
			profile := store.MaterialProfile(ctx, m.config.Identity())
			if profile == "" {
				profile = m.config.RunnerExecution.DefaultProfile
			}
			p, err := m.config.RunnerExecution.Profile(profile, "")
			if err == nil {
				_, err = runnerclient.GenerationExecution(p.Execution)
			}
			if err != nil || m.tests == nil {
				material.Status = "configuration_required"
				material.Diagnostic = "请配置本机Codex测试生成方案"
			} else {
				busy := false
				for _, g := range rows {
					if g.Command.Plan != material.Contract.PlanRef.Path || g.Settled() {
						continue
					}
					busy = true
					if !g.Cancelled {
						_, _ = m.tests.service.Stop(ctx, g.ID, pmm.Hash([]string{g.ID, "superseded"})[:32])
					}
				}
				if busy {
					material.Status = "pending"
					material.Diagnostic = "等待上一版测试生成停止并核实资源"
					if e = store.SaveMaterial(ctx, m.config.Identity(), material); e != nil {
						return e
					}
					continue
				}
				// The deterministic first intent survives restart; explicit retries get a new ID.
				material.GenerationID = pmm.Hash([]string{m.config.Identity(), material.Contract.PlanRef.Path, material.Inputs})[:32]
				if previous.Inputs == material.Inputs && previous.GenerationID != "" {
					material.GenerationID = previous.GenerationID
				}
				if e = store.SaveMaterial(ctx, m.config.Identity(), material); e != nil {
					return e
				}
				generated, generateErr := m.tests.service.Generate(ctx, testmanagement.GenerateCommand{RequestID: material.GenerationID, Plan: material.Contract.PlanRef.Path, Profile: profile, Commit: observed.Commit})
				err = generateErr
				if err != nil {
					material.Status = "failed"
					var failure *runnerclient.Failure
					if errors.As(err, &failure) && failure.Unknown {
						material.Status = "unknown"
					}
					material.Diagnostic = err.Error()
					if saved, readErr := m.store.Generation(ctx, m.config.Identity(), material.GenerationID); readErr == nil {
						material.Status = saved.State
					}
				} else {
					material.Status = generated.State
				}
			}
		}
		if e = store.SaveMaterial(ctx, m.config.Identity(), material); e != nil {
			return e
		}
	}
	if m.tests != nil {
		wanted := map[string]bool{}
		for _, task := range tasks {
			if task.Status != "open" && task.Status != "in_progress" {
				continue
			}
			value, e := store.Material(ctx, m.config.Identity(), task.TaskID)
			if e == nil && value.Inputs != "" {
				wanted[value.Contract.PlanRef.Path+"\n"+value.Inputs] = true
			}
		}
		rows, e := m.store.Generations(ctx, m.config.Identity())
		if e != nil {
			return e
		}
		for _, g := range rows {
			if wanted[g.Command.Plan+"\n"+g.Inputs] || g.Cancelled || g.Settled() {
				continue
			}
			_, _ = m.tests.service.Stop(ctx, g.ID, pmm.Hash([]string{g.ID, "superseded"})[:32])
		}
	}
	return m.syncGenerationBatch(ctx)
}

func (m *projectMaintenance) freshness(ctx context.Context, c runnerstate.Candidate) string {
	o := m.current()
	if o.Commit == "" || o.Diagnostic != "" {
		return "unknown"
	}
	store, ok := m.store.(materialStore)
	if !ok {
		return "unknown"
	}
	latest, e := store.Material(ctx, m.config.Identity(), c.TaskID)
	if e != nil || latest.Commit != o.Commit {
		return "unknown"
	}
	if latest.Digest == "" || c.MaterialDigest != latest.Digest {
		return "outdated"
	}
	return "current"
}

func (m *projectMaintenance) prepareLatest(ctx context.Context, w taskmodel.FixedWork, candidate bool, base func(context.Context, taskmodel.FixedWork, bool) (string, error)) (taskmodel.FixedWork, string, error) {
	if _, e := m.check(ctx); e != nil {
		return w, "", e
	}
	if e := m.refresh(ctx); e != nil {
		return w, "", e
	}
	store, ok := m.store.(materialStore)
	if !ok {
		return w, "", errors.New("project material store unavailable")
	}
	material, e := store.Material(ctx, m.config.Identity(), w.Record.TaskID)
	if e != nil {
		return w, "", e
	}
	if material.Status != "ready" {
		return w, "", &taskmodel.Error{Code: "materials_not_ready", Message: "测试安排尚未就绪：" + material.Status + " " + material.Diagnostic}
	}
	g, e := m.store.Generation(ctx, m.config.Identity(), material.GenerationID)
	if e != nil {
		return w, "", e
	}
	definitions, e := testmanagement.Select(g.Result, g.Catalogue)
	if e != nil {
		return w, "", e
	}
	w.ConfirmedDigest = w.Contract.Digest
	w.Contract = material.Contract
	w.MaterialDigest = material.Digest
	w.FrozenTestPlan, e = json.Marshal(testmanagement.Snapshot{Status: "ready", GenerationID: g.ID, Inputs: material.Inputs, PlanPath: w.Contract.PlanRef.Path, Commit: material.Commit, Definitions: definitions, Digest: testmanagement.Digest(definitions)})
	if e != nil {
		return w, "", e
	}
	b, e := base(ctx, w, candidate)
	if e == nil && b != material.Commit {
		e = errors.New("远端版本已变化，请重新检查后创建候选")
	}
	return w, b, e
}

func (m *projectMaintenance) candidateDocuments(ctx context.Context, id string) (any, error) {
	store, ok := m.store.(materialStore)
	if !ok {
		return nil, errors.New("material store unavailable")
	}
	c, e := store.Candidate(ctx, m.config.Identity(), id)
	if e != nil {
		return nil, e
	}
	latest, e := store.Material(ctx, m.config.Identity(), c.TaskID)
	if e != nil {
		return nil, e
	}
	type file struct {
		Path       string `json:"path"`
		Before     string `json:"before"`
		After      string `json:"after"`
		Diagnostic string `json:"diagnostic,omitempty"`
	}
	files := []file{}
	refs := append([]taskmodel.FileReference{c.Contract.SpecRef, c.Contract.PlanRef}, c.Contract.CommonRefs...)
	refs = append(refs, c.Contract.AcceptanceRefs...)
	refs = append(refs, taskmodel.FileReference{Path: testmanagement.RulesPath, Commit: c.BaseRevision}, taskmodel.FileReference{Path: testmanagement.CataloguePath, Commit: c.BaseRevision})
	seen := map[string]bool{}
	for _, r := range refs {
		if seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		before, err := m.read(ctx, r.Commit, r.Path)
		if err != nil {
			return nil, err
		}
		after, err := m.read(ctx, latest.Commit, r.Path)
		f := file{Path: r.Path, Before: string(before), After: string(after)}
		if err != nil {
			f.Diagnostic = err.Error()
		}
		files = append(files, f)
	}
	after := ""
	if g, err := m.store.Generation(ctx, m.config.Identity(), latest.GenerationID); err == nil {
		defs, err := testmanagement.Select(g.Result, g.Catalogue)
		if err == nil {
			b, _ := json.MarshalIndent(defs, "", "  ")
			after = string(b)
		}
	}
	var snapshot testmanagement.Snapshot
	_ = json.Unmarshal(c.TestPlan, &snapshot)
	before, _ := json.MarshalIndent(snapshot.Definitions, "", "  ")
	files = append(files, file{Path: "系统测试安排", Before: string(before), After: after})
	return struct {
		Status       string `json:"status"`
		Commit       string `json:"commit"`
		LatestCommit string `json:"latest_commit"`
		Files        []file `json:"files"`
	}{m.freshness(ctx, c), c.BaseRevision, latest.Commit, files}, nil
}

func (m *projectMaintenance) taskDocuments(ctx context.Context, id string) (any, error) {
	store, ok := m.store.(materialStore)
	if !ok {
		return nil, errors.New("material store unavailable")
	}
	latest, e := store.Material(ctx, m.config.Identity(), id)
	if e != nil {
		return nil, e
	}
	type document struct {
		Path       string `json:"path"`
		Body       string `json:"body"`
		Diagnostic string `json:"diagnostic,omitempty"`
	}
	files := []document{}
	refs := append([]taskmodel.FileReference{latest.Contract.SpecRef, latest.Contract.PlanRef}, latest.Contract.CommonRefs...)
	refs = append(refs, latest.Contract.AcceptanceRefs...)
	refs = append(refs, taskmodel.FileReference{Path: testmanagement.RulesPath}, taskmodel.FileReference{Path: testmanagement.CataloguePath})
	seen := map[string]bool{}
	for _, ref := range refs {
		if ref.Path == "" || seen[ref.Path] {
			continue
		}
		seen[ref.Path] = true
		body, err := m.read(ctx, latest.Commit, ref.Path)
		d := document{Path: ref.Path, Body: string(body)}
		if err != nil {
			d.Diagnostic = err.Error()
		}
		files = append(files, d)
	}
	if g, e := m.store.Generation(ctx, m.config.Identity(), latest.GenerationID); e == nil && g.State == "generated" {
		defs, e := testmanagement.Select(g.Result, g.Catalogue)
		if e == nil {
			body, _ := json.MarshalIndent(defs, "", "  ")
			files = append(files, document{Path: "系统测试安排", Body: string(body)})
		}
	}
	status := latest.Status
	if o := m.current(); o.Commit != latest.Commit || o.Diagnostic != "" {
		status = "unknown"
	}
	return struct {
		Commit     string     `json:"commit"`
		Status     string     `json:"status"`
		Diagnostic string     `json:"diagnostic,omitempty"`
		Files      []document `json:"files"`
	}{latest.Commit, status, latest.Diagnostic, files}, nil
}
