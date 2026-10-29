package roadmap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/prompts"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func sampleAnalysis() model.Analysis {
	return model.Analysis{Tasks: []model.Node{
		{Key: "Alpha", KeyOrigin: "markdown", Title: "控制候选", Goal: "控制", Scope: "当前候选", Acceptance: "停止完成", Evidence: "Alpha 控制候选", Original: model.Dependencies{Local: []string{}, Formal: []string{"8"}}, Proposed: model.Dependencies{Local: []string{}, Formal: []string{"8"}}},
		{Key: "Beta", KeyOrigin: "markdown", Title: "显示状态", Goal: "显示", Scope: "状态面板", Acceptance: "归属正确", Evidence: "Beta 显示状态", Original: model.Dependencies{Local: []string{"Alpha"}, Formal: []string{}}, Proposed: model.Dependencies{Local: []string{"Alpha"}, Formal: []string{}}},
	}, Questions: []string{}, Overlap: "不与已有调度重复"}
}
func sampleSource() model.Source {
	body := "任意 Markdown 结构\nAlpha 控制候选\nBeta 显示状态\n已有正式 #8"
	return model.Source{ID: model.SourceID("p", model.Root+"one.md"), Path: model.Root + "one.md", Body: body, Digest: model.Digest([]byte(body))}
}
func TestRoadmapIdentityDependenciesAndConfirmation(t *testing.T) {
	source := sampleSource()
	graph := []model.Existing{{ID: "8", Title: "调度", Predecessors: []string{}}}
	a := sampleAnalysis()
	raw, _ := json.Marshal(a)
	result, e := model.DecodeAnalysis(raw, source, graph)
	if e != nil {
		t.Fatal(e)
	}
	r := model.Record{State: "review", Source: source, Existing: graph, GraphDigest: model.GraphDigest(graph), Analysis: &result}
	confirmed, e := model.Confirm(r, source, graph, strings.Repeat("c", 32))
	if e != nil || confirmed.State != "confirmed" || confirmed.ConfirmedDigest == "" || len(confirmed.Mapping) > 0 {
		t.Fatal(confirmed, e)
	}
	if model.SourceID("p", model.Root+"two.md") == source.ID {
		t.Fatal("two Roadmaps shared identity")
	}
	changed := source
	changed.Digest = "changed"
	if _, e = model.Confirm(r, changed, graph, "next"); e == nil {
		t.Fatal("changed source accepted")
	}
	changedGraph := append([]model.Existing(nil), graph...)
	changedGraph[0].MaterialDigest = "new"
	if _, e = model.Confirm(r, source, changedGraph, "next"); e == nil {
		t.Fatal("changed task materials accepted")
	}
	for _, mutate := range []func(*model.Analysis){
		func(a *model.Analysis) { a.Tasks[1].Key = "Alpha" },
		func(a *model.Analysis) { a.Tasks[0].Proposed.Local = []string{"Beta"} },
		func(a *model.Analysis) { a.Tasks[0].Proposed.Formal = []string{"999"} },
		func(a *model.Analysis) { a.Tasks[0].Evidence = "model invented quote" },
		func(a *model.Analysis) { a.Tasks[0].ExistingID = "8" }, // Cannot alter the existing Task's dependencies.
	} {
		bad := sampleAnalysis()
		mutate(&bad)
		body, _ := json.Marshal(bad)
		if _, e = model.DecodeAnalysis(body, source, graph); e == nil {
			t.Fatal("invalid identity/dependency/evidence accepted", string(body))
		}
	}
}
func TestRoadmapReadOnlySourceBoundaries(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, filepath.FromSlash(model.Root+"一.md"))
	if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(file, []byte("# 目标\n先讨论"), 0600); e != nil {
		t.Fatal(e)
	}
	sources, e := ListSources(root, "p")
	if e != nil || len(sources) != 1 || sources[0].Body != "# 目标\n先讨论" {
		t.Fatal(sources, e)
	}
	for _, name := range []string{"../outside.md", model.Root + "../bad.md", "C:/bad.md", model.Root + "a\\b.md"} {
		if _, e = ReadSource(root, "p", name, ""); e == nil {
			t.Fatal("unsafe source read", name)
		}
	}
	if e = os.WriteFile(file, []byte{0xff, 0xfe}, 0600); e != nil {
		t.Fatal(e)
	}
	sources, e = ListSources(root, "p")
	if e != nil || sources[0].Diagnostic == "" {
		t.Fatal("bad encoding hidden", sources, e)
	}
}

