package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
	progresstools "github.com/erranliu/SteerLine/tools"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func pmGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	b, e := c.CombinedOutput()
	if e != nil {
		t.Fatalf("Git %v: %v %s", args, e, b)
	}
	return strings.TrimSpace(string(b))
}
func TestPMRemoteFreshObservedCommitAndRewrite(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "SteerlineTest")
	remote := filepath.Join(root, "origin.git")
	pmGit(t, root, "init", "--bare", "--initial-branch=main", remote)
	pmGit(t, root, "clone", remote, work)
	pmGit(t, work, "config", "user.name", "PM test")
	pmGit(t, work, "config", "user.email", "pm@example.invalid")
	_ = os.WriteFile(filepath.Join(work, "code.txt"), []byte("one"), 0600)
	pmGit(t, work, "add", ".")
	pmGit(t, work, "-c", "commit.gpgsign=false", "commit", "-qm", "one")
	pmGit(t, work, "push", "origin", "main")
	c := Config{Repository: work, Remote: "origin", Branch: "main", Git: backend.Command{Executable: "git"}, ProcessHost: resourceprocess.New(time.Minute)}
	m := newProjectMaintenance(c, nil)
	ctx := context.Background()
	first, e := m.check(ctx)
	if e != nil || first.Commit == "" {
		t.Fatal(first, e)
	}
	_ = os.WriteFile(filepath.Join(work, "code.txt"), []byte("two"), 0600)
	pmGit(t, work, "add", ".")
	pmGit(t, work, "-c", "commit.gpgsign=false", "commit", "-qm", "two")
	pmGit(t, work, "push", "origin", "main")
	second, e := m.check(ctx)
	if e != nil || second.Commit == first.Commit || second.Rewritten {
		t.Fatal("fresh fetch not used", second, e)
	}
	pmGit(t, remote, "update-ref", "refs/heads/main", first.Commit)
	rewritten, e := m.check(ctx)
	if e != nil || !rewritten.Rewritten {
		t.Fatal("history rewrite hidden", rewritten, e)
	}
	pmGit(t, remote, "update-ref", "-d", "refs/heads/main")
	failed, e := m.check(ctx)
	if e == nil || failed.Diagnostic == "" || failed.Commit != rewritten.Commit || failed.LastSuccess != rewritten.LastSuccess {
		t.Fatal("failure erased observation", failed, e)
	}
	if got := pmGit(t, work, "rev-parse", "HEAD"); got != second.Commit {
		t.Fatal("observer changed local branch")
	}
}
func TestPMRemoteSharesInflightObservation(t *testing.T) {
	// Invalid Git configuration makes any accidental second fetch observable.
	m := newProjectMaintenance(Config{}, nil)
	done := make(chan struct{})
	m.checking = done
	result := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			o, e := m.check(context.Background())
			if e == nil && o.Commit != "shared" {
				e = fmt.Errorf("different observation: %+v", o)
			}
			result <- e
		}()
	}
	m.mu.Lock()
	m.observation = pm.Observation{Commit: "shared"}
	close(done)
	m.mu.Unlock()
	for i := 0; i < 2; i++ {
		if e := <-result; e != nil {
			t.Fatal(e)
		}
	}
}

type pmsStore struct {
	graph       storageports.TaskGraphSnapshot
	contract    taskmodel.TaskContract
	materials   map[string]pm.Material
	generations map[string]testmanagement.Generation
	receipts    map[string]json.RawMessage
	batch       pm.GenerationBatch
	profile     string
	candidates  map[string]runnerstate.Candidate
}

