package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type plannedTestsStore struct {
	*pmsStore
	rows      []roadmap.Record
	contracts map[string]taskmodel.TaskContract
}

func (s *plannedTestsStore) Roadmaps(context.Context, string) ([]roadmap.Record, error) {
	return s.rows, nil
}
func (s *plannedTestsStore) TaskContract(_ context.Context, _ string, t taskmodel.TaskRecord) (taskmodel.TaskContract, error) {
	return s.contracts[t.TaskID], nil
}

func plannedTestsWaitWithoutBlockingProviderAndRequireDeliveredDefinitions(t *testing.T) {
	ctx := context.Background()
	m, base := pmsFixture(t)
	_, err := m.check(ctx)
	require(t, err)
	a := roadmap.Analysis{Coordination: "Reuse the declared owners and confirmed dependency handoffs.", Tasks: []roadmap.Node{{Key: "tools", ProvidesTests: []string{"new.content"}}, {Key: "sample", Proposed: roadmap.Dependencies{Local: []string{"tools"}}}}}
	r := roadmap.Record{State: "admitted", Source: roadmap.Source{ID: strings.Repeat("a", 32)}, Analysis: &a, CodeCommit: m.current().Commit, DocumentsVerified: true, Mapping: map[string]string{"tools": "101", "sample": "102"}}
	r.ConfirmedDigest = roadmap.ConfirmationDigest(r)
	r.Documents = roadmap.PlannedDocuments(r)
	for _, path := range []string{"SteerlineFlow/environment-project.json", "SteerlineFlow/testing/test-list.json"} {
		raw, err := m.readCommittedMaterial(ctx, r.CodeCommit, path)
		require(t, err)
		body := string(raw)
		r.Materials = append(r.Materials, runnercontract.Document{Path: path, Content: &body})
	}
	files := map[string]string{}
	for i := range r.Documents {
		d := &r.Documents[i]
		d.Spec.Body = "# Spec\n## Scope\nOne\n"
		d.Plan.Body = "# Plan\n## Test\nUse selected tests after predecessor delivery.\n"
		d.Acceptance[0].Body = "# Acceptance\nPending\n"
		d.Tests = []string{"one"}
		files[d.Spec.Path], files[d.Plan.Path], files[d.Acceptance[0].Path] = d.Spec.Body, d.Plan.Body, d.Acceptance[0].Body
	}
	r.Documents[0].ProvidesTests = []string{"new.content"}
	r.Documents[1].Tests = []string{"one", "new.content"}
	for _, d := range r.Documents {
		path := strings.TrimSuffix(d.Plan.Path, ".plan.md") + ".tests.json"
		raw, err := roadmap.TestsJSON(d)
		require(t, err)
		files[path] = raw
	}
	for path, body := range files {
		full := filepath.Join(m.config.Repository, filepath.FromSlash(path))
		require(t, os.MkdirAll(filepath.Dir(full), 0700))
		require(t, os.WriteFile(full, []byte(body), 0600))
	}
	pmGit(t, m.config.Repository, "add", ".")
	pmGit(t, m.config.Repository, "-c", "commit.gpgsign=false", "commit", "-qm", "publish planned tests")
	pmGit(t, m.config.Repository, "push", "origin", "main")
	_, err = m.check(ctx)
	require(t, err)
	store := &plannedTestsStore{pmsStore: base, rows: []roadmap.Record{r}, contracts: map[string]taskmodel.TaskContract{}}
	for i, id := range []string{"101", "102"} {
		d := r.Documents[i]
		c := base.contract
		c.SpecRef = taskmodel.FileReference{Path: d.Spec.Path, Commit: m.current().Commit, SHA256: roadmap.Digest([]byte(d.Spec.Body))}
		c.PlanRef = taskmodel.FileReference{Path: d.Plan.Path, Commit: m.current().Commit, SHA256: roadmap.Digest([]byte(d.Plan.Body))}
		c.Digest, _ = taskmodel.ContractDigest(c)
		store.contracts[id] = c
	}
	store.graph.Tasks = append(store.graph.Tasks, taskmodel.TaskRecord{TaskID: "102", Status: "open"})
	store.graph.ByID = map[string]taskmodel.TaskRecord{"101": store.graph.Tasks[0], "102": store.graph.Tasks[1]}
	store.graph.Edges = []taskmodel.TaskDependencyRecord{{PredecessorID: "101", SuccessorID: "102"}}
	m.store = store
	require(t, m.refresh(ctx))
	state, err := m.readinessState(ctx)
	require(t, err)
	if state.Readiness.Status != "ready" || state.Readiness.Waiting != 1 || store.materials["102"].Status != "invalid" {
		t.Fatal(state.Readiness, store.materials["102"].Diagnostic)
	}
	preparation, err := m.taskReadiness(ctx, "102")
	require(t, err)
	if preparation.Status != "waiting_tests" || !strings.Contains(preparation.Diagnostic, "new.content") {
		t.Fatal(preparation)
	}
	calls := 0
	basePrepare := func(context.Context, taskmodel.FixedWork, bool) (string, error) {
		calls++
		return m.current().Commit, nil
	}
	_, _, err = m.prepareLatest(ctx, taskmodel.FixedWork{Record: store.graph.Tasks[0], Contract: store.contracts["101"]}, false, basePrepare)
	require(t, err)
	if calls != 1 {
		t.Fatal("provider cannot start")
	}
	_, _, err = m.prepareLatest(ctx, taskmodel.FixedWork{Record: store.graph.Tasks[1], Contract: store.contracts["102"]}, false, basePrepare)
	if err == nil || calls != 1 {
		t.Fatal("waiting consumer started")
	}
	// Unrelated or malformed selections are never deferred.
	material := store.materials["102"]
	material.MissingTests = []string{"unpromised"}
	store.materials["102"] = material
	state, err = m.readinessState(ctx)
	require(t, err)
	if state.Readiness.Status != "not_ready" {
		t.Fatal("unpromised ID deferred")
	}
	material.MissingTests = []string{"new.content"}
	store.materials["102"] = material
	// A delivered producer that forgot the definitions is a real failure.
	store.graph.Tasks[0].Status = "closed"
	store.graph.ByID["101"] = store.graph.Tasks[0]
	store.graph.Deliveries = map[string]taskmodel.TaskDeliveryRecord{"101": {TaskID: "101"}}
	state, err = m.readinessState(ctx)
	require(t, err)
	if state.Readiness.Status != "not_ready" {
		t.Fatal("missing delivered output accepted")
	}
	// Once registered in a new commit, the consumer gets real frozen definitions.
	catalogue := `{"tests":[{"id":"one","name":"One","level":"L1","environment":"isolated","command":"test","check":"passes"},{"id":"new.content","name":"New content","level":"L4","environment":"isolated","command":"test-new","check":"new content passes"}]}`
	require(t, os.WriteFile(filepath.Join(m.config.Repository, "SteerlineFlow/testing/test-list.json"), []byte(catalogue), 0600))
	pmGit(t, m.config.Repository, "add", ".")
	pmGit(t, m.config.Repository, "-c", "commit.gpgsign=false", "commit", "-qm", "deliver new test")
	pmGit(t, m.config.Repository, "push", "origin", "main")
	_, err = m.check(ctx)
	require(t, err)
	require(t, m.refresh(ctx))
	var frozen taskmodel.FixedWork
	_, _, err = m.prepareLatest(ctx, taskmodel.FixedWork{Record: store.graph.Tasks[1], Contract: store.contracts["102"]}, false, func(_ context.Context, w taskmodel.FixedWork, _ bool) (string, error) {
		frozen = w
		return m.current().Commit, nil
	})
	require(t, err)
	if store.materials["102"].Status != "ready" || !strings.Contains(string(frozen.FrozenTestPlan), "test-new") {
		t.Fatal(store.materials["102"], string(frozen.FrozenTestPlan))
	}
}