func TestRoadmapPublishedLocalIdentityCannotBeReallocated(t *testing.T) {
	a := sampleAnalysis()
	mapping := map[string]string{"Alpha": "8"}
	if model.ValidateSourceMapping(a, mapping) == nil {
		t.Fatal("published local code became a new Task")
	}
	a.Tasks[0].ExistingID = "9"
	if model.ValidateSourceMapping(a, mapping) == nil {
		t.Fatal("published local code changed its Task")
	}
	a.Tasks[0].ExistingID = "8"
	if err := model.ValidateSourceMapping(a, mapping); err != nil {
		t.Fatal(err)
	}
}

func TestRoadmapDocumentsRequireCompleteVerifiedSafeSet(t *testing.T) {
	makeRecord := func() model.Record {
		a := sampleAnalysis()
		r := model.Record{Analysis: &a, ConfirmedDigest: model.ValueDigest(a), CodeCommit: strings.Repeat("a", 40), DocumentsVerified: true}
		for _, n := range a.Tasks {
			r.Documents = append(r.Documents, model.Documents{Key: n.Key,
				Spec:       model.Document{Path: "SteerlineFlow/spec/" + n.Key + ".spec.md", Body: "# Spec"},
				Plan:       model.Document{Path: "SteerlineFlow/plans/" + n.Key + ".plan.md", Body: "# Plan"},
				Acceptance: []model.Document{{Path: "SteerlineFlow/testing/acceptance/" + n.Key + ".md", Body: "# 验收"}}})
		}
		return r
	}
	if err := model.ValidateDocuments(makeRecord()); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*model.Record){
		func(r *model.Record) { r.DocumentsVerified = false },
		func(r *model.Record) { r.ConfirmedDigest = "stale" },
		func(r *model.Record) { r.CodeCommit = strings.Repeat("x", 40) },
		func(r *model.Record) { r.Documents = r.Documents[:1] },
		func(r *model.Record) { r.Documents[0].Acceptance = nil },
		func(r *model.Record) { r.Documents[0].Spec.Path = "SteerlineFlow/spec/../outside.md" },
		func(r *model.Record) { r.Documents[1].Spec.Path = r.Documents[0].Spec.Path },
		func(r *model.Record) { r.Documents[0].Spec.Body = "" },
		func(r *model.Record) { r.Documents[0].Spec.Body = string([]byte{0xff}) },
		func(r *model.Record) { r.Documents[0].Spec.Path = "SteerlineFlow/spec/a.exe" },
	} {
		r := makeRecord()
		mutate(&r)
		if model.ValidateDocuments(r) == nil {
			t.Fatal("unsafe or incomplete document set accepted", r)
		}
	}
}

type memoryRoadmaps struct {
	rows map[string]model.Stored
	deny bool
}

