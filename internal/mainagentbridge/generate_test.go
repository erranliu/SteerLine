package mainagentbridge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/mainagent/contract"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type fakeRunner struct {
	request                  runnercontract.Request
	run                      runnercontract.Run
	submits, inspects, stops int
	submitError              error
	onSubmit                 func(context.Context)
	keepRunning              bool
}

func (f *fakeRunner) Submit(ctx context.Context, r runnercontract.Request) (runnerclient.Receipt, error) {
	f.request = r
	f.submits++
	var in runnercontract.GenerateRunInput
	_ = json.Unmarshal(r.Input, &in)
	f.run = runnercontract.Run{RunID: "original-run", Revision: 1, InputDigest: "original-digest", Workflow: in.Workflow, Correlation: in.Correlation, Status: "completed", StopStatus: "stopped", Execution: runnercontract.Execution{AttemptID: "attempt", SessionID: "session", TurnID: "turn"}, ContentResult: &runnercontract.ContentResult{Status: "succeeded", Text: "请依据提供的材料继续讨论，明确尚待确认的要求。", Source: runnercontract.Final{Verified: true, NativeOutcome: "succeeded", Source: "provider.final", ItemID: "item", CommandID: "command", AttemptID: "attempt", SessionID: "session", TurnID: "turn"}}}
	final := f.run.ContentResult.Source
	final.Text = f.run.ContentResult.Text
	f.run.ContentResult.Source = final
	f.run.Final = &final
	if f.keepRunning {
		f.run.Status, f.run.StopStatus = "running", "running"
	}
	if f.onSubmit != nil {
		f.onSubmit(ctx)
	}
	return runnerclient.Receipt{RunID: f.run.RunID, CommandID: "submit-command", ReceiptStatus: "accepted"}, f.submitError
}
func (f *fakeRunner) Inspect(_ context.Context, selector, id string) (runnerclient.Inspection, error) {
	f.inspects++
	if selector == "request-id" && id != f.request.RequestID || selector == "run-id" && id != f.run.RunID {
		return runnerclient.Inspection{}, errors.New("wrong original selector")
	}
	id = f.run.RunID
	return runnerclient.Inspection{Run: &f.run, Command: &runnercontract.Command{RequestID: f.request.RequestID, Command: "submit", RunID: &id}}, nil
}
func (f *fakeRunner) Call(_ context.Context, command string, args []string, r *runnercontract.Request) (runnerclient.Response, error) {
	if command != "stop" || len(args) != 2 || args[1] != f.run.RunID {
		return runnerclient.Response{}, errors.New("unexpected business action")
	}
	f.stops++
	f.run.Status, f.run.StopStatus = "stopped", "stopped"
	f.run.Revision++
	raw, _ := json.Marshal(runnerclient.Receipt{RunID: f.run.RunID, CommandID: "stop-command", ReceiptStatus: "accepted"})
	return runnerclient.Response{Data: raw}, nil
}
func attachFake(s *Service) *fakeRunner {
	f := &fakeRunner{}
	s.Runner = f
	s.RunnerBuild = runnercontract.BuildPin{StateDir: "C:/fixture/runner"}
	raw, e := os.ReadFile("../runnerclient/testdata/content-capabilities.json")
	if e != nil {
		panic(e)
	}
	var caps runnerclient.Capabilities
	if e = json.Unmarshal(raw, &caps); e != nil {
		panic(e)
	}
	cat, e := runnerclient.CompileCatalog(caps)
	if e != nil {
		panic(e)
	}
	s.Prepare = func(context.Context) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
		return runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"read_only","model":"gpt-6.1-sol","reasoning_effort":"high"}`)}, cat, nil
	}
	return f
}
func step(t *testing.T, s *Service) {
	t.Helper()
	if e := s.ProcessNext(context.Background()); e != nil {
		t.Fatal(e)
	}
}
func prepared(t *testing.T) (*Service, Source, *fakeRunner) {
	t.Helper()
	s, source, _ := fixture()
	f := s.Runner.(*fakeRunner)
	if e := s.Sync(context.Background(), []Source{source}); e != nil {
		t.Fatal(e)
	}
	step(t, s)
	return s, source, f
}
func TestAdvisoryGenerateWireAndSettledOutput(t *testing.T) {
	s, source, f := prepared(t)
	step(t, s)
	var in map[string]json.RawMessage
	_ = json.Unmarshal(f.request.Input, &in)
	var g map[string]json.RawMessage
	_ = json.Unmarshal(in["generate"], &g)
	for _, key := range []string{"repository", "directory", "output_schema"} {
		if _, ok := g[key]; ok {
			t.Fatal("unexpected capability", key)
		}
	}
	r, _ := s.Get(context.Background(), SourceID(s.Project, source))
	var instruction string
	_ = json.Unmarshal(g["instruction"], &instruction)
	if instruction != r.PreparedText || !strings.Contains(instruction, "Empty input is unspecified") || !strings.Contains(instruction, "本次仅需理解已提供的正文") {
		t.Fatal("actual wire lost frozen complete input")
	}
	step(t, s)
	r, _ = s.Get(context.Background(), r.ID)
	if !r.Complete() || r.State != "ready" || r.Suggestion == nil || f.submits != 1 {
		t.Fatal(r, f.submits)
	}
}
func TestAdvisoryUnknownSubmitRestartOnlyObserves(t *testing.T) {
	s, source, f := prepared(t)
	f.submitError = &runnerclient.Failure{Code: "lost_receipt", Unknown: true}
	step(t, s)
	restarted := &Service{Project: s.Project, Store: s.Store, Runner: f, RunnerBuild: s.RunnerBuild}
	step(t, restarted)
	r, _ := s.Get(context.Background(), SourceID(s.Project, source))
	if f.submits != 1 || f.inspects != 1 || r.State != "ready" {
		t.Fatal(r, f.submits, f.inspects)
	}
}
func TestAdvisoryCancellationWinsDuringSubmissionAndStopsOriginal(t *testing.T) {
	s, source, f := prepared(t)
	entered := make(chan struct{})
	f.onSubmit = func(ctx context.Context) { close(entered); <-ctx.Done() }
	f.submitError = &runnerclient.Failure{Unknown: true, Code: "cancelled_transport"}
	done := make(chan error, 1)
	go func() { done <- s.ProcessNext(context.Background()) }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	r, _ := s.Get(ctx, SourceID(s.Project, source))
	r, e := s.Cancel(ctx, r.ID, r.Revision)
	if e != nil || r.Settled || r.State != "cancelling" {
		t.Fatal(r, e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	f.run.StopStatus, f.run.Status = "running", "running"
	step(t, s)
	r, _ = s.Get(context.Background(), r.ID)
	if r.Settled || f.stops != 1 || r.StopRequest == nil {
		t.Fatal(r, f.stops)
	}
	step(t, s)
	r, _ = s.Get(context.Background(), r.ID)
	if r.State != "cancelled" || !r.Complete() || r.Suggestion != nil || f.submits != 1 || f.stops != 1 {
		t.Fatal(r)
	}
}
func TestAdvisoryResultWaitsForSettlementAndSharesBudget(t *testing.T) {
	s, source, f := prepared(t)
	q := runnerbinding.NewContentQueue(1)
	s.Queue = q
	q.UpdateContent("other", "roadmap:other", time.Now().Add(-time.Hour), true, false)
	r, _ := s.Get(context.Background(), SourceID(s.Project, source))
	s.track(r)
	step(t, s)
	if f.submits != 0 {
		t.Fatal("shared budget bypassed")
	}
	q.UpdateContent("other", "roadmap:other", time.Time{}, true, true)
	f.keepRunning = true
	step(t, s)
	step(t, s)
	r, _ = s.Get(context.Background(), r.ID)
	if r.State != "settling" || r.Complete() || r.Suggestion == nil {
		t.Fatal(r)
	}
	q.UpdateContent("other", "roadmap:new", time.Now(), false, false)
	if q.AcquireContent("other", "roadmap:new") {
		t.Fatal("released before settlement")
	}
	f.run.Status, f.run.StopStatus = "completed", "stopped"
	step(t, s)
	if !q.AcquireContent("other", "roadmap:new") {
		t.Fatal("settled slot not released")
	}
}
func TestAdvisoryRejectsForgedObservationAndInvalidOutput(t *testing.T) {
	for _, kind := range []string{"correlation", "digest", "source", "command", "empty-attempt", "action"} {
		t.Run(kind, func(t *testing.T) {
			s, source, f := prepared(t)
			step(t, s)
			if kind == "correlation" {
				f.run.Correlation["steerline_project"] = "wrong"
			}
			if kind == "digest" {
				r, _ := s.Get(context.Background(), SourceID(s.Project, source))
				r.RunnerDigest = "different"
				_, _ = s.save(context.Background(), r)
			}
			if kind == "source" {
				f.run.ContentResult.Source.TurnID = "different"
			}
			if kind == "command" {
				f.run.ContentResult.Source.CommandID = "other-command"
			}
			if kind == "empty-attempt" {
				f.run.ContentResult.Source.AttemptID = ""
				f.run.Execution.AttemptID = ""
				f.run.Final.AttemptID = ""
			}
			if kind == "action" {
				f.run.ContentResult.Text = `{"answer":"apply"}`
				f.run.ContentResult.Source.Text = f.run.ContentResult.Text
				f.run.Final.Text = f.run.ContentResult.Text
			}
			e := s.ProcessNext(context.Background())
			r, _ := s.Get(context.Background(), SourceID(s.Project, source))
			if r.Suggestion != nil || r.State == "ready" {
				t.Fatal("accepted forged observation", r)
			}
			if (kind == "correlation" || kind == "digest") && e == nil {
				t.Fatal("binding accepted")
			}
		})
	}
}
func TestAdvisoryPreparationFailureRetainsPartialContext(t *testing.T) {
	s, source, _ := fixture()
	s.Build = func(_ context.Context, a Source, id string) (contract.Request, error) {
		return contract.Request{Protocol: contract.Protocol, RequestID: id, Capability: contract.Capability, Context: contract.Context{Goal: a.Goal, Materials: []contract.Material{{ID: "question", Label: "原题", Body: "provided"}}}}, errors.New("missing bound Spec")
	}
	if e := s.Sync(context.Background(), []Source{source}); e != nil {
		t.Fatal(e)
	}
	step(t, s)
	r, _ := s.Get(context.Background(), SourceID(s.Project, source))
	if r.State != "failed" || r.Request == nil || len(r.Request.Context.Materials) != 1 {
		t.Fatal(r)
	}
}

func TestAdvisorySourceReappearsWithoutRegenerationOrSnapshotReplacement(t *testing.T) {
	for _, state := range []string{"ready", "failed", "cancelled", "cancelling"} {
		t.Run(state, func(t *testing.T) {
			s, source, f := prepared(t)
			step(t, s)
			step(t, s)
			ctx := context.Background()
			id := SourceID(s.Project, source)
			original, e := s.Get(ctx, id)
			if e != nil {
				t.Fatal(e)
			}
			original.State = state
			original.Cancelled = state == "cancelled" || state == "cancelling"
			original.Settled = state != "cancelling"
			original.StopRequest = runnerbinding.ContentStop("immutable-stop", 1)
			if _, e = s.save(ctx, original); e != nil {
				t.Fatal(e)
			}
			if e = s.Sync(ctx, nil); e != nil {
				t.Fatal(e)
			}
			hidden, _ := s.Get(ctx, id)
			if hidden.SourceActive {
				t.Fatal("resolved source remained active")
			}
			source.Goal = "different later text"
			source.Revision++
			if e = s.Sync(ctx, []Source{source}); e != nil {
				t.Fatal(e)
			}
			restored, _ := s.Get(ctx, id)
			before, _ := json.Marshal(original.RunnerRequest)
			after, _ := json.Marshal(restored.RunnerRequest)
			oldSource, _ := json.Marshal(original.Source)
			newSource, _ := json.Marshal(restored.Source)
			if !restored.SourceActive || restored.State != state || restored.Cancelled != original.Cancelled || restored.Settled != original.Settled || restored.InputDigest != original.InputDigest || string(before) != string(after) || string(oldSource) != string(newSource) || restored.StopRequest.RequestID != "immutable-stop" || f.submits != 1 {
				t.Fatal("source restoration changed original generation", restored)
			}
		})
	}
}
