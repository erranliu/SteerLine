package testmanagement

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type memoryGenerations struct {
	rows      map[string]Generation
	fail      bool
	failSaved bool
}

func (m *memoryGenerations) Generation(_ context.Context, p, id string) (Generation, error) {
	r, ok := m.rows[p+id]
	if !ok {
		return r, ErrNotFound
	}
	raw, _ := json.Marshal(r)
	_ = json.Unmarshal(raw, &r)
	return r, nil
}
func (m *memoryGenerations) Generations(ctx context.Context, p string) ([]Generation, error) {
	r := []Generation{}
	for _, v := range m.rows {
		if v.Project == p {
			copy, _ := m.Generation(ctx, p, v.ID)
			r = append(r, copy)
		}
	}
	return r, nil
}
func (m *memoryGenerations) SaveGeneration(_ context.Context, r Generation, expected uint64) error {
	if m.fail {
		return errors.New("persist denied")
	}
	if m.failSaved && r.State == "saved" {
		return errors.New("saved receipt lost")
	}
	old := m.rows[r.Project+r.ID]
	if old.Revision != expected {
		return errors.New("stale revision")
	}
	raw, _ := json.Marshal(r)
	var copy Generation
	_ = json.Unmarshal(raw, &copy)
	m.rows[r.Project+r.ID] = copy
	return nil
}

type generationCLI struct {
	submits, stops int
	lost           bool
	hidden         bool
	request        runnerclient.Request
	run            runnerclient.Run
}