func (m *memoryRoadmaps) Roadmap(_ context.Context, p, id string) (model.Record, error) {
	r, ok := m.rows[id]
	if !ok || r.Project != p {
		return model.Record{}, model.ErrNotFound
	}
	b, _ := json.Marshal(r)
	var clone model.Stored
	_ = json.Unmarshal(b, &clone)
	return clone.Value(), nil
}
func (m *memoryRoadmaps) Roadmaps(ctx context.Context, p string) ([]model.Record, error) {
	out := []model.Record{}
	for id := range m.rows {
		r, e := m.Roadmap(ctx, p, id)
		if e == nil {
			out = append(out, r)
		}
	}
	return out, nil
}
func (m *memoryRoadmaps) RoadmapRequest(ctx context.Context, p, id string) (model.Record, error) {
	for _, s := range m.rows {
		r := s.Value()
		if r.RequestID == id || r.StopID == id || r.ConfirmationRequest == id {
			return m.Roadmap(ctx, p, r.ID)
		}
	}
	return model.Record{}, model.ErrNotFound
}
func (m *memoryRoadmaps) RoadmapMapping(context.Context, string, string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (m *memoryRoadmaps) SaveRoadmap(_ context.Context, r model.Record, expected uint64) error {
	if m.deny {
		return errors.New("storage denied")
	}
	if m.rows[r.ID].Revision != expected {
		return errors.New("stale_revision")
	}
	b, _ := json.Marshal(model.StoreValue(r))
	var clone model.Stored
	_ = json.Unmarshal(b, &clone)
	m.rows[r.ID] = clone
	return nil
}

type roadmapRunner struct {
	request                  runnercontract.Request
	run                      runnercontract.Run
	submits, stops, inspects int
	lost, hidden             bool
	files                    map[string][]byte
}

type racingRoadmapStop struct {
	*roadmapRunner
	unknown  bool
	requests []runnercontract.Request
}

func (f *racingRoadmapStop) Call(ctx context.Context, command string, args []string, r *runnercontract.Request) (runnerclient.Response, error) {
	f.requests = append(f.requests, *r)
	if len(f.requests) == 1 {
		f.run.Revision++
		return runnerclient.Response{}, &runnerclient.Failure{Code: "stale_revision", Unknown: f.unknown}
	}
	var input struct {
		Revision uint64 `json:"expected_revision"`
	}
	_ = json.Unmarshal(r.Input, &input)
	if input.Revision != f.run.Revision || r.RequestID == f.requests[0].RequestID {
		return runnerclient.Response{}, errors.New("unobserved replacement stop")
	}
	f.run.StopStatus = "stopped"
	return f.roadmapRunner.Call(ctx, command, args, r)
}
func TestRoadmapCloseReconcilesDefinitiveStopRevisionRace(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprint(unknown), func(t *testing.T) {
			s, store, original, in := generationSetup(t)
			f := &racingRoadmapStop{roadmapRunner: original, unknown: unknown}
			s.Runner = f
			if _, err := s.Parse(context.Background(), in); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			err := s.Close(ctx)
			r, _ := store.Roadmap(context.Background(), s.Project, in.RequestID)
			if unknown {
				if err == nil || len(f.requests) != 1 || r.StopRequest == nil || r.StopRequest.RequestID != f.requests[0].RequestID {
					t.Fatal("uncertain stop was replaced or declared settled", err, r, f.requests)
				}
			} else if err != nil || len(f.requests) != 2 || !r.Settled() {
				t.Fatal("rejected stop did not settle before close", err, r, f.requests)
			}
		})
	}
}

