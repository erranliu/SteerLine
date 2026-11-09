package mainagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/mainagent/model"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestMaterialAnalysisSlowStartDoesNotBlockActorObservation(t *testing.T) {
	s, _, _, input := setup(t)
	original := s.Context
	entered, release := make(chan struct{}), make(chan struct{})
	s.Context = func(ctx context.Context, in model.Start) (model.Context, error) {
		close(entered)
		<-release
		return original(ctx, in)
	}
	done := make(chan error, 1)
	go func() { _, err := s.Start(context.Background(), strings.Repeat("a", 32), input); done <- err }()
	<-entered
	defer func() {
		close(release)
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	observed := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	go func() { observed <- s.ObserveNext(ctx) }()
	select {
	case err := <-observed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("actor observation waited for the user request's material fetch")
	}
}

type memory struct{ rows map[string]model.Record }

func (m *memory) Analysis(_ context.Context, p, id string) (model.Record, error) {
	r, ok := m.rows[id]
	if !ok || r.Project != p {
		return r, model.ErrNotFound
	}
	b, _ := json.Marshal(r)
	var copy model.Record
	_ = json.Unmarshal(b, &copy)
	return copy, nil
}
func (m *memory) Analyses(ctx context.Context, p string) ([]model.Record, error) {
	out := []model.Record{}
	for id := range m.rows {
		r, _ := m.Analysis(ctx, p, id)
		if r.Project == p {
			out = append(out, r)
		}
	}
	return out, nil
}
func (m *memory) SaveAnalysis(_ context.Context, r model.Record, expected uint64) error {
	if m.rows[r.ID].Revision != expected {
		return errors.New("stale_revision")
	}
	b, _ := json.Marshal(r)
	var copy model.Record
	_ = json.Unmarshal(b, &copy)
	m.rows[r.ID] = copy
	return nil
}

type fakeRunner struct {
	submits, stops           int
	request                  runnercontract.Request
	run                      runnercontract.Run
	files                    map[string][]byte
	rejected, lost, stopLost bool
}

func (f *fakeRunner) Submit(_ context.Context, q runnercontract.Request) (runnerclient.Receipt, error) {
	f.submits++
	f.request = q
	if f.rejected {
		return runnerclient.Receipt{}, &runnerclient.Failure{Code: "invalid_input", Unknown: false}
	}
	var in runnercontract.GenerateRunInput
	_ = json.Unmarshal(q.Input, &in)
	f.run = runnercontract.Run{RunID: "analysis-run", Revision: 1, Correlation: in.Correlation, Workflow: in.Workflow, InputDigest: model.Digest(q.Input), Status: "running", Execution: runnercontract.Execution{AttemptID: "attempt", SessionID: "session", TurnID: "turn"}}
	f.run.RepositorySource = &runnercontract.RepositorySource{LocalPath: in.Generate.Repository.LocalPath, Commit: in.Generate.Repository.Commit, Tree: strings.Repeat("b", 40)}
	for _, path := range in.Generate.Repository.Paths {
		body := f.files[path]
		f.run.RepositorySource.Files = append(f.run.RepositorySource.Files, runnercontract.RepositoryFile{Path: path, Blob: strings.Repeat("c", 40), SHA256: model.Digest(body), SizeBytes: len(body)})
	}
	for _, d := range in.Generate.Materials {
		f.run.Materials = append(f.run.Materials, runnercontract.Material{Kind: "file", Path: d.Path, Digest: model.Digest([]byte(*d.Content)), SnapshotPath: filepath.Join(os.TempDir(), "private", d.Path), ExecutionLocation: filepath.Join(os.TempDir(), "private", d.Path)})
	}
	if f.lost {
		return runnerclient.Receipt{}, &runnerclient.Failure{Code: "transport_timeout", Unknown: true}
	}
	return runnerclient.Receipt{ReceiptStatus: "accepted", RunID: f.run.RunID, CommandID: "submit"}, nil
}
func (f *fakeRunner) Inspect(_ context.Context, selector, id string) (runnerclient.Inspection, error) {
	runID := f.run.RunID
	return runnerclient.Inspection{Run: &f.run, Command: &runnercontract.Command{Command: "submit", RequestID: f.request.RequestID, RunID: &runID}}, nil
}
func (f *fakeRunner) Call(_ context.Context, command string, args []string, q *runnercontract.Request) (runnerclient.Response, error) {
	f.stops++
	if f.stopLost {
		return runnerclient.Response{}, &runnerclient.Failure{Unknown: true, Code: "timeout"}
	}
	f.run.StopStatus = "stopped"
	raw, _ := json.Marshal(runnerclient.Receipt{ReceiptStatus: "accepted", RunID: f.run.RunID, CommandID: "stop"})
	return runnerclient.Response{Data: raw}, nil
}
func (f *fakeRunner) complete(result model.Result) {
	body, _ := json.Marshal(result)
	source := runnercontract.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "analysis-command", AttemptID: "attempt", SessionID: "session", TurnID: "turn"}
	f.run.Final = &source
	f.run.Status, f.run.StopStatus = "completed", "stopped"
	f.run.ContentResult = &runnercontract.ContentResult{Status: "succeeded", JSON: body, Source: source, RepositorySource: f.run.RepositorySource, FileMaterials: f.run.Materials}
}

