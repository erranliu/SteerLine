package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestRoadmapAdmissionAtomicNumbersSourceAndReplay(t *testing.T) {
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	files := map[string]string{"SteerlineFlow/spec/roadmap/a.spec.md": "# Spec\n目标", "SteerlineFlow/plans/roadmap/a.plan.md": "# Plan\n实施", "SteerlineFlow/testing/acceptance/roadmap/a.md": "# Acceptance\n通过条件"}
	for name, body := range files {
		full := filepath.Join(c.Repository, filepath.FromSlash(name))
		require(t, os.MkdirAll(filepath.Dir(full), 0700))
		require(t, os.WriteFile(full, []byte(body), 0600))
	}
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "roadmap documents")
	commit := fixtureGit(t, c.Repository, "rev-parse", "HEAD")
	reader, e := OpenNativeTaskProject(ctx, &c)
	require(t, e)
	require(t, reader.Close())
	store, e := OpenNativeTaskCommandStore(ctx, &c)
	require(t, e)
	defer store.Close()
	target, e := c.TaskRepositoryTarget(ctx)
	require(t, e)
	reference := func(path string) taskmodel.FileReference {
		return taskmodel.FileReference{Path: path, Commit: commit, SHA256: roadmap.Digest([]byte(files[path]))}
	}
	batch := func(prefix string, count int) taskmodel.TaskBatchInput {
		out := taskmodel.TaskBatchInput{SchemaVersion: taskmodel.BatchSchemaVersion, ProjectID: c.Identity(), Tasks: []taskmodel.TaskBatchTask{}, Dependencies: []taskmodel.TaskBatchEdge{}}
		for i := 0; i < count; i++ {
			key := fmt.Sprintf("T%d", i+1)
			contract := taskmodel.TaskContract{ContractID: prefix + key, Repository: target, Remote: c.Remote, Branch: c.Branch, SpecRef: reference("SteerlineFlow/spec/roadmap/a.spec.md"), PlanRef: reference("SteerlineFlow/plans/roadmap/a.plan.md"), AcceptanceRefs: []taskmodel.FileReference{reference("SteerlineFlow/testing/acceptance/roadmap/a.md")}}
			contract.Digest, e = taskmodel.ContractDigest(contract)
			require(t, e)
			out.Tasks = append(out.Tasks, taskmodel.TaskBatchTask{LocalKey: key, Title: prefix + key, Type: "task", Contract: contract})
		}
		if count == 2 {
			out.Dependencies = append(out.Dependencies, taskmodel.TaskBatchEdge{Predecessor: taskmodel.TaskBatchReference{LocalKey: "T1"}, Successor: taskmodel.TaskBatchReference{LocalKey: "T2"}})
		}
		return out
	}
	commands := NewTaskCommands(store, c)
	one := batch("one", 2)
	request := strings.Repeat("1", 32)
	first, e := commands.AdmitRoadmap(ctx, "roadmap-one", one, request, 1, "fixture")
	require(t, e)
	if first.TaskIDs["T1"] != "0" || first.TaskIDs["T2"] != "1" {
		t.Fatal(first)
	}
	graph, e := store.TaskGraphSnapshot(ctx, c.Identity())
	require(t, e)
	if len(graph.Tasks) != 2 || len(graph.Edges) != 1 || graph.Edges[0].PredecessorID != "0" || graph.Edges[0].SuccessorID != "1" {
		t.Fatal(graph)
	}
	// Replay does not reread Git or allocate, even when the observer has a newer authority version.
	replayCommands := commands
	replayCommands.Documents = nil
	again, e := replayCommands.AdmitRoadmap(ctx, "roadmap-one", one, request, 999, "fixture")
	require(t, e)
	if roadmap.ValueDigest(again) != roadmap.ValueDigest(first) {
		t.Fatal("receipt changed")
	}
	bad := batch("conflict", 2)
	if _, e = commands.AdmitRoadmap(ctx, "roadmap-one", bad, strings.Repeat("2", 32), 1, "fixture"); e == nil {
		t.Fatal("duplicate source assigned twice")
	}
	after, e := store.TaskGraphSnapshot(ctx, c.Identity())
	require(t, e)
	if len(after.Tasks) != 2 || after.Authority.GraphRevision != graph.Authority.GraphRevision {
		t.Fatal("failed source transaction leaked Tasks/graph")
	}
	mapping, e := store.RoadmapMapping(ctx, c.Identity(), "roadmap-one")
	require(t, e)
	if mapping["T1"] != "0" || len(mapping) != 2 {
		t.Fatal(mapping)
	}
	second, e := commands.AdmitRoadmap(ctx, "roadmap-two", batch("two", 1), strings.Repeat("3", 32), 1, "fixture")
	require(t, e)
	if second.TaskIDs["T1"] != "2" {
		t.Fatal("failed transaction leaked numbers or two T1 identities collided", second)
	}
	altered := one
	altered.Tasks = append([]taskmodel.TaskBatchTask(nil), one.Tasks...)
	altered.Tasks[0].Title = "changed"
	if _, e = commands.AdmitRoadmap(ctx, "roadmap-one", altered, request, 1, "fixture"); e == nil {
		t.Fatal("same request changed body")
	}
	// Concurrent independent Roadmaps cannot obtain the same repository number.
	var wg sync.WaitGroup
	results := make(chan string, 3)
	failures := make(chan error, 3)
	for i := 0; i < 3; i++ {
		input := batch(fmt.Sprintf("parallel-%d-", i), 1)
		wg.Add(1)
		go func(i int, input taskmodel.TaskBatchInput) {
			defer wg.Done()
			receipt, e := commands.AdmitRoadmap(ctx, fmt.Sprintf("parallel-%d", i), input, fmt.Sprintf("%032x", i+10), 1, "fixture")
			if e != nil {
				failures <- e
			} else {
				results <- receipt.TaskIDs["T1"]
			}
		}(i, input)
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	seen := map[string]bool{}
	for id := range results {
		if seen[id] || id == "0" || id == "1" || id == "2" {
			t.Fatal("duplicate allocation", id)
		}
		seen[id] = true
	}
	if len(seen) != 3 {
		t.Fatal(seen)
	}
}

