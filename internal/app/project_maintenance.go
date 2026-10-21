package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pm "github.com/erranliu/SteerLine/internal/projectmaintenance"
	pmm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

type maintenanceStore interface {
	Generation(context.Context, string, string) (testmanagement.Generation, error)
	Generations(context.Context, string) ([]testmanagement.Generation, error)
	MaintenanceReceipt(context.Context, string, string) (json.RawMessage, error)
	TaskGraphSnapshot(context.Context, string) (storageports.TaskGraphSnapshot, error)
	TaskContract(context.Context, string, taskmodel.TaskRecord) (taskmodel.TaskContract, error)
	TaskExecution(context.Context, string, string) (taskmodel.ExecutionSnapshot, error)
	MaterialApproval(context.Context, string, string) (pmm.Approval, error)
	SaveMaterialApproval(context.Context, pmm.Approval, uint64) (pmm.Approval, error)
	ConfirmTaskMaterials(context.Context, string, string, uint64, taskmodel.TaskContract, string) (taskmodel.TaskRecord, error)
}
type projectMaintenance struct {
	config      Config
	store       maintenanceStore
	mu          sync.Mutex
	observation pmm.Observation
	checking    chan struct{}
}

func newProjectMaintenance(c Config, s maintenanceStore) *projectMaintenance {
	return &projectMaintenance{config: c, store: s}
}
func (m *projectMaintenance) git(ctx context.Context, args ...string) ([]byte, error) {
	return (commandRunner{Dir: m.config.Repository, Timeout: positiveDuration(m.config.CommandTimeout, 30*time.Second), ProcessHost: m.config.ProcessHost}).Output(ctx, m.config.Git, args...)
}

type materialReadKey struct{}

