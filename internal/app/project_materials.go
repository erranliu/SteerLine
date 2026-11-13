package app

import (
	"context"
	"encoding/json"
	"errors"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance"
	pmm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"sort"
	"strings"
	"time"
)

type materialStore interface {
	Material(context.Context, string, string) (pmm.Material, error)
	SaveMaterial(context.Context, string, pmm.Material) error
	Candidate(context.Context, string, string) (runnerstate.Candidate, error)
}

func isRetryableMaterialRead(err error) bool {
	var missing *missingCommittedFileError
	var nonregular *nonRegularCommittedFileError
	var invalid *taskmodel.Error
	return !errors.As(err, &missing) && !errors.As(err, &nonregular) && !errors.Is(err, errMismatchedTestWriter) && !errors.As(err, &invalid)
}
func (m *projectMaintenance) materialInput(ctx context.Context, task taskmodel.TaskRecord, commit string) (result pmm.Material, resultErr error) {
	ctx = materialReads(ctx)
	result = pmm.Material{TaskID: task.TaskID, Commit: commit, Status: "invalid"}
	var arrangement, catalogue string
	failure := pmm.Blocker{Category: pmm.TaskMaterial, Code: pmm.TaskMaterialInvalid}
	defer func() {
		if resultErr == nil {
			return
		}
		var missing *missingCommittedFileError
		var nonregular *nonRegularCommittedFileError
		var invalidCatalogue *pm.InvalidCatalogueError
		switch {
		case errors.As(resultErr, &missing):
			failure.Path = missing.path
			switch missing.path {
			case arrangement:
				failure.Category, failure.Code = pmm.TestPlan, pmm.TestPlanMissing
			case catalogue:
				failure.Category, failure.Code = pmm.TestPlan, pmm.TestCatalogueInvalid
			}
		case errors.As(resultErr, &nonregular):
			failure.Path = nonregular.path
			switch nonregular.path {
			case arrangement:
				failure.Category, failure.Code = pmm.TestPlan, pmm.TestPlanInvalid
			case catalogue:
				failure.Category, failure.Code = pmm.TestPlan, pmm.TestCatalogueInvalid
			}
		case errors.As(resultErr, &invalidCatalogue):
			failure = pmm.Blocker{Category: pmm.TestPlan, Code: pmm.TestCatalogueInvalid, Path: invalidCatalogue.Path}
		}
		result.Failure = &failure
	}()
	contract, err := m.store.TaskContract(ctx, m.config.Identity(), task)
	if err != nil {
		return result, err
	}
	result.Contract = contract
	// Reuse the existing request cache and batch reader. Serial per-file Git
	// processes can exhaust the finite assessment budget before parsing begins.
	arrangement, err = testmodel.ArrangementPath(contract.PlanRef.Path)
	if err != nil {
		return result, err
	}
	paths := []string{testmodel.ProjectEntryPath, testmodel.ProtocolPath, testmodel.WriterPath, arrangement}
	refs := append([]taskmodel.FileReference{contract.SpecRef, contract.PlanRef}, contract.CommonRefs...)
	refs = append(refs, contract.AcceptanceRefs...)
	for _, ref := range refs {
		paths = append(paths, ref.Path)
	}
	reader := taskDocuments{repository: m.config.Repository, git: m.config.Git, timeout: positiveDuration(m.config.CommandTimeout, 30*time.Second), processHost: m.config.ProcessHost}
	files, err := reader.readCommittedFiles(ctx, commit, paths)
	if err != nil {
		if isRetryableMaterialRead(err) {
			result.Status = "unknown"
		}
		return result, err
	}
	entry, err := testmodel.ParseProjectEntry(files[testmodel.ProjectEntryPath])
	if err != nil {
		return result, err
	}
	catalogue = entry.TestingCatalogue
	if _, err = reader.readCommittedFiles(ctx, commit, entry.Paths()); err != nil {
		if isRetryableMaterialRead(err) {
			result.Status = "unknown"
		}
		return result, err
	}
	result.Contract, err = m.resolved(ctx, contract, commit)
	if err != nil {
		if isRetryableMaterialRead(err) {
			result.Status = "unknown"
		}
		return result, err
	}
	read := func(path string) ([]byte, error) {
		body, e := m.readCommittedMaterial(ctx, commit, path)
		if e != nil && isRetryableMaterialRead(e) {
			result.Status = "unknown"
		}
		return body, e
	}
	values := []string{}
	refs = append([]taskmodel.FileReference{result.Contract.SpecRef, result.Contract.PlanRef}, result.Contract.CommonRefs...)
	refs = append(refs, result.Contract.AcceptanceRefs...)
	for _, ref := range refs {
		b, err := read(ref.Path)
		if err != nil {
			return result, err
		}
		values = append(values, ref.Path, pmm.TextDigest(b))
	}
	protocol, err := prompts.TestingRules()
	if err != nil {
		return result, err
	}
	if err = verifyTestWriter(read); err != nil {
		return result, err
	}
	if _, err = m.config.projectPolicySlots(ctx, commit); err != nil {
		if isRetryableMaterialRead(err) {
			result.Status = "unknown"
		}
		return result, err
	}
	failure = pmm.Blocker{Category: pmm.TestPlan, Code: pmm.TestPlanInvalid, Path: arrangement}
	plan, basis, err := pm.Arrangement(contract.PlanRef.Path, commit, read, protocol)
	if err != nil {
		var missing *testmodel.MissingTestIDs
		if errors.As(err, &missing) {
			result.MissingTests = append([]string(nil), missing.IDs...)
		}
		return result, err
	}
	if _, err = boundedTestArrangement(plan); err != nil {
		return result, err
	}
	if _, err = testmanagement.InitialProgress(plan); err != nil {
		return result, err
	}
	result.TestPlan, err = json.Marshal(plan)
	if err != nil {
		return result, err
	}
	result.Digest = pmm.Hash(append(values, basis...))
	result.Status = "ready"
	return result, nil
}