func TestRoadmapSourceAndMissingDocumentCapability(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	ctx := context.Background()
	reader, e := OpenNativeTaskProject(ctx, &c)
	require(t, e)
	require(t, reader.Close())
	store, e := OpenNativeTaskCommandStore(ctx, &c)
	require(t, e)
	defer store.Close()
	c.runtimeMaintenance = newProjectMaintenance(c, store)
	port := &roadmapPort{config: c, store: store}
	name := roadmap.Root + "source.md"
	full := filepath.Join(c.Repository, filepath.FromSlash(name))
	require(t, os.MkdirAll(filepath.Dir(full), 0700))
	require(t, os.WriteFile(full, []byte("# Roadmap\nT1 新任务"), 0600))
	source, e := port.source(ctx, name)
	require(t, e)
	if !source.Modified || source.Head == "" {
		t.Fatal(source)
	}
	before := fixtureGit(t, c.Repository, "status", "--porcelain")
	view, e := port.read(ctx, "/roadmaps", nil)
	require(t, e)
	body, _ := json.Marshal(view)
	if !strings.Contains(string(body), `"documents_available":false`) || !strings.Contains(string(body), "固定仓库") {
		t.Fatal(string(body))
	}
	if _, e = port.read(ctx, "/roadmaps/sources/"+source.ID, nil); e != nil {
		t.Fatal(e)
	}
	if fixtureGit(t, c.Repository, "status", "--porcelain") != before {
		t.Fatal("GET changed source worktree")
	}
	graph, e := store.TaskGraphSnapshot(ctx, c.Identity())
	require(t, e)
	if len(graph.Tasks) != 0 {
		t.Fatal("reading created tasks")
	}
}