// Each assessment uses one request-scoped cache of immutable Git blobs. It is
// never shared between requests or used for mutable workspace files.
func materialReads(ctx context.Context) context.Context {
	return context.WithValue(ctx, materialReadKey{}, map[string][]byte{})
}
func (m *projectMaintenance) read(ctx context.Context, commit, path string) ([]byte, error) {
	cache, _ := ctx.Value(materialReadKey{}).(map[string][]byte)
	key := commit + ":" + path
	if b, ok := cache[key]; ok {
		return b, nil
	}
	if e := taskmodel.ValidateFileReference(taskmodel.FileReference{Path: path, Commit: commit, SHA256: strings.Repeat("0", 64)}); e != nil {
		return nil, e
	}
	b, e := (taskDocuments{repository: m.config.Repository, git: m.config.Git, timeout: positiveDuration(m.config.CommandTimeout, 30*time.Second), processHost: m.config.ProcessHost}).readCommittedFile(ctx, commit, path)
	if e == nil && cache != nil {
		cache[key] = b
	}
	return b, e
}
func (m *projectMaintenance) current() pmm.Observation {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.observation
}
func (m *projectMaintenance) check(ctx context.Context) (pmm.Observation, error) {
	m.mu.Lock()
	if done := m.checking; done != nil {
		m.mu.Unlock()
		select {
		case <-done:
			o := m.current()
			if o.Diagnostic != "" {
				return o, errors.New(o.Diagnostic)
			}
			return o, nil
		case <-ctx.Done():
			return pmm.Observation{}, ctx.Err()
		}
	}

	before := m.observation
	m.checking = make(chan struct{})
	done := m.checking
	m.mu.Unlock()
	o := before
	o.CheckedAt = time.Now().UTC()
	o.Diagnostic = ""
	ref := "refs/remotes/" + m.config.Remote + "/" + m.config.Branch
	_, e := m.git(ctx, "fetch", "--no-tags", "--no-write-fetch-head", m.config.Remote, "+refs/heads/"+m.config.Branch+":"+ref)
	if e == nil {
		var b []byte
		b, e = m.git(ctx, "rev-parse", "--verify", ref+"^{commit}")
		if e == nil {
			o.Commit = strings.TrimSpace(string(b))
			o.Previous = before.Commit
			o.LastSuccess = o.CheckedAt
			o.Rewritten = false
			if before.Commit != "" && before.Commit != o.Commit {
				_, a := m.git(ctx, "merge-base", "--is-ancestor", before.Commit, o.Commit)
				o.Rewritten = a != nil
			}
		}
	}
	if e != nil {
		o.Diagnostic = e.Error()
	}
	m.mu.Lock()
	m.observation = o
	m.checking = nil
	close(done)
	m.mu.Unlock()
	return o, e
}
func (m *projectMaintenance) observe(ctx context.Context) {
	// A bounded background reader never holds the project actor or a SQL transaction.
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	for {
		c, cancel := context.WithTimeout(ctx, 35*time.Second)
		_, _ = m.check(c)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (m *projectMaintenance) localWorkspace(ctx context.Context) (string, error) {
	head, e := m.git(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	if e != nil {
		return "", e
	}
	branch, e := m.git(ctx, "symbolic-ref", "--quiet", "HEAD")
	if e != nil {
		return "", errors.New("checked-out branch required for file application")
	}
	return strings.TrimSpace(string(head)) + "\n" + strings.TrimSpace(string(branch)), nil
}
func (m *projectMaintenance) workspace(ctx context.Context) (string, error) {
	snapshot, e := m.localWorkspace(ctx)
	if e != nil {
		return "", e
	}
	// No writes during an active merge or rebase. Git reports state paths, not guesses.
	for _, state := range []string{"MERGE_HEAD", "REBASE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD"} {
		if _, e := m.git(ctx, "rev-parse", "--verify", state); e == nil {
			return "", errors.New("Git operation in progress")
		}
	}
	for _, state := range []string{"rebase-merge", "rebase-apply"} {
		path, err := m.git(ctx, "rev-parse", "--git-path", state)
		if err != nil {
			return "", err
		}
		name := strings.TrimSpace(string(path))
		if !filepath.IsAbs(name) {
			name = filepath.Join(m.config.Repository, name)
		}
		if _, err = os.Stat(name); err == nil {
			return "", errors.New("Git operation in progress")
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	remote, err := m.git(ctx, "remote", "get-url", m.config.Remote)
	if err != nil {
		return "", err
	}
	target := pmm.Hash([]string{m.config.Identity(), m.config.Remote, m.config.Branch, strings.TrimSpace(string(remote))})
	return snapshot + "\n" + target, nil
}
func (m *projectMaintenance) assessment(ctx context.Context, mode, plan, commit string) (pm.Assessment, error) {
	plans := []string{}
	if plan != "" {
		plans = append(plans, plan)
	} else {
		g, e := m.store.TaskGraphSnapshot(ctx, m.config.Identity())
		if e != nil {
			return pm.Assessment{}, e
		}
		for _, r := range g.Tasks {
			if r.Status != "open" {
				continue
			}
			c, e := m.store.TaskContract(ctx, m.config.Identity(), r)
			if e != nil {
				return pm.Assessment{}, e
			}
			plans = append(plans, c.PlanRef.Path)
		}
	}
	approvals := map[string]pmm.Approval{}
	for _, p := range plans {
		a, e := m.store.MaterialApproval(ctx, m.config.Identity(), p)
		if e != nil {
			return pm.Assessment{}, e
		}
		approvals[p] = a
	}
	reader := func(p string) ([]byte, error) { return m.read(ctx, commit, p) }
	if mode == "local" {
		w, e := m.localWorkspace(ctx)
		if e != nil {
			return pm.Assessment{}, e
		}
		commit = "draft:" + w
		files := map[string]string{}
		reader = func(p string) ([]byte, error) {
			b, e := testmanagement.ReadProjectFile(m.config.Repository, p)
			if e == nil {
				files[p] = pmm.Digest(b)
			}
			return b, e
		}
		a, err := pm.Evaluate(mode, commit, plans, reader, approvals)
		if err != nil {
			return a, err
		}
		for path, digest := range files {
			b, e := testmanagement.ReadProjectFile(m.config.Repository, path)
			if e != nil || pmm.Digest(b) != digest {
				return pm.Assessment{}, errors.New("workspace materials changed during review")
			}
		}
		end, err := m.localWorkspace(ctx)
		if err != nil {
			return a, err
		}
		if end != w {
			return pm.Assessment{}, errors.New("Git workspace changed during review")
		}
		return a, nil
	}
	return pm.Evaluate(mode, commit, plans, reader, approvals)
}
func (m *projectMaintenance) resolved(ctx context.Context, c taskmodel.TaskContract, commit string) (taskmodel.TaskContract, error) {
	resolve := func(ref taskmodel.FileReference) (taskmodel.FileReference, error) {
		b, e := m.read(ctx, commit, ref.Path)
		if e != nil {
			return ref, e
		}
		ref.Commit = commit
		ref.SHA256 = taskPayloadDigest(b)
		if e = taskmodel.ValidateDocumentContent(ref, b, taskmodel.IsAcceptanceReference(c.AcceptanceRefs, ref)); e != nil {
			return ref, e
		}
		return ref, nil
	}
	var e error
	if c.SpecRef, e = resolve(c.SpecRef); e != nil {
		return c, e
	}
	if c.PlanRef, e = resolve(c.PlanRef); e != nil {
		return c, e
	}
	c.CommonRefs = append([]taskmodel.FileReference(nil), c.CommonRefs...)
	c.AcceptanceRefs = append([]taskmodel.FileReference(nil), c.AcceptanceRefs...)
	for i := range c.CommonRefs {
		if c.CommonRefs[i], e = resolve(c.CommonRefs[i]); e != nil {
			return c, e
		}
	}
	for i := range c.AcceptanceRefs {
		if c.AcceptanceRefs[i], e = resolve(c.AcceptanceRefs[i]); e != nil {
			return c, e
		}
	}
	c.Digest, e = taskmodel.ContractDigest(c)
	return c, e
}
func contentContract(c taskmodel.TaskContract) string {
	c.SpecRef.Commit = ""
	c.PlanRef.Commit = ""
	c.CommonRefs = append([]taskmodel.FileReference(nil), c.CommonRefs...)
	c.AcceptanceRefs = append([]taskmodel.FileReference(nil), c.AcceptanceRefs...)
	for i := range c.CommonRefs {
		c.CommonRefs[i].Commit = ""
	}
	for i := range c.AcceptanceRefs {
		c.AcceptanceRefs[i].Commit = ""
	}
	c.Digest = ""
	return pmm.Hash(c)
}
func (m *projectMaintenance) prepare(ctx context.Context, w taskmodel.FixedWork, candidate bool, base func(context.Context, taskmodel.FixedWork, bool) (string, error)) (taskmodel.FixedWork, string, error) {
	ctx = materialReads(ctx)
	if candidate {
		x, e := m.store.TaskExecution(ctx, m.config.Identity(), w.Record.TaskID)
		if e != nil {
			return w, "", e
		}
		w.Contract = x.Contract
		w.FrozenTestPlan = x.TestPlan
		b, e := base(ctx, w, true)
		return w, b, e
	}
	o, e := m.check(ctx)
	if e != nil {
		return w, "", e
	}
	updated, e := m.resolved(ctx, w.Contract, o.Commit)
	if e != nil {
		return w, "", e
	}
	if contentContract(updated) != contentContract(w.Contract) {
		return w, "", &taskmodel.Error{Code: "materials_changed", Message: "任务材料已变化，请在项目维护中审阅并确认"}
	}
	a, e := m.assessment(ctx, "baseline", updated.PlanRef.Path, o.Commit)
	if e != nil {
		return w, "", e
	}
	item := a.Items[0]
	if item.Status != "ready" {
		return w, "", &taskmodel.Error{Code: "materials_not_ready", Message: fmt.Sprintf("%s：%s，请到项目维护核对", item.Plan, item.Status+" "+item.Diagnostic)}
	}
	w.ConfirmedDigest = w.Contract.Digest
	w.Contract = updated
	w.MaterialPlan = item.Plan
	w.MaterialInputs = item.Inputs
	w.MaterialOutput = item.Output
	w.MaterialRevision = item.ApprovalRevision
	// Freeze the definitions already validated by the common rule pipeline.
	// A second Git read must not turn a ready assessment into an invalid snapshot.
	w.FrozenTestPlan, e = json.Marshal(testmanagement.Snapshot{Status: "ready", PlanPath: item.Plan, Commit: o.Commit, Definitions: item.Definitions, Digest: testmanagement.Digest(item.Definitions)})
	if e != nil {
		return w, "", e
	}
	// The same fresh fetched target must contain all predecessor deliveries.
	b, e := base(ctx, w, false)
	if e != nil {
		return w, "", e
	}
	if b != o.Commit {
		return w, "", errors.New("baseline changed during material preparation; check again")
	}
	return w, b, nil
}

// payloads are fixed before writes; the project actor serializes mutations.
type materialReview struct {
	RequestID    string `json:"request_id"`
	Plan         string `json:"plan"`
	Mode         string `json:"mode"`
	ReviewDigest string `json:"review_digest"`
}
type taskReview struct {
	RequestID    string                 `json:"request_id"`
	TaskID       string                 `json:"task_id"`
	Version      uint64                 `json:"version"`
	Contract     taskmodel.TaskContract `json:"contract"`
	ReviewDigest string                 `json:"review_digest"`
}

func (m *projectMaintenance) approve(ctx context.Context, in materialReview, expected uint64) (pmm.Approval, error) {
	if raw, e := m.store.MaintenanceReceipt(ctx, m.config.Identity(), in.RequestID); e == nil {
		var a pmm.Approval
		if json.Unmarshal(raw, &a) != nil || a.Plan != in.Plan || a.Project != m.config.Identity() || a.Revision == 0 || pmm.Hash([]string{a.Commit, a.Plan, a.Inputs, a.Output, fmt.Sprint(a.Revision - 1)}) != in.ReviewDigest {
			return a, errors.New("request_conflict: material review differs")
		}
		return a, nil
	} else if !errors.Is(e, storageports.ErrNotFound) {
		return pmm.Approval{}, e
	}
	o := m.current()
	if in.Mode != "local" && o.Commit == "" {
		return pmm.Approval{}, errors.New("check remote first")
	}
	if in.Mode != "local" {
		in.Mode = "baseline"
	}
	a, e := m.assessment(ctx, in.Mode, in.Plan, o.Commit)
	if e != nil {
		return pmm.Approval{}, e
	}
	if len(a.Items) != 1 {
		return pmm.Approval{}, errors.New("select one Plan")
	}
	i := a.Items[0]
	if i.Status == "invalid" || i.ReviewDigest != in.ReviewDigest || expected != max(uint64(1), i.ApprovalRevision) {
		return pmm.Approval{}, errors.New("stale_review: Plan inputs or arrangement changed")
	}
	return m.store.SaveMaterialApproval(ctx, pmm.Approval{Project: m.config.Identity(), Plan: i.Plan, Inputs: i.Inputs, Output: i.Output, Commit: a.Commit, By: "local-user", RequestID: in.RequestID}, i.ApprovalRevision)
}
func (m *projectMaintenance) taskPreview(ctx context.Context, id string) (any, error) {
	ctx = materialReads(ctx)
	o, e := m.check(ctx)
	if e != nil {
		return nil, e
	}
	g, e := m.store.TaskGraphSnapshot(ctx, m.config.Identity())
	if e != nil {
		return nil, e
	}
	r, ok := g.ByID[id]
	if !ok {
		return nil, errors.New("task not found")
	}
	c, e := m.store.TaskContract(ctx, m.config.Identity(), r)
	if e != nil {
		return nil, e
	}
	next, e := m.resolved(ctx, c, o.Commit)
	if e != nil {
		return nil, e
	}
	v := taskReview{TaskID: id, Version: r.Version, Contract: next}
	v.ReviewDigest = pmm.Hash(v)
	type diff struct {
		Path      string `json:"path"`
		Before    string `json:"before"`
		After     string `json:"after"`
		Truncated bool   `json:"truncated"`
	}
	diffs := []diff{}
	refs := append([]taskmodel.FileReference{c.SpecRef, c.PlanRef}, c.CommonRefs...)
	refs = append(refs, c.AcceptanceRefs...)
	for _, ref := range refs {
		before, _ := m.read(ctx, ref.Commit, ref.Path)
		after, e := m.read(ctx, o.Commit, ref.Path)
		if e != nil {
			return nil, e
		}
		if taskPayloadDigest(after) == ref.SHA256 {
			continue
		}
		limit := 8192
		d := diff{Path: ref.Path, Before: string(before), After: string(after)}
		if len(d.Before) > limit {
			d.Before = d.Before[:limit]
			d.Truncated = true
		}
		if len(d.After) > limit {
			d.After = d.After[:limit]
			d.Truncated = true
		}
		diffs = append(diffs, d)
	}
	return struct {
		taskReview
		Original taskmodel.TaskContract `json:"original"`
		Diffs    []diff                 `json:"diffs"`
	}{v, c, diffs}, nil
}
func (m *projectMaintenance) confirm(ctx context.Context, in taskReview) (taskmodel.TaskRecord, error) {
	wanted := in.ReviewDigest
	check := in
	check.RequestID = ""
	check.ReviewDigest = ""
	if wanted != pmm.Hash(check) {
		return taskmodel.TaskRecord{}, errors.New("task review differs")
	}
	if _, e := m.store.MaintenanceReceipt(ctx, m.config.Identity(), in.RequestID); e == nil {
		return m.store.ConfirmTaskMaterials(ctx, m.config.Identity(), in.TaskID, in.Version, in.Contract, in.RequestID)
	} else if !errors.Is(e, storageports.ErrNotFound) {
		return taskmodel.TaskRecord{}, e
	}
	if e := NewTaskService(nil, m.config).VerifyContractFiles(ctx, in.Contract); e != nil {
		return taskmodel.TaskRecord{}, e
	}
	return m.store.ConfirmTaskMaterials(ctx, m.config.Identity(), in.TaskID, in.Version, in.Contract, in.RequestID)
}
func (m *projectMaintenance) view(ctx context.Context, mode, plan string) (any, error) {
	ctx = materialReads(ctx)
	o := m.current()
	if mode != "local" && o.Commit == "" {
		return struct {
			Observation pmm.Observation `json:"observation"`
			Assessment  pm.Assessment   `json:"assessment"`
		}{o, pm.Assessment{Items: []pm.Item{}}}, nil
	}
	if mode == "" {
		mode = "baseline"
	}
	a, e := m.assessment(ctx, mode, plan, o.Commit)
	local, _ := m.git(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	type taskItem struct {
		ID         string `json:"task_id"`
		Plan       string `json:"plan"`
		Diagnostic string `json:"diagnostic"`
	}
	taskItems := []taskItem{}
	graph, err := m.store.TaskGraphSnapshot(ctx, m.config.Identity())
	if err != nil {
		return nil, err
	}
	for _, record := range graph.Tasks {
		if record.Status != "open" || mode == "local" {
			continue
		}
		contract, err := m.store.TaskContract(ctx, m.config.Identity(), record)
		if err != nil {
			return nil, err
		}
		if plan != "" && contract.PlanRef.Path != plan {
			continue
		}
		next, err := m.resolved(ctx, contract, o.Commit)
		diagnostic := ""
		if err != nil {
			diagnostic = err.Error()
		} else if contentContract(next) != contentContract(contract) {
			diagnostic = "任务材料已变化，首次启动前需要审阅确认"
		}
		if diagnostic != "" {
			taskItems = append(taskItems, taskItem{record.TaskID, contract.PlanRef.Path, diagnostic})
		}
	}
	type generationItem struct {
		ID    string `json:"id"`
		Plan  string `json:"plan"`
		State string `json:"state"`
	}
	generations := []generationItem{}
	rows, err := m.store.Generations(ctx, m.config.Identity())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		generations = append(generations, generationItem{row.ID, row.Command.Plan, row.State})
	}
	return struct {
		Observation pmm.Observation  `json:"observation"`
		Assessment  pm.Assessment    `json:"assessment"`
		Hint        string           `json:"hint"`
		Tasks       []taskItem       `json:"tasks"`
		Generations []generationItem `json:"generations"`
		LocalCommit string           `json:"local_commit"`
	}{o, a, "请使用正常 Git 流程更新本地仓库；代码更新后建议核对测试适用性", taskItems, generations, strings.TrimSpace(string(local))}, e
}