// Refresh only committed material. No model, durable preparation or Git write.
func (m *projectMaintenance) refresh(ctx context.Context) error {
	ctx = materialReads(ctx)
	store, ok := m.store.(materialStore)
	if !ok {
		return errors.New("project material store unavailable")
	}
	observed := m.current()
	if observed.Commit == "" || observed.Diagnostic != "" {
		return nil
	}
	graph, err := m.store.TaskGraphSnapshot(ctx, m.config.Identity())
	if err != nil {
		return err
	}
	tasks := append([]taskmodel.TaskRecord(nil), graph.Tasks...)
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].TaskID < tasks[j].TaskID })
	for _, task := range tasks {
		if task.Status != "open" && task.Status != "in_progress" {
			continue
		}
		previous, e := store.Material(ctx, m.config.Identity(), task.TaskID)
		if e != nil && !errors.Is(e, storageports.ErrNotFound) {
			return e
		}
		if e == nil && previous.Commit == observed.Commit && previous.Status != "unknown" {
			continue
		}
		readCtx, cancel := context.WithTimeout(ctx, positiveDuration(m.config.CommandTimeout, time.Minute))
		material, inputErr := m.materialInput(readCtx, task, observed.Commit)
		cancel()
		if inputErr != nil {
			material.Diagnostic = inputErr.Error()
		}
		if e = store.SaveMaterial(ctx, m.config.Identity(), material); e != nil {
			return e
		}
	}
	return nil
}