func (f *generationCLI) Submit(_ context.Context, r runnerclient.Request) (runnerclient.Receipt, error) {
	f.submits++
	f.request = r
	var input runnercontract.GenerateRunInput
	_ = json.Unmarshal(r.Input, &input)
	f.run = runnerclient.Run{RunID: "original-run", Revision: 1, Workflow: input.Workflow, Correlation: input.Correlation, InputDigest: "original-input"}
	if f.lost {
		return runnerclient.Receipt{}, &runnerclient.Failure{Unknown: true, Code: "request_outcome_unknown"}
	}
	return runnerclient.Receipt{RunID: f.run.RunID, CommandID: "submit-command", ReceiptStatus: "accepted"}, nil
}
func (f *generationCLI) Inspect(_ context.Context, selector, id string, _ bool) (runnerclient.Inspection, error) {
	if f.hidden {
		return runnerclient.Inspection{}, nil
	}
	if f.submits == 0 {
		return runnerclient.Inspection{}, ErrNotFound
	}
	copy := f.run
	if selector == "request-id" && id != f.request.RequestID {
		return runnerclient.Inspection{}, errors.New("wrong original request")
	}
	return runnerclient.Inspection{Run: &copy, Command: &runnerclient.Command{Command: "submit", RequestID: f.request.RequestID, CommandID: "submit-command", RunID: &copy.RunID}}, nil
}
func (f *generationCLI) Call(_ context.Context, command string, args []string, r *runnerclient.Request) (runnerclient.Response, error) {
	if command != "stop" || len(args) != 2 || args[1] != f.run.RunID {
		return runnerclient.Response{}, errors.New("wrong stop target")
	}
	var input map[string]json.RawMessage
	_ = json.Unmarshal(r.Input, &input)
	if len(input) != 1 || input["expected_revision"] == nil {
		return runnerclient.Response{}, errors.New("wrong stop payload")
	}
	f.stops++
	raw, _ := json.Marshal(runnerclient.Receipt{RunID: f.run.RunID, CommandID: "stop-command", ReceiptStatus: "accepted"})
	return runnerclient.Response{Data: raw}, nil
}
func (f *generationCLI) finish(result string) {
	f.run.Revision++
	f.run.StopStatus = "stopped"
	f.run.ContentResult = &runnercontract.ContentResult{Status: "succeeded", JSON: json.RawMessage(result), Source: runnercontract.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "command", TurnID: "turn"}}
}
func generationFixture(t *testing.T) (*GenerationService, *memoryGenerations, *generationCLI, GenerateCommand) {
	t.Helper()
	root := t.TempDir()
	for p, text := range map[string]string{"docs/spec/task.plan.md": "# Plan\n验证规则。", RulesPath: "# 测试规范\n最小验证。"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := json.Marshal(Catalogue{Tests: []Definition{definition()}})
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, CataloguePath)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, CataloguePath), raw, 0600); err != nil {
		t.Fatal(err)
	}
	capsRaw, err := os.ReadFile("../runnerclient/testdata/content-capabilities.json")
	if err != nil {
		t.Fatal(err)
	}
	var caps runnerclient.Capabilities
	if err = json.Unmarshal(capsRaw, &caps); err != nil {
		t.Fatal(err)
	}
	catalog, err := runnerclient.CompileCatalog(caps)
	if err != nil {
		t.Fatal(err)
	}
	m := &memoryGenerations{rows: map[string]Generation{}}
	f := &generationCLI{}
	s := &GenerationService{Project: "project", Repository: root, Store: m, Runner: f, Prepare: func(context.Context, string) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
		return runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"read_only","model":"test-model"}`)}, catalog, nil
	}}
	return s, m, f, GenerateCommand{RequestID: strings.Repeat("a", 32), Plan: "docs/spec/task.plan.md", Profile: "local"}
}
func TestTMGenerationSaveAndReplay(t *testing.T) {
	s, _, f, in := generationFixture(t)
	ctx := context.Background()
	r, err := s.Generate(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if r.State != "accepted" || f.submits != 1 {
		t.Fatal(r)
	}
	f.finish(`{"tests":["unit.rules"]}`)
	if err = s.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Store.Generation(ctx, s.Project, in.RequestID)
	if r.State != "saved" {
		t.Fatalf("not saved %+v", r)
	}
	raw, err := os.ReadFile(filepath.Join(s.Repository, r.Target))
	if err != nil || !strings.Contains(string(raw), "unit.rules") {
		t.Fatal(string(raw), err)
	}
	if _, err = s.Generate(ctx, in); err != nil || f.submits != 1 {
		t.Fatal("replayed model", err)
	}
	in.Plan = "different.plan.md"
	if _, err = s.Generate(ctx, in); err == nil {
		t.Fatal("request input changed")
	}
}
func TestTMGenerationFailureAndEmptyKeepFile(t *testing.T) {
	for _, value := range []string{`{"tests":[]}`, `{"tests":["unknown"]}`, `{"tests":["unit.rules","unit.rules"]}`} {
		t.Run(value, func(t *testing.T) {
			s, _, f, in := generationFixture(t)
			target, _ := ArrangementPath(in.Plan)
			old := []byte("preserve original")
			_ = os.WriteFile(filepath.Join(s.Repository, target), old, 0600)
			if _, err := s.Generate(context.Background(), in); err != nil {
				t.Fatal(err)
			}
			f.finish(value)
			_ = s.Observe(context.Background())
			got, _ := os.ReadFile(filepath.Join(s.Repository, target))
			if string(got) != string(old) {
				t.Fatal("overwrote original")
			}
		})
	}
}
func TestTMGenerationPersistenceAndUnknown(t *testing.T) {
	s, m, f, in := generationFixture(t)
	ctx := context.Background()
	m.fail = true
	if _, err := s.Generate(ctx, in); err == nil || f.submits != 0 {
		t.Fatal("submitted before persistence")
	}
	m.fail = false
	f.lost = true
	if _, err := s.Generate(ctx, in); err == nil {
		t.Fatal("expected unknown")
	}
	_ = s.Observe(ctx)
	if f.submits != 1 {
		t.Fatal("automatic resubmit")
	}
	if _, err := s.Stop(ctx, in.RequestID, strings.Repeat("b", 32)); err != nil {
		t.Fatal(err)
	}
	_ = s.Observe(ctx)
	if f.stops != 1 || f.submits != 1 {
		t.Fatalf("duplicated action %+v", f)
	}
}
func TestTMGenerationChangesAndRestart(t *testing.T) {
	s, m, f, in := generationFixture(t)
	ctx := context.Background()
	if _, err := s.Generate(ctx, in); err != nil {
		t.Fatal(err)
	}
	s.allowSave = nil
	f.finish(`{"tests":["unit.rules"]}`)
	_ = s.Observe(ctx)
	r, _ := m.Generation(ctx, s.Project, in.RequestID)
	if r.State != "generated" {
		t.Fatal("restart wrote file", r.State)
	}
	if _, err := os.Stat(filepath.Join(s.Repository, r.Target)); !os.IsNotExist(err) {
		t.Fatal("restart saved without intent")
	}
	if _, err := s.Generate(ctx, in); err != nil {
		t.Fatal(err)
	}
	r, _ = m.Generation(ctx, s.Project, in.RequestID)
	if r.State != "saved" || f.submits != 1 {
		t.Fatal("explicit completion failed", r.State)
	}
	s2, _, f2, in2 := generationFixture(t)
	_, _ = s2.Generate(ctx, in2)
	_ = os.WriteFile(filepath.Join(s2.Repository, in2.Plan), []byte("# changed Plan"), 0600)
	f2.finish(`{"tests":["unit.rules"]}`)
	_ = s2.Observe(ctx)
	r, _ = s2.Store.Generation(ctx, s2.Project, in2.RequestID)
	if r.State != "stale" {
		t.Fatal("changed inputs accepted", r.State)
	}
}

func TestTMGenerationCancelledBeforeLateBindingDoesNotSave(t *testing.T) {
	s, _, f, in := generationFixture(t)
	f.lost, f.hidden = true, true
	ctx := context.Background()
	_, _ = s.Generate(ctx, in)
	if _, err := s.Stop(ctx, in.RequestID, strings.Repeat("b", 32)); err != nil {
		t.Fatal(err)
	}
	if f.stops != 0 {
		t.Fatal("unbound stop sent to a different Run")
	}
	f.hidden = false
	if err := s.Observe(ctx); err != nil || f.stops != 1 {
		t.Fatal("late binding lost original stop", err)
	}
	f.finish(`{"tests":["unit.rules"]}`)
	if err := s.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	target, _ := ArrangementPath(in.Plan)
	if _, err := os.Stat(filepath.Join(s.Repository, target)); !os.IsNotExist(err) {
		t.Fatal("cancelled generation wrote its late result")
	}
	if err := s.Close(ctx); err != nil || f.stops != 1 || f.submits != 1 {
		t.Fatal("closure resent an original action", err)
	}
}

func TestTMGenerationSavedFileSurvivesLostReceipt(t *testing.T) {
	s, m, f, in := generationFixture(t)
	ctx := context.Background()
	if _, err := s.Generate(ctx, in); err != nil {
		t.Fatal(err)
	}
	m.failSaved = true
	f.finish(`{"tests":["unit.rules"]}`)
	_ = s.Observe(ctx)
	target, _ := ArrangementPath(in.Plan)
	before, err := os.ReadFile(filepath.Join(s.Repository, target))
	if err != nil {
		t.Fatal("file was not saved before its lost receipt", err)
	}
	m.failSaved = false
	r, err := s.Generate(ctx, in)
	after, _ := os.ReadFile(filepath.Join(s.Repository, target))
	if err != nil || r.State != "saved" || f.submits != 1 || string(before) != string(after) {
		t.Fatal("lost save receipt caused regeneration or replacement", r.State, err)
	}
}

func TestTMGenerationCompatibleBuildObservesOriginalWithoutResubmit(t *testing.T) {
	s, m, f, in := generationFixture(t)
	s.Build = runnercontract.BuildPin{Version: "old"}
	original, err := s.Generate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	f.finish(`{"tests":["unit.rules"]}`)
	s.Build = runnercontract.BuildPin{Version: "new"}
	if err = s.Observe(context.Background()); err != nil {
		t.Fatal(err)
	}
	blocked, _ := m.Generation(context.Background(), s.Project, in.RequestID)
	if blocked.Diagnostic == "" || blocked.State == "saved" {
		t.Fatal("different build accepted without proof", blocked)
	}
	checks := 0
	s.VerifyBinding = func(g Generation) error {
		checks++
		if g.Runner != original.Runner || g.RunID != original.RunID || g.InputDigest != original.InputDigest {
			return errors.New("changed original")
		}
		return nil
	}
	if err = s.Observe(context.Background()); err != nil {
		t.Fatal(err)
	}
	saved, _ := m.Generation(context.Background(), s.Project, in.RequestID)
	if checks == 0 || f.submits != 1 || saved.State != "saved" || saved.Runner != original.Runner || saved.RunID != original.RunID {
		t.Fatal(saved, f.submits, checks)
	}
}