func (f *roadmapRunner) Submit(_ context.Context, r runnercontract.Request) (runnerclient.Receipt, error) {
	f.submits++
	f.request = r
	var in runnercontract.GenerateRunInput
	_ = json.Unmarshal(r.Input, &in)
	f.run = runnercontract.Run{RunID: "original-run", Revision: 1, Workflow: in.Workflow, Correlation: in.Correlation, InputDigest: "original-input"}
	if in.Generate.Repository != nil {
		source := &runnercontract.RepositorySource{LocalPath: in.Generate.Repository.LocalPath, Commit: in.Generate.Repository.Commit, Tree: strings.Repeat("b", 40)}
		for path, body := range f.files {
			source.Files = append(source.Files, runnercontract.RepositoryFile{Path: path, Blob: strings.Repeat("c", 40), SHA256: model.Digest(body), SizeBytes: len(body)})
		}
		f.run.RepositorySource = source
	}
	if f.lost {
		return runnerclient.Receipt{}, &runnerclient.Failure{Unknown: true}
	}
	return runnerclient.Receipt{RunID: f.run.RunID, CommandID: "submit", ReceiptStatus: "accepted"}, nil
}
func (f *roadmapRunner) Inspect(_ context.Context, selector, id string, _ bool) (runnerclient.Inspection, error) {
	f.inspects++
	if f.hidden {
		return runnerclient.Inspection{}, nil
	}
	if selector == "request-id" && id != f.request.RequestID {
		return runnerclient.Inspection{}, errors.New("replacement request observed")
	}
	r := f.run
	if r.ContentResult != nil && r.ContentResult.RepositorySource == nil {
		copy := *r.ContentResult
		copy.RepositorySource = r.RepositorySource
		r.ContentResult = &copy
	}
	return runnerclient.Inspection{Run: &r, Command: &runnerclient.Command{Command: "submit", RequestID: f.request.RequestID, RunID: &r.RunID}}, nil
}
func (f *roadmapRunner) Call(_ context.Context, command string, args []string, _ *runnercontract.Request) (runnerclient.Response, error) {
	if command != "stop" || len(args) != 2 || args[1] != f.run.RunID {
		return runnerclient.Response{}, errors.New("wrong stop")
	}
	f.stops++
	b, _ := json.Marshal(runnerclient.Receipt{RunID: f.run.RunID, CommandID: "stop", ReceiptStatus: "accepted"})
	return runnerclient.Response{Data: b}, nil
}
func generationSetup(t *testing.T) (*Service, *memoryRoadmaps, *roadmapRunner, ParseInput) {
	t.Helper()
	b, e := os.ReadFile("../runnerclient/testdata/content-capabilities.json")
	if e != nil {
		t.Fatal(e)
	}
	var caps runnerclient.Capabilities
	if e = json.Unmarshal(b, &caps); e != nil {
		t.Fatal(e)
	}
	catalog, e := runnerclient.CompileCatalog(caps)
	if e != nil {
		t.Fatal(e)
	}
	catalog = repositoryCatalog(t, catalog)
	store := &memoryRoadmaps{rows: map[string]model.Stored{}}
	runner := &roadmapRunner{}
	source := sampleSource()
	source.Head = strings.Repeat("a", 40)
	graph, _ := json.Marshal([]model.Existing{{ID: "8", Title: "调度", Predecessors: []string{}}})
	rules, e := prompts.RoadmapParseRules()
	if e != nil {
		t.Fatal(e)
	}
	files := map[string][]byte{source.Path: []byte(source.Body), GraphPath: graph, MappingPath: []byte(`{}`), ParseRulesPath: rules}
	runner.files = files
	s := &Service{Project: "p", Repository: t.TempDir(), ReadCommitted: func(_ context.Context, commit, path string) ([]byte, error) {
		body, ok := files[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return append([]byte(nil), body...), nil
	}, Store: store, Runner: runner, Source: func(context.Context, string) (model.Source, error) { return source, nil }, Graph: func(context.Context) ([]model.Existing, error) {
		return []model.Existing{{ID: "8", Title: "调度", Predecessors: []string{}}}, nil
	}, Prepare: func(context.Context, string) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
		return runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"read_only","model":"test-model"}`)}, catalog, nil
	}}
	return s, store, runner, ParseInput{RequestID: strings.Repeat("a", 32), Path: source.Path, Digest: source.Digest, Profile: "local"}
}

func TestDocumentDrivenRoadmapMaterials(t *testing.T) {
	for _, mode := range []string{"ready", "missing", "wrong-original", "wrong-graph", "wrong-mapping", "wrong-rules", "unreadable"} {
		t.Run(mode, func(t *testing.T) {
			s, store, peer, in := generationSetup(t)
			switch mode {
			case "missing":
				delete(peer.files, ParseRulesPath)
			case "wrong-original":
				peer.files[in.Path] = []byte("same path, different original")
			case "wrong-graph":
				peer.files[GraphPath] = []byte(`[]`)
			case "wrong-mapping":
				peer.files[MappingPath] = []byte(`{"Alpha":"99"}`)
			case "wrong-rules":
				peer.files[ParseRulesPath] = []byte("changed rules")
			case "unreadable":
				s.ReadCommitted = func(context.Context, string, string) ([]byte, error) { return nil, os.ErrPermission }
			}
			_, err := s.Parse(context.Background(), in)
			if mode != "ready" {
				if err == nil || peer.submits != 0 || len(store.rows) != 0 {
					t.Fatal("unprepared materials submitted", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var request runnercontract.GenerateRunInput
			if err = json.Unmarshal(peer.request.Input, &request); err != nil {
				t.Fatal(err)
			}
			if request.Generate.Repository == nil || len(request.Generate.Materials) != 0 {
				t.Fatal("repository materials missing")
			}
			for _, line := range strings.Split(sampleSource().Body, "\n") {
				if strings.Contains(string(peer.request.Input), line) {
					t.Fatal("source hidden in instruction/schema", line)
				}
			}
		})
	}
}
func TestRoadmapGenerateDurableRequestUnknownAndConfirm(t *testing.T) {
	s, store, f, in := generationSetup(t)
	ctx := context.Background()
	store.deny = true
	if _, e := s.Parse(ctx, in); e == nil || f.submits != 0 {
		t.Fatal("submission preceded persistence")
	}
	store.deny = false
	f.lost = true
	r, e := s.Parse(ctx, in)
	if e == nil || r.State != "unknown" || f.submits != 1 {
		t.Fatal(r, e)
	}
	if _, e = s.Parse(ctx, in); e != nil || f.submits != 1 {
		t.Fatal("unknown request resubmitted", e)
	}
	a := sampleAnalysis()
	raw, _ := json.Marshal(a)
	f.run.ContentResult = &runnercontract.ContentResult{Status: "succeeded", JSON: raw, Source: runnercontract.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "result", TurnID: "turn"}}
	f.run.StopStatus = "stopped"
	f.run.Revision++
	r, e = s.Observe(ctx, r.ID)
	if e != nil || r.State != "review" {
		t.Fatal(r, e)
	}
	r, e = s.Confirm(ctx, r.ID, strings.Repeat("b", 32), r.Revision)
	if e != nil || r.State != "confirmed" || len(r.Mapping) != 0 || f.submits != 1 {
		t.Fatal(r, e)
	}
	before := f.inspects
	if e = s.ObserveNext(ctx); e != nil || f.inspects != before {
		t.Fatal("settled content kept polling", e)
	}
}
func TestRoadmapCancelledUnknownOnlyStopsOriginal(t *testing.T) {
	s, _, f, in := generationSetup(t)
	ctx := context.Background()
	f.lost = true
	r, _ := s.Parse(ctx, in)
	r, e := s.Stop(ctx, r.ID, strings.Repeat("b", 32))
	if e != nil {
		t.Fatal(e)
	}
	f.hidden = true
	if _, e = s.Observe(ctx, r.ID); e != nil || f.submits != 1 || f.stops != 0 {
		t.Fatal(e)
	}
	f.hidden = false
	r, e = s.Observe(ctx, r.ID)
	if e != nil || f.stops != 1 || !r.Cancelled {
		t.Fatal(r, e)
	}
	f.run.StopStatus = "stopped"
	f.run.Revision++
	if _, e = s.Observe(ctx, r.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Observe(ctx, r.ID); e != nil || f.stops != 1 || f.submits != 1 {
		t.Fatal("duplicate input after stop", e)
	}
}