type materialFreshnessReader interface {
	freshness(context.Context, runnerstate.Candidate) string
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

func (m *projectMaintenance) prepareLatest(ctx context.Context, w taskmodel.FixedWork, commit string, base func(context.Context, taskmodel.FixedWork, string) (string, error)) (taskmodel.FixedWork, string, error) {
	if e := m.refresh(ctx); e != nil {
		return w, "", e
	}
	state, e := m.readinessState(ctx)
	if e != nil {
		return w, "", e
	}
	if e = requireProjectReadiness(state.Readiness); e != nil {
		return w, "", e
	}
	if state.Observation.Commit != commit {
		return w, "", &taskmodel.Error{Code: "project_not_ready", Message: "远端版本已变化，请重新检查后创建候选"}
	}
	var material pmm.Material
	for _, value := range state.Materials {
		if value.TaskID == w.Record.TaskID {
			material = value
			break
		}
	}
	if material.Status != "ready" {
		return w, "", &taskmodel.Error{Code: "project_not_ready", Message: "项目未就绪：当前任务已变化，请重新核实"}
	}
	w.ConfirmedDigest = w.Contract.Digest
	w.Contract = material.Contract
	w.MaterialDigest = material.Digest
	w.FrozenTestPlan = append(json.RawMessage(nil), material.TestPlan...)
	b, e := base(ctx, w, commit)
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
	arrangementPath, _ := testmodel.ArrangementPath(c.Contract.PlanRef.Path)
	refs = append(refs, taskmodel.FileReference{Path: arrangementPath, Commit: c.BaseRevision})
	entry, readErr := testmodel.ReadProjectEntry(func(path string) ([]byte, error) { return m.readCommittedMaterial(ctx, c.BaseRevision, path) })
	if readErr != nil {
		return nil, readErr
	}
	for _, path := range entry.Paths() {
		refs = append(refs, taskmodel.FileReference{Path: path, Commit: c.BaseRevision})
	}
	seen := map[string]bool{}
	for _, r := range refs {
		if seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		before, err := m.readCommittedMaterial(ctx, r.Commit, r.Path)
		if err != nil {
			return nil, err
		}
		after, err := m.readCommittedMaterial(ctx, latest.Commit, r.Path)
		f := file{Path: r.Path, Before: string(before), After: string(after)}
		if err != nil {
			f.Diagnostic = err.Error()
		}
		files = append(files, f)
	}
	var latestPlan testmodel.Snapshot
	_ = json.Unmarshal(latest.TestPlan, &latestPlan)
	latestDefinitions, _ := json.MarshalIndent(latestPlan.Definitions, "", "  ")
	after := string(latestDefinitions)
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

type taskPreparation struct {
	Commit     string `json:"commit"`
	Status     string `json:"status"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

func (m *projectMaintenance) preparation(latest pmm.Material) taskPreparation {
	status := latest.Status
	if o := m.current(); o.Commit != latest.Commit || o.Diagnostic != "" {
		status = "unknown"
	}
	return taskPreparation{Commit: latest.Commit, Status: status, Diagnostic: latest.Diagnostic}
}

// Polling startup readiness must not read complete documents or invoke Git.
func (m *projectMaintenance) taskReadiness(ctx context.Context, id string) (taskPreparation, error) {
	store, ok := m.store.(materialStore)
	if !ok {
		return taskPreparation{}, errors.New("material store unavailable")
	}
	latest, err := store.Material(ctx, m.config.Identity(), id)
	if errors.Is(err, storageports.ErrNotFound) {
		graph, graphErr := m.store.TaskGraphSnapshot(ctx, m.config.Identity())
		if graphErr != nil {
			return taskPreparation{}, graphErr
		}
		for _, task := range graph.Tasks {
			if task.TaskID == id {
				return taskPreparation{Status: "unprepared", Diagnostic: "等待项目检查读取 Git 任务材料"}, nil
			}
		}
	}
	if err != nil {
		return taskPreparation{}, err
	}
	if latest.Commit == m.current().Commit && len(latest.MissingTests) > 0 {
		graph, graphErr := m.store.TaskGraphSnapshot(ctx, m.config.Identity())
		if graphErr != nil {
			return taskPreparation{}, graphErr
		}
		for _, task := range graph.Tasks {
			if task.TaskID != id {
				continue
			}
			waiting, waitErr := m.waitingForPlannedTests(ctx, task, graph, latest)
			if waitErr != nil {
				return taskPreparation{}, waitErr
			}
			if waiting {
				latest.Status = "waiting_tests"
				latest.Diagnostic = "等待同批前置任务交付并登记测试：" + strings.Join(latest.MissingTests, ", ")
			}
		}
	}
	return m.preparation(latest), nil
}

func (m *projectMaintenance) taskDocuments(ctx context.Context, id string) (any, error) {
	store, ok := m.store.(materialStore)
	if !ok {
		return nil, errors.New("material store unavailable")
	}
	latest, e := store.Material(ctx, m.config.Identity(), id)
	if errors.Is(e, storageports.ErrNotFound) {
		status, err := m.taskReadiness(ctx, id)
		if err != nil {
			return nil, err
		}
		return struct {
			taskPreparation
			Files []any `json:"files"`
		}{status, []any{}}, nil
	}
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
	arrangementPath, _ := testmodel.ArrangementPath(latest.Contract.PlanRef.Path)
	refs = append(refs, taskmodel.FileReference{Path: arrangementPath})
	entry, readErr := testmodel.ReadProjectEntry(func(path string) ([]byte, error) { return m.readCommittedMaterial(ctx, latest.Commit, path) })
	if readErr != nil {
		return nil, readErr
	}
	for _, path := range entry.Paths() {
		refs = append(refs, taskmodel.FileReference{Path: path})
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		if ref.Path == "" || seen[ref.Path] {
			continue
		}
		seen[ref.Path] = true
		body, err := m.readCommittedMaterial(ctx, latest.Commit, ref.Path)
		d := document{Path: ref.Path, Body: string(body)}
		if err != nil {
			d.Diagnostic = err.Error()
		}
		files = append(files, d)
	}

	preparation, err := m.taskReadiness(ctx, id)
	if err != nil {
		return nil, err
	}
	return struct {
		taskPreparation
		Files []document `json:"files"`
	}{preparation, files}, nil
}