func TestRoadmapUnknownReceiptKeepsBusinessEffectPending(t *testing.T) {
	r := roadmap.Record{State: "publishing", Submission: "started", Projection: &runnercontract.Run{StopStatus: "stopped"}, Publication: &roadmap.Publication{State: "pr_unknown"}}
	if !roadmapEffectPending("roadmap_publish", r) {
		t.Fatal("settled parse incorrectly resolved the unknown PR")
	}
	r.State = "awaiting-admission"
	if roadmapEffectPending("roadmap_publish", r) {
		t.Fatal("verified publication remained pending")
	}
	r.State, r.AdmissionRequest = "admission-unknown", strings.Repeat("c", 32)
	if !roadmapEffectPending("roadmap_admit", r) {
		t.Fatal("missing Task receipt was incorrectly resolved")
	}
	r.State = "admitted"
	if roadmapEffectPending("roadmap_admit", r) {
		t.Fatal("original Task receipt remained pending")
	}
}

func TestRoadmapPublicationRejectsAnotherInFlightGeneration(t *testing.T) {
	c, commit := nativeTaskFixture(t)
	ctx := context.Background()
	reader, err := OpenNativeTaskProject(ctx, &c)
	require(t, err)
	require(t, reader.Close())
	store, err := OpenNativeTaskCommandStore(ctx, &c)
	require(t, err)
	defer store.Close()
	a := roadmap.Analysis{Tasks: []roadmap.Node{{Key: "T1", Title: "One"}}}
	old := roadmap.Record{ID: strings.Repeat("a", 32), RequestID: strings.Repeat("a", 32), Project: c.Identity(), Revision: 1,
		Source: roadmap.Source{ID: strings.Repeat("b", 32)}, Analysis: &a, ConfirmedDigest: roadmap.ValueDigest(a), CodeCommit: commit,
		State: "publishing", DocumentsVerified: true,
		Documents: []roadmap.Documents{{Key: "T1", Spec: roadmap.Document{Path: "SteerlineFlow/spec/one.md", Body: "# Spec"},
			Plan: roadmap.Document{Path: "SteerlineFlow/plans/one.md", Body: "# Plan"}, Acceptance: []roadmap.Document{{Path: "SteerlineFlow/testing/acceptance/one.md", Body: "# 验收"}}}},
		Publication: &roadmap.Publication{RequestID: strings.Repeat("e", 32), State: "pr_unknown"}}
	require(t, store.SaveRoadmap(ctx, old, 0))
	current := old
	current.ID = strings.Repeat("c", 32)
	current.RequestID = current.ID
	current.Publication = nil
	current.State = "documents-review"
	port := &roadmapPort{config: c, store: store}
	got, err := port.publish(ctx, current, strings.Repeat("d", 32), roadmap.ReviewDigest(current), true)
	if err == nil || !strings.Contains(err.Error(), "在途") || got.Publication != nil {
		t.Fatal("another publication was accepted", got, err)
	}
}

func TestRoadmapGeneratedIndexCannotOverwriteAnotherSource(t *testing.T) {
	r := roadmap.Record{Source: roadmap.Source{ID: strings.Repeat("a", 32), Path: roadmap.Root + "one.md"}}
	name := "SteerlineFlow/Roadmap/generated/" + r.Source.ID + ".json"
	body, _ := json.Marshal(map[string]any{"roadmap_id": r.Source.ID, "source": r.Source})
	if !ownedRoadmapMetadata(name, body, r) {
		t.Fatal("same-source index update rejected")
	}
	other := r
	other.Source.Path = roadmap.Root + "two.md"
	if ownedRoadmapMetadata(name, body, other) || ownedRoadmapMetadata("SteerlineFlow/spec/unrelated.md", body, r) || ownedRoadmapMetadata(name, []byte("{}"), r) {
		t.Fatal("unrelated file treated as owned metadata")
	}
}