func (s *pmsStore) TaskGraphSnapshot(context.Context, string) (storageports.TaskGraphSnapshot, error) {
	return s.graph, nil
}
func (s *pmsStore) TaskContract(context.Context, string, taskmodel.TaskRecord) (taskmodel.TaskContract, error) {
	return s.contract, nil
}
func (s *pmsStore) Material(_ context.Context, p, t string) (pm.Material, error) {
	m, ok := s.materials[t]
	if !ok {
		return m, storageports.ErrNotFound
	}
	return m, nil
}
func (s *pmsStore) SaveMaterial(_ context.Context, p string, m pm.Material) error {
	s.materials[m.TaskID] = m
	return nil
}
func (s *pmsStore) MaterialProfile(context.Context, string) string { return s.profile }
func (s *pmsStore) SetMaterialProfile(_ context.Context, p, v string) error {
	s.profile = v
	return nil
}
func (s *pmsStore) Generation(_ context.Context, p, id string) (testmanagement.Generation, error) {
	g, ok := s.generations[id]
	if !ok {
		return g, testmanagement.ErrNotFound
	}
	return g, nil
}
func (s *pmsStore) Generations(context.Context, string) ([]testmanagement.Generation, error) {
	out := []testmanagement.Generation{}
	for _, g := range s.generations {
		out = append(out, g)
	}
	return out, nil
}
func (s *pmsStore) SaveGeneration(_ context.Context, g testmanagement.Generation, rev uint64) error {
	if s.generations[g.ID].Revision != rev {
		return fmt.Errorf("revision mismatch")
	}
	s.generations[g.ID] = g
	return nil
}
func (s *pmsStore) MaintenanceReceipt(_ context.Context, p, id string) (json.RawMessage, error) {
	r, ok := s.receipts[id]
	if !ok {
		return nil, storageports.ErrNotFound
	}
	return r, nil
}
func (s *pmsStore) SaveMaintenanceOperation(_ context.Context, p, id string, b []byte) error {
	s.receipts[id] = append(json.RawMessage(nil), b...)
	return nil
}
func (s *pmsStore) Candidate(_ context.Context, p, id string) (runnerstate.Candidate, error) {
	c, ok := s.candidates[id]
	if !ok {
		return c, storageports.ErrNotFound
	}
	return c, nil
}

type pmsCLI struct {
	submits int
	request runnerclient.Request
	run     runnerclient.Run
	lost    bool
	t       *testing.T
}