func setup(t *testing.T) (*Service, *memory, *fakeRunner, model.Start) {
	t.Helper()
	raw, err := os.ReadFile("../runnerclient/testdata/content-capabilities.json")
	if err != nil {
		t.Fatal(err)
	}
	var caps runnerclient.Capabilities
	if err = json.Unmarshal(raw, &caps); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile("../runnerclient/testdata/generate-repository-capability.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Generate *runnerclient.GenerateCapability `json:"generate"`
		Schema   json.RawMessage                  `json:"generate_input"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	caps.Generate = fixture.Generate
	caps.ConfigurationSchemas.GenerateInput = fixture.Schema
	cat, err := runnerclient.CompileCatalog(caps)
	if err != nil {
		t.Fatal(err)
	}
	rules, _ := prompts.MainAgentRules()
	doc := model.Document{Path: "task.spec.md", Commit: strings.Repeat("a", 40), Body: "Empty input must return an error.", SHA256: model.Digest([]byte("Empty input must return an error."))}
	m := &memory{rows: map[string]model.Record{}}
	f := &fakeRunner{files: map[string][]byte{model.RulesPath: rules, doc.Path: []byte(doc.Body)}}
	s := &Service{Project: "p", Repository: t.TempDir(), Store: m, Runner: f, Build: runnercontract.BuildPin{StateDir: t.TempDir()}, Read: func(_ context.Context, commit, path string) ([]byte, error) {
		body, ok := f.files[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return body, nil
	}, Context: func(_ context.Context, in model.Start) (model.Context, error) {
		return model.Context{CandidateID: in.CandidateID, CandidateRevision: in.CandidateRevision, RepositoryCommit: doc.Commit, OriginalBase: doc.Commit, Original: []model.Document{doc}, Current: []model.Document{doc}, SpecPath: doc.Path, PlanPath: "task.plan.md", Question: &runnercontract.Question{ID: "question", AllowFreeText: true}}, nil
	}, Prepare: func(context.Context) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
		return runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"model":"test-model","access_mode":"read_only"}`)}, cat, nil
	}}
	return s, m, f, model.Start{CandidateID: strings.Repeat("b", 32), CandidateRevision: 7, Kind: "clarification", Note: "What should empty input do?"}
}
func validResult() model.Result {
	return model.Result{Disposition: "clarification", Summary: "Existing requirement applies", Recommendation: "Return the specified error and verify empty input.", Questions: []string{}, Changes: []model.Change{}, Evidence: []model.Evidence{{Path: "task.spec.md", Commit: strings.Repeat("a", 40), Quote: "Empty input must return an error."}}, Answer: &model.Answer{Note: "Return an error.", Answers: map[string][]string{}}}
}
func TestMaterialAnalysisFrozenInputAndFollowUp(t *testing.T) {
	s, _, f, in := setup(t)
	ctx := context.Background()
	id := strings.Repeat("d", 32)
	r, err := s.Start(ctx, id, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Start(ctx, id, in); err != nil || f.submits != 1 {
		t.Fatal("replayed analysis submitted again", err)
	}
	changed := in
	changed.Note = "changed"
	if _, err = s.Start(ctx, id, changed); err == nil {
		t.Fatal("same ID accepted changed report")
	}
	var q runnercontract.GenerateRunInput
	_ = json.Unmarshal(r.Request.Input, &q)
	if q.Generate.Materials[0].Kind != "file" || strings.Contains(q.Generate.Instruction, in.Note) || !strings.Contains(q.Generate.Instruction, model.RulesPath) {
		t.Fatal("context was inlined or rules missing")
	}
	f.complete(validResult())
	r, err = s.Observe(ctx, id)
	if err != nil || r.State != "review" || r.Result == nil {
		t.Fatal(r.Diagnostic, err)
	}
	in.ParentID = id
	in.Note = "Keep that decision; clarify the message."
	next, err := s.Start(ctx, strings.Repeat("e", 32), in)
	if err != nil || !strings.Contains(string(next.Context.Prior), "Return an error.") {
		t.Fatal("follow-up lost original decision", err)
	}
}
func TestMaterialAnalysisRejectsWrongEvidenceAndSource(t *testing.T) {
	for _, name := range []string{"session", "turn", "command", "file_manifest", "repository_manifest", "quote", "version", "other_file", "no_question", "out_of_scope"} {
		t.Run(name, func(t *testing.T) {
			s, _, f, in := setup(t)
			r, err := s.Start(context.Background(), strings.Repeat("d", 32), in)
			if err != nil {
				t.Fatal(err)
			}
			result := validResult()
			switch name {
			case "quote":
				result.Evidence[0].Quote = "invented"
			case "version":
				result.Evidence[0].Commit = strings.Repeat("0", 40)
			case "other_file":
				result.Disposition = "revision"
				result.Answer = nil
				result.Changes = []model.Change{{Path: "code.go", Body: "malicious", Reason: "change", BeforeSHA256: "none"}}
			case "no_question":
				r.Context.Question = nil
				s.Store.(*memory).rows[r.ID] = r
			case "out_of_scope":
				result.Disposition = "out_of_scope"
			}
			f.complete(result)
			switch name {
			case "session":
				f.run.ContentResult.Source.SessionID = "foreign"
			case "turn":
				f.run.ContentResult.Source.TurnID = "foreign"
			case "command":
				f.run.ContentResult.Source.CommandID = "foreign"
			case "file_manifest":
				f.run.ContentResult.FileMaterials = append([]runnercontract.Material{}, f.run.Materials...)
				f.run.ContentResult.FileMaterials[0].Digest = "wrong"
			case "repository_manifest":
				copy := *f.run.RepositorySource
				copy.Commit = strings.Repeat("f", 40)
				f.run.ContentResult.RepositorySource = &copy
			}
			got, err := s.Observe(context.Background(), r.ID)
			if err != nil || got.State != "failed" || got.Result != nil {
				t.Fatal("unverified result accepted", name, got, err)
			}
		})
	}
}
func TestMaterialAnalysisUnknownNeverResubmits(t *testing.T) {
	s, _, f, in := setup(t)
	f.lost = true
	r, err := s.Start(context.Background(), strings.Repeat("d", 32), in)
	if err != nil || r.State != "unknown" {
		t.Fatal(r, err)
	}
	for range 3 {
		_, err = s.Observe(context.Background(), r.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.submits != 1 {
		t.Fatal("unknown submit repeated")
	}
	f.stopLost = true
	if _, err = s.Stop(context.Background(), r.ID); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		_, err = s.Observe(context.Background(), r.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.stops != 1 {
		t.Fatal("unknown stop replaced or repeated")
	}
}
func TestMaterialAnalysisMissingRulesMakesNoSubmit(t *testing.T) {
	s, _, f, in := setup(t)
	delete(f.files, model.RulesPath)
	if _, err := s.Start(context.Background(), strings.Repeat("d", 32), in); err == nil || f.submits != 0 {
		t.Fatal("missing rules fell back or submitted", err)
	}
}