func (f *pmsCLI) Submit(_ context.Context, r runnerclient.Request) (runnerclient.Receipt, error) {
	f.submits++
	f.request = r
	var in runnercontract.GenerateRunInput
	_ = json.Unmarshal(r.Input, &in)
	f.run = runnerclient.Run{RunID: fmt.Sprint("generation-", f.submits), Revision: 1, Workflow: in.Workflow, Correlation: in.Correlation, InputDigest: fmt.Sprint("input-", f.submits)}
	if in.Generate.Repository != nil {
		f.run.RepositorySource = appRepositoryManifest(f.t, *in.Generate.Repository)
	}
	if f.lost {
		return runnerclient.Receipt{}, &runnerclient.Failure{Unknown: true}
	}
	return runnerclient.Receipt{RunID: f.run.RunID, ReceiptStatus: "accepted"}, nil
}
func (f *pmsCLI) Inspect(context.Context, string, string) (runnerclient.Inspection, error) {
	run := f.run
	return runnerclient.Inspection{Run: &run, Command: &runnerclient.Command{Command: "submit", RequestID: f.request.RequestID, RunID: &run.RunID}}, nil
}
func (f *pmsCLI) Call(context.Context, string, []string, *runnerclient.Request) (runnerclient.Response, error) {
	return runnerclient.Response{}, fmt.Errorf("unexpected call")
}
func (f *pmsCLI) finish() {
	f.run.Revision++
	f.run.StopStatus = "stopped"
	f.run.ContentResult = &runnercontract.ContentResult{Status: "succeeded", JSON: json.RawMessage(`{"tests":["one"]}`), RepositorySource: f.run.RepositorySource, Source: runnercontract.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "command", TurnID: "turn"}}
}
func pmsFixture(t *testing.T) (*projectMaintenance, *pmsStore, *pmsCLI) {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "SteerlineTest")
	remote := filepath.Join(root, "origin.git")
	pmGit(t, root, "init", "--bare", "--initial-branch=main", remote)
	pmGit(t, root, "clone", remote, repo)
	pmGit(t, repo, "config", "user.name", "PM test")
	pmGit(t, repo, "config", "user.email", "pm@example.invalid")
	files := map[string]string{"SteerlineFlow/environment-project.json": `{"version":1,"requirements":["SteerlineFlow/testing.md"],"testing_catalogue":"SteerlineFlow/testing/test-list.json"}`, "task.spec.md": "# Spec\n## Acceptance\nOne\n", "task.plan.md": "# Plan\nTest one\n", "SteerlineFlow/testing.md": "# Testing\nRules\n", "SteerlineFlow/testing/test-list.json": `{"tests":[{"id":"one","name":"One","level":"L1","environment":"isolated","command":"test","check":"passes"}]}`}
	rules, err := prompts.TestingRules()
	require(t, err)
	files["SteerlineFlow/testing/runtime-progress.md"] = string(rules)
	files["tools/testing/RuntimeProgress.ps1"] = progresstools.PowerShellWriter
	for name, body := range files {
		p := filepath.Join(repo, name)
		if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
	}
	pmGit(t, repo, "add", ".")
	pmGit(t, repo, "-c", "commit.gpgsign=false", "commit", "-qm", "materials")
	pmGit(t, repo, "push", "origin", "main")
	commit := pmGit(t, repo, "rev-parse", "HEAD")
	c := taskmodel.TaskContract{ContractID: "contract", SpecRef: taskmodel.FileReference{Path: "task.spec.md", Commit: commit, SHA256: pm.Digest([]byte(files["task.spec.md"]))}, PlanRef: taskmodel.FileReference{Path: "task.plan.md", Commit: commit, SHA256: pm.Digest([]byte(files["task.plan.md"]))}}
	c.Digest, _ = taskmodel.ContractDigest(c)
	task := taskmodel.TaskRecord{TaskID: "101", Status: "open"}
	store := &pmsStore{graph: storageports.TaskGraphSnapshot{Tasks: []taskmodel.TaskRecord{task}}, contract: c, materials: map[string]pm.Material{}, generations: map[string]testmanagement.Generation{}, receipts: map[string]json.RawMessage{}, candidates: map[string]runnerstate.Candidate{}}
	profile := RunnerProfile{ID: "local", Name: "Local", Execution: runnerclient.ExecutionInput{Adapter: "codex-app-server", Target: runnerclient.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"read_only"}`)}}
	m := newProjectMaintenance(Config{Repository: repo, Remote: "origin", Branch: "main", Git: backend.Command{Executable: "git"}, ProcessHost: resourceprocess.New(time.Minute), RunnerExecution: RunnerExecutionConfig{DefaultProfile: "local", Profiles: []RunnerProfile{profile}}}, store)
	cli := &pmsCLI{t: t}
	cat := promotionCatalog(t)
	service := &testmanagement.GenerationService{Project: m.config.Identity(), Repository: repo, Store: store, Runner: cli, ReadCommitted: m.read, Prepare: func(context.Context, string) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
		return profile.Execution, cat, nil
	}}
	m.config.runtimeCatalog = cat
	m.config.RunnerExecution.Profiles[0].Workflow = runnerclient.Workflow{ID: "development", Version: 2}
	m.config.RunnerExecution.Profiles[0].Execution.Settings = json.RawMessage(`{"access_mode":"full_access"}`)
	m.tests = &testManagementPort{service: service}
	return m, store, cli
}
func TestPMSGenerationFreshnessAndCandidateVersions(t *testing.T) {
	ctx := context.Background()
	m, store, cli := pmsFixture(t)
	if _, e := m.check(ctx); e != nil {
		t.Fatal(e)
	}
	if e := m.refresh(ctx); e != nil {
		t.Fatal(e)
	}
	if cli.submits != 1 {
		t.Fatal("initial automatic generation missing")
	}
	if e := m.refresh(ctx); e != nil || cli.submits != 1 {
		t.Fatal("duplicate auto generation", e)
	}
	cli.finish()
	if e := m.tests.service.Observe(ctx); e != nil {
		t.Fatal(e)
	}
	if e := m.refresh(ctx); e != nil {
		t.Fatal(e)
	}
	first := store.materials["101"]
	if first.Status != "ready" {
		t.Fatal(first)
	}
	work := taskmodel.FixedWork{Record: store.graph.Tasks[0], Contract: store.contract}
	base := func(context.Context, taskmodel.FixedWork, bool) (string, error) { return m.current().Commit, nil }
	fixed, b, e := m.prepare(ctx, work, false, base)
	if e != nil || b != first.Commit || fixed.MaterialDigest != first.Digest {
		t.Fatal("latest freeze", fixed, b, e)
	}
	run := runnerclient.RunInput{Execution: runnerclient.ExecutionInput{Adapter: "codex-app-server", Target: runnerclient.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"full_access"}`)}, Workflow: runnerclient.Workflow{ID: "development", Version: 2}}
	snapshot, e := m.config.prepareTestPlan(ctx, fixed, &run)
	if e != nil {
		t.Fatal(e)
	}
	if len(snapshot) == 0 || len(run.Documents.References) != 0 || run.ResultFile == nil || !strings.HasSuffix(run.Workflow.PromptOverrides["implement.execute"], testPromptContext(t, snapshot)) {
		t.Fatal("compact system context not frozen")
	}
	candidate := runnerstate.Candidate{ID: "original", TaskID: "101", MaterialDigest: first.Digest, Contract: fixed.Contract, BaseRevision: b, TestPlan: snapshot}
	store.candidates[candidate.ID] = candidate
	if m.freshness(ctx, candidate) != "current" {
		t.Fatal("new candidate not current")
	}
	// Source-only commits reuse generation and keep document eligibility.
	pmGit(t, m.config.Repository, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "source only")
	pmGit(t, m.config.Repository, "push", "origin", "main")
	if _, e = m.check(ctx); e != nil {
		t.Fatal(e)
	}
	if e = m.refresh(ctx); e != nil {
		t.Fatal(e)
	}
	if cli.submits != 1 || m.freshness(ctx, candidate) != "current" {
		t.Fatal("commit-only change invalidated material")
	}
	if e = os.WriteFile(filepath.Join(m.config.Repository, "task.plan.md"), []byte("# Plan\nChanged test requirements\n"), 0600); e != nil {
		t.Fatal(e)
	}
	pmGit(t, m.config.Repository, "add", ".")
	pmGit(t, m.config.Repository, "-c", "commit.gpgsign=false", "commit", "-qm", "new Plan")
	pmGit(t, m.config.Repository, "push", "origin", "main")
	if _, e = m.check(ctx); e != nil {
		t.Fatal(e)
	}
	if e = m.refresh(ctx); e != nil {
		t.Fatal(e)
	}
	if cli.submits != 2 || m.freshness(ctx, candidate) != "outdated" {
		t.Fatal("Plan change failed to generate/expire")
	}
	if _, _, e = m.prepare(ctx, work, true, base); e == nil {
		t.Fatal("new candidate used unready generation")
	}
	cli.finish()
	if e = m.tests.service.Observe(ctx); e != nil {
		t.Fatal(e)
	}
	if e = m.refresh(ctx); e != nil {
		t.Fatal(e)
	}
	next, _, e := m.prepare(ctx, work, true, base)
	if e != nil || next.Contract.PlanRef.Commit == fixed.Contract.PlanRef.Commit {
		t.Fatal("later candidate retained Task first version", e)
	}
	if _, e = m.candidateDocuments(ctx, candidate.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(m.config.Repository, "task.tests.json")); !os.IsNotExist(e) {
		t.Fatal("generation wrote a repository sidecar", e)
	}
	m.mu.Lock()
	m.observation.Diagnostic = "offline"
	m.mu.Unlock()
	if m.freshness(ctx, candidate) != "unknown" {
		t.Fatal("offline claimed current eligibility")
	}
}

func TestPMSRetriesTransientStaticMaterialReadsAtSameCommit(t *testing.T) {
	for _, name := range []string{testmodel.WriterPath, "SteerlineFlow/runner-policies/git.md"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			m, store, cli := pmsFixture(t)
			observation, err := m.check(ctx)
			require(t, err)
			host := m.config.ProcessHost
			m.config.ProcessHost = failingDocumentHost{ManagedProcessHost: host, stage: name, cause: context.DeadlineExceeded}
			require(t, m.refresh(ctx))
			if got := store.materials["101"]; got.Status != "pending" || got.Commit != observation.Commit || cli.submits != 0 {
				t.Fatalf("failed pinned read should leave preparation pending: %+v", got)
			}
			m.config.ProcessHost = host
			require(t, m.refresh(ctx))
			cli.finish()
			require(t, m.tests.service.Observe(ctx))
			require(t, m.refresh(ctx))
			if got := store.materials["101"]; got.Status != "ready" || got.Commit != observation.Commit || len(got.TestPlan) == 0 || cli.submits != 1 {
				t.Fatalf("same pinned commit did not recover once: %+v", got)
			}
		})
	}
}

func TestNative101GeneratedArrangementStartsWithoutSidecar(t *testing.T) {
	ctx := context.Background()
	m, store, cli := pmsFixture(t)
	repo := m.config.Repository
	const planPath = "SteerlineFlow/plans/native-101.plan.md"
	const expected = `{"tests":["N101-CONTENT","N101-SCOPE","demo.progress.roundtrip","demo.progress.retry","demo.progress.cleanup"]}`
	if source := os.Getenv("STEERLINE_TASK101_REPOSITORY"); source != "" {
		const fixed = "9d81d9c2652177faf6354d4e9f9f3aec79535d11"
		pmGit(t, repo, "fetch", "--no-tags", "--", source, fixed)
		pmGit(t, repo, "-c", "core.hooksPath=/dev/null", "checkout", "-B", "main", fixed)
		store.contract.SpecRef.Path = "SteerlineFlow/spec/native-101.spec.md"
	} else {
		require(t, os.MkdirAll(filepath.Join(repo, filepath.Dir(planPath)), 0700))
		pmGit(t, repo, "mv", "task.plan.md", planPath)
		var selection testmodel.Arrangement
		require(t, json.Unmarshal([]byte(expected), &selection))
		catalogue := testmanagement.Catalogue{}
		// Catalogue order differs from the generated execution order.
		for i := len(selection.Tests) - 1; i >= 0; i-- {
			id := selection.Tests[i]
			catalogue.Tests = append(catalogue.Tests, testmanagement.Definition{ID: id, Name: id, Level: "L4", Environment: "isolated", Command: "sample test", Check: "passes"})
		}
		raw, err := json.Marshal(catalogue)
		require(t, err)
		require(t, os.WriteFile(filepath.Join(repo, testmodel.CataloguePath), raw, 0600))
		pmGit(t, repo, "add", ".")
		pmGit(t, repo, "-c", "commit.gpgsign=false", "commit", "-qm", "native 101 materials")
	}
	store.contract.PlanRef.Path = planPath
	store.contract.Digest, _ = taskmodel.ContractDigest(store.contract)
	pmGit(t, repo, "push", "--force", "origin", "main")
	_, err := m.check(ctx)
	require(t, err)
	require(t, m.refresh(ctx))
	cli.finish()
	cli.run.ContentResult.JSON = json.RawMessage(expected)
	require(t, m.tests.service.Observe(ctx))
	require(t, m.refresh(ctx))
	ready := store.materials["101"]
	if got := ready.Status; got != "ready" {
		t.Fatal("generation not ready", got)
	}
	work := taskmodel.FixedWork{Record: store.graph.Tasks[0], Contract: store.contract}
	fixed, commit, err := m.prepare(ctx, work, false, func(context.Context, taskmodel.FixedWork, bool) (string, error) {
		return m.current().Commit, nil
	})
	require(t, err)
	run := callerTestRun()
	snapshot, err := m.config.prepareTestPlan(ctx, fixed, &run)
	require(t, err)
	if string(snapshot) != string(ready.TestPlan) || run.ResultFile == nil {
		t.Fatal("start did not freeze the ready arrangement")
	}
	arrangement, err := base64.StdEncoding.DecodeString(run.ResultFile.InitialContentBase64)
	require(t, err)
	if string(arrangement) != expected {
		t.Fatal("generated order changed", string(arrangement))
	}
	sidecar := "SteerlineFlow/plans/native-101.tests.json"
	if pmGit(t, repo, "ls-tree", commit, "--", sidecar) != "" || strings.Contains(run.Workflow.PromptOverrides["implement.execute"], expected) {
		t.Fatal("arrangement leaked into Git or the prompt")
	}
	t.Logf("101 ready/start: base=%s bytes=%d SHA-256=%s", commit, len(arrangement), testmanagement.ContentDigest(arrangement))
}
func TestPMSFastForwardAndOriginalUpdateReplay(t *testing.T) {
	ctx := context.Background()
	m, _, _ := pmsFixture(t)
	old := pmGit(t, m.config.Repository, "rev-parse", "HEAD")
	remoteRepo := filepath.Join(t.TempDir(), "SteerlineTest")
	pmGit(t, filepath.Dir(remoteRepo), "clone", filepath.Join(filepath.Dir(m.config.Repository), "origin.git"), remoteRepo)
	pmGit(t, remoteRepo, "-c", "user.name=PM", "-c", "user.email=pm@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "remote update")
	pmGit(t, remoteRepo, "push", "origin", "main")
	if _, e := m.check(ctx); e != nil {
		t.Fatal(e)
	}
	local := m.localBaseline(ctx)
	if local.Status != "behind" || local.Behind != 1 || !local.CanUpdate {
		t.Fatal(local)
	}
	if e := os.WriteFile(filepath.Join(m.config.Repository, "unsaved.txt"), []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	if m.localBaseline(ctx).CanUpdate {
		t.Fatal("dirty workspace allowed update")
	}
	if _, e := m.updateLocal(ctx, "blocked"); e == nil {
		t.Fatal("dirty update accepted")
	}
	if e := os.Remove(filepath.Join(m.config.Repository, "unsaved.txt")); e != nil {
		t.Fatal(e)
	}
	result, e := m.updateLocal(ctx, "original")
	if e != nil {
		t.Fatal(e)
	}
	fact := result.(updateFact)
	if fact.Status != "applied" || fact.Before != old {
		t.Fatal(fact)
	}
	if _, e = m.updateLocal(ctx, "original"); e != nil {
		t.Fatal(e)
	}
	if _, e = m.commitLog(ctx, "0", "history"); e != nil {
		t.Fatal(e)
	}

	// An ignored local file must not be silently replaced by a newly tracked one.
	if e = os.WriteFile(filepath.Join(remoteRepo, "ignored.txt"), []byte("remote"), 0600); e != nil {
		t.Fatal(e)
	}
	pmGit(t, remoteRepo, "add", "ignored.txt")
	pmGit(t, remoteRepo, "-c", "user.name=PM", "-c", "user.email=pm@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "Track previously ignored path")
	pmGit(t, remoteRepo, "push", "origin", "main")
	if e = os.WriteFile(filepath.Join(m.config.Repository, ".git", "info", "exclude"), []byte("ignored.txt\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(m.config.Repository, "ignored.txt"), []byte("keep local ignored bytes"), 0600); e != nil {
		t.Fatal(e)
	}
	before := pmGit(t, m.config.Repository, "rev-parse", "HEAD")
	result, e = m.updateLocal(ctx, "ignored-conflict")
	if e != nil {
		t.Fatal(e)
	}
	blocked := result.(updateFact)
	if blocked.Status == "applied" || blocked.Diagnostic == "" {
		t.Fatal("ignored file was overwritten", blocked)
	}
	bytes, e := os.ReadFile(filepath.Join(m.config.Repository, "ignored.txt"))
	if e != nil || string(bytes) != "keep local ignored bytes" {
		t.Fatal("ignored bytes changed", e)
	}
	if pmGit(t, m.config.Repository, "rev-parse", "HEAD") != before {
		t.Fatal("ignored conflict moved HEAD")
	}
	if e = os.Remove(filepath.Join(m.config.Repository, "ignored.txt")); e != nil {
		t.Fatal(e)
	}
	result, e = m.updateLocal(ctx, "ignored-resolved")
	if e != nil || result.(updateFact).Status != "applied" {
		t.Fatal("resolved conflict did not fast-forward", result, e)
	}
	pmGit(t, m.config.Repository, "-c", "user.name=PM", "-c", "user.email=pm@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "local only")
	if _, e = m.updateLocal(ctx, "other"); e == nil {
		t.Fatal("local-ahead update accepted")
	}
}

func TestPMSGenerationUnknownDoesNotOfferAutomaticRetry(t *testing.T) {
	ctx := context.Background()
	m, store, cli := pmsFixture(t)
	m.config.RunnerExecution.DefaultProfile = ""
	if _, e := m.check(ctx); e != nil {
		t.Fatal(e)
	}
	if e := m.refresh(ctx); e != nil {
		t.Fatal(e)
	}
	if store.materials["101"].Status != "configuration_required" || cli.submits != 0 {
		t.Fatal("missing profile started generation")
	}
	store.profile = "local"
	cli.lost = true
	if e := m.refresh(ctx); e != nil {
		t.Fatal(e)
	}
	if store.materials["101"].Status != "unknown" || cli.submits != 1 {
		t.Fatal("unknown submit lost original state", store.materials["101"], cli.submits)
	}
	if e := m.refresh(ctx); e != nil || cli.submits != 1 {
		t.Fatal("unknown auto-retried", e, cli.submits)
	}
	if e := m.tests.service.Observe(ctx); e != nil {
		t.Fatal(e)
	}
	if e := m.refresh(ctx); e != nil || cli.submits != 1 {
		t.Fatal("observation resubmitted", e)
	}
}

func TestPMSBaselineComparisonAndBlockedUpdates(t *testing.T) {
	ctx := context.Background()
	m, _, _ := pmsFixture(t)
	original := pmGit(t, m.config.Repository, "rev-parse", "HEAD")
	// A separate clone advances only the isolated bare remote.
	repo := filepath.Join(t.TempDir(), "SteerlineTest")
	pmGit(t, filepath.Dir(repo), "clone", filepath.Join(filepath.Dir(m.config.Repository), "origin.git"), repo)
	pmGit(t, repo, "-c", "user.name=PM", "-c", "user.email=pm@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "Remote title")
	pmGit(t, repo, "push", "origin", "main")
	if _, e := m.check(ctx); e != nil {
		t.Fatal(e)
	}
	remote := m.current().Commit
	info := m.commitInfo(ctx, remote, true)
	if info.Title != "Remote title" || info.Time == "" || info.Author != "PM" {
		t.Fatal(info)
	}
	if _, e := m.commitLog(ctx, "-1", "history"); e == nil {
		t.Fatal("negative page accepted")
	}
	if _, e := m.commitLog(ctx, "0", "invalid"); e == nil {
		t.Fatal("invalid scope accepted")
	}
	pmGit(t, m.config.Repository, "checkout", "-b", "other")
	if got := m.localBaseline(ctx); got.CanUpdate || !strings.Contains(got.Reason, "other") {
		t.Fatal("wrong branch allowed", got)
	}
	pmGit(t, m.config.Repository, "checkout", "main")
	lock := filepath.Join(m.config.Repository, ".git", "steerline-update.lock")
	if e := os.WriteFile(lock, []byte("other operation"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := m.updateLocal(ctx, "locked"); e == nil {
		t.Fatal("locked update accepted")
	}
	if e := os.Remove(lock); e != nil {
		t.Fatal(e)
	}
	state := filepath.Join(m.config.Repository, ".git", "MERGE_HEAD")
	if e := os.WriteFile(state, []byte(remote), 0600); e != nil {
		t.Fatal(e)
	}
	if got := m.localBaseline(ctx); got.CanUpdate || got.Reason == "" {
		t.Fatal("merge in progress allowed", got)
	}
	if e := os.Remove(state); e != nil {
		t.Fatal(e)
	}
	pmGit(t, m.config.Repository, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "Local title")
	got := m.localBaseline(ctx)
	if got.Status != "diverged" || got.Ahead != 1 || got.Behind != 1 || got.CanUpdate {
		t.Fatal(got)
	}
	if _, e := m.updateLocal(ctx, "diverged"); e == nil {
		t.Fatal("diverged update accepted")
	}
	if head := pmGit(t, m.config.Repository, "rev-parse", "HEAD"); head == original || head == remote {
		t.Fatal("lost local commit")
	}
	pmGit(t, m.config.Repository, "checkout", "--detach", original)
	if got := m.localBaseline(ctx); got.CanUpdate || got.Reason == "" {
		t.Fatal("detached allowed", got)
	}
}

func TestPMSBaselineGiteaPRMetadata(t *testing.T) {
	for _, mode := range []string{"associated", "none", "failed", "invalid", "foreign", "hidden"} {
		t.Run(mode, func(t *testing.T) {
			var host string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/repos/owner/repo" {
					if mode == "hidden" {
						w.WriteHeader(404)
					} else {
						fmt.Fprint(w, `{"name":"repo"}`)
					}
					return
				}
				if r.Method != "GET" || r.URL.Path != "/api/v1/repos/owner/repo/commits/"+strings.Repeat("a", 40)+"/pull" || r.Header.Get("Authorization") != "token fixture-token" {
					t.Error("incorrect read-only PR query")
				}
				switch mode {
				case "none", "hidden":
					w.WriteHeader(404)
				case "failed":
					w.WriteHeader(503)
				case "invalid":
					fmt.Fprint(w, "invalid")
				case "foreign":
					fmt.Fprint(w, `{"html_url":"https://elsewhere.invalid/owner/repo/pulls/1"}`)
				default:
					fmt.Fprintf(w, `{"html_url":%q}`, host+"/owner/repo/pulls/3")
				}
			}))
			defer server.Close()
			host = server.URL
			t.Setenv("PMS_GITEA_TOKEN", "fixture-token")
			m := &projectMaintenance{config: Config{PR: executionmodel.PRProviderConfig{Kind: "gitea", BaseURL: host, Repository: "owner/repo", TokenEnv: "PMS_GITEA_TOKEN"}}}
			urls, diagnostic := m.commitPullRequests(context.Background(), strings.Repeat("a", 40))
			if mode == "associated" {
				if len(urls) != 1 || diagnostic != "" {
					t.Fatal(urls, diagnostic)
				}
			} else if mode == "none" {
				if len(urls) != 0 || diagnostic != "" {
					t.Fatal(urls, diagnostic)
				}
			} else if len(urls) != 0 || diagnostic == "" {
				t.Fatal("provider failure was reported as no PR", urls, diagnostic)
			}
		})
	}
}

type metadataCommandProbe struct {
	backend.ManagedProcessHost
	commands []string
}

func (p *metadataCommandProbe) StartManaged(command *exec.Cmd) (backend.StopHandle, error) {
	p.commands = append(p.commands, command.Path)
	return nil, fmt.Errorf("metadata process intercepted")
}
func TestPMSMetadataHonorsConfiguredCommand(t *testing.T) {
	for _, executable := range []string{"controlled-provider", filepath.Join(t.TempDir(), "gh.exe")} {
		t.Run(filepath.Base(executable), func(t *testing.T) {
			probe := &metadataCommandProbe{}
			m := &projectMaintenance{config: Config{ProcessHost: probe, PR: executionmodel.PRProviderConfig{Kind: "command", BaseURL: "https://github.com", Repository: "e2e/project", Query: backend.Command{Executable: executable}}}}
			urls, diagnostic := m.commitPullRequests(context.Background(), strings.Repeat("a", 40))
			if len(urls) != 0 || diagnostic == "" {
				t.Fatal("unsupported/failed metadata query was reported as no PR")
			}
			if executable == "controlled-provider" {
				if len(probe.commands) != 0 {
					t.Fatal("custom provider escaped to an unconfigured CLI", probe.commands)
				}
			} else if len(probe.commands) != 1 || probe.commands[0] != executable {
				t.Fatal("selected GitHub executable was replaced", probe.commands)
			}
		})
	}
}

func (s *pmsStore) GenerationBatch(context.Context, string) (pm.GenerationBatch, error) {
	return s.batch, nil
}
func (s *pmsStore) SaveGenerationBatch(_ context.Context, _ string, b pm.GenerationBatch) error {
	s.batch = b
	return nil
}

func TestPMSGenerationSharesPinnedReadCacheWithoutCrossRequestReuse(t *testing.T) {
	m, _, _ := pmsFixture(t)
	ctx := materialReads(context.Background())
	commit := pmGit(t, m.config.Repository, "rev-parse", "HEAD")
	path := "SteerlineFlow/testing.md"
	expected, err := m.read(ctx, commit, path)
	require(t, err)
	c := m.config
	c.runtimeMaintenance = m
	c.Git.Executable = filepath.Join(t.TempDir(), "unavailable-git.exe")
	m.config.Git = c.Git
	port := newTestManagementPort(c, nil, runnerclient.Client{}, nil)
	got, err := port.service.ReadCommitted(ctx, commit, path)
	require(t, err)
	if string(got) != string(expected) {
		t.Fatal("generation reread or changed pinned policy")
	}
	if _, err := port.service.ReadCommitted(materialReads(context.Background()), commit, path); err == nil {
		t.Fatal("immutable cache leaked into another request")
	}
	if _, err := port.service.ReadCommitted(ctx, strings.Repeat("f", 40), path); err == nil {
		t.Fatal("cache replaced another source commit")
	}
}

func TestPMReservationRejectsPendingEnvironmentObservation(t *testing.T) {
	m, _, _ := pmsFixture(t)
	ctx := context.Background()
	before, err := m.check(ctx)
	require(t, err)
	// Preflight approved this baseline before a new observation began.
	reservations := 0
	require(t, m.reserveBaseline(before.Commit, func() error { reservations++; return nil }))
	entered, release := make(chan struct{}), make(chan struct{})
	m.environmentObservation = func(context.Context, string) error { close(entered); <-release; return nil }
	finished := make(chan error, 1)
	go func() { _, err := m.check(ctx); finished <- err }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("observation did not enter revocation callback")
	}
	err = m.reserveBaseline(before.Commit, func() error { reservations++; return nil })
	close(release)
	require(t, <-finished)
	if err == nil || reservations != 1 {
		t.Fatal("pending observation admitted an old preflight", err, reservations)
	}
	require(t, m.reserveBaseline(before.Commit, func() error { reservations++; return nil }))
}
