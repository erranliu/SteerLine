package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/steeragent/suggestion/contract"
	suggestionbridge "github.com/erranliu/SteerLine/internal/steeragentbridge/suggestion"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	nativefixture "github.com/erranliu/SteerLine/internal/testkit/protocol"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

func TestContinuationSuggestionCurrentQuestionReadIsExactAndReadOnly(t *testing.T) {
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "advisory.db"))
	require(t, err)
	defer store.Close()
	source := suggestionbridge.Source{CandidateID: strings.Repeat("b", 32), Occurrence: "attempt:question", Revision: 1, Reason: "pending_question"}
	id := suggestionbridge.SourceID(c.Identity(), source)
	record := suggestionbridge.Record{Project: c.Identity(), ID: id, Revision: 1, Source: source, State: "ready", Settled: true, Suggestion: &contract.Suggestion{Prompt: "原建议\r\n保留字节"}}
	raw, err := json.Marshal(record)
	require(t, err)
	require(t, store.SaveAdvisory(ctx, c.Identity(), id, source.CandidateID, raw, 0))
	p := &continuationSuggestionPort{config: c, service: &suggestionbridge.Service{Project: c.Identity(), Store: store}}
	for _, test := range []struct {
		candidate, occurrence string
		found                 bool
	}{
		{source.CandidateID, source.Occurrence, true},
		{source.CandidateID, "old-attempt:question", false},
		{source.CandidateID, "attempt:other-question", false},
		{strings.Repeat("c", 32), source.Occurrence, false},
	} {
		out, err := p.read(ctx, "/main-agent", url.Values{"candidate_id": {test.candidate}, "occurrence": {test.occurrence}})
		require(t, err)
		encoded, err := json.Marshal(out)
		require(t, err)
		var response struct {
			Record *suggestionbridge.Detail `json:"record"`
		}
		require(t, json.Unmarshal(encoded, &response))
		if (response.Record != nil) != test.found {
			t.Fatalf("wrong question selected: %s", encoded)
		}
		if test.found && (response.Record.Occurrence != source.Occurrence || response.Record.Prompt != record.Suggestion.Prompt) {
			t.Fatal(response)
		}
	}
	for _, values := range []url.Values{
		{"candidate_id": {source.CandidateID}, "occurrence": {""}},
		{"candidate_id": {"bad"}, "occurrence": {source.Occurrence}},
		{"candidate_id": {source.CandidateID}, "occurrence": {source.Occurrence, source.Occurrence}},
	} {
		if _, err := p.read(ctx, "/main-agent", values); err == nil {
			t.Fatal("invalid source accepted", values)
		}
	}
	after, _, err := store.LoadAdvisory(ctx, c.Identity(), id)
	require(t, err)
	if !bytes.Equal(raw, after) {
		t.Fatal("GET changed advisory")
	}
}

func TestContinuationSuggestionConfigurationFreezesAtPreparation(t *testing.T) {
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	c.runtimeGenerations = runnerbinding.NewContentQueue(1)
	raw, err := os.ReadFile("../runnerclient/testdata/content-capabilities.json")
	require(t, err)
	var caps runnerclient.Capabilities
	require(t, json.Unmarshal(raw, &caps))
	catalog, err := runnerclient.CompileCatalog(caps)
	require(t, err)
	c.runtimeNative = &runnerclient.ConfigurationSnapshot{Catalog: catalog, Query: nativefixture.ExecutionConfigurationFixture}
	c.RunnerExecution.SteerAgent = &executionconfig.SteerAgent{Model: "model-before", ReasoningEffort: "high"}
	profiles, _ := json.Marshal(c.RunnerExecution.Profiles)
	db := filepath.Join(t.TempDir(), "advisory.db")
	store, err := sqlite.OpenSQLiteStore(db)
	require(t, err)
	t.Cleanup(func() { _ = store.Close() })
	cli := runnerclient.Client{StateDir: t.TempDir()}
	configure := func(config Config) *continuationSuggestionPort {
		p := newContinuationSuggestionPort(config, store, cli, nil)
		p.service.Build = func(_ context.Context, source suggestionbridge.Source, id string) (contract.Request, error) {
			return contract.Request{Protocol: contract.Protocol, Capability: contract.Capability, RequestID: id, Context: contract.Context{Goal: source.Goal, Materials: []contract.Material{}}}, nil
		}
		p.service.Runner = &advisoryPublicRunner{}
		return p
	}
	loaded := configure(c)
	source := suggestionbridge.Source{CandidateID: strings.Repeat("b", 32), Occurrence: "attempt:original", Revision: 1, Reason: "pending_question", Goal: "Confirm the original requirement"}
	require(t, loaded.service.Sync(ctx, []suggestionbridge.Source{source}))
	require(t, loaded.service.ProcessNext(ctx))
	id := suggestionbridge.SourceID(c.Identity(), source)
	original, err := loaded.service.Get(ctx, id)
	require(t, err)
	if original.RunnerRequest == nil || original.State != "queued" || original.Submission != "pending" {
		t.Fatal("original request was not prepared and queued", original)
	}
	originalRequest, _ := json.Marshal(original.RunnerRequest)
	// Saving a new configuration does not replace an already loaded host snapshot.
	saved := c.RunnerExecution
	saved.SteerAgent = &executionconfig.SteerAgent{Model: "model-after", ReasoningEffort: "medium"}
	configFile := filepath.Join(t.TempDir(), "execution.json")
	encoded, _ := json.Marshal(saved)
	require(t, os.WriteFile(configFile, encoded, 0600))
	stillLoaded, _, err := loaded.service.Prepare(ctx)
	require(t, err)
	var settings struct{ Model, ReasoningEffort string }
	require(t, json.Unmarshal(stillLoaded.Settings, &settings))
	if settings.Model != "model-before" {
		t.Fatal("saving changed the loaded host", string(stillLoaded.Settings))
	}
	// Reopen persisted records, then construct the owner from the newly loaded file.
	require(t, store.Close())
	store, err = sqlite.OpenSQLiteStore(db)
	require(t, err)
	encoded, err = os.ReadFile(configFile)
	require(t, err)
	reloaded := c
	require(t, executionconfig.Decode(encoded, &reloaded.RunnerExecution))
	restarted := configure(reloaded)
	require(t, restarted.service.ProcessNext(ctx))
	fake := restarted.service.Runner.(*advisoryPublicRunner)
	var sent runnercontract.GenerateRunInput
	require(t, json.Unmarshal(fake.request.Input, &sent))
	require(t, json.Unmarshal(sent.Execution.Settings, &settings))
	if settings.Model != "model-before" || fake.request.RequestID != original.RunnerRequest.RequestID {
		t.Fatal("queued request was rebuilt using new settings", sent.Execution, fake.request)
	}
	require(t, restarted.service.ProcessNext(ctx))
	restored, err := restarted.service.Get(ctx, id)
	require(t, err)
	restoredRequest, _ := json.Marshal(restored.RunnerRequest)
	if !bytes.Equal(originalRequest, restoredRequest) || restored.InputDigest != original.InputDigest || !restored.Complete() {
		t.Fatal("original request or digest changed across restart", restored)
	}
	next := source
	next.Occurrence = "attempt:next"
	require(t, restarted.service.Sync(ctx, []suggestionbridge.Source{source, next}))
	require(t, restarted.service.ProcessNext(ctx))
	prepared, err := restarted.service.Get(ctx, suggestionbridge.SourceID(c.Identity(), next))
	require(t, err)
	if prepared.RunnerRequest == nil {
		t.Fatal("new request not prepared", prepared)
	}
	require(t, json.Unmarshal(prepared.RunnerRequest.Input, &sent))
	var selected map[string]any
	require(t, json.Unmarshal(sent.Execution.Settings, &selected))
	afterProfiles, _ := json.Marshal(reloaded.RunnerExecution.Profiles)
	if selected["model"] != "model-after" || selected["reasoning_effort"] != "medium" || selected["access_mode"] != "read_only" || !bytes.Equal(profiles, afterProfiles) || fake.submits != 1 {
		t.Fatal("new snapshot or unrelated profiles changed", selected, fake.submits)
	}
}

func TestContinuationSuggestionAdvisorySourceIsExplicitAndFreezesOwnerFacts(t *testing.T) {
	c := runnerstate.Candidate{ID: strings.Repeat("a", 32), TaskID: "1", Participation: "active", BaseRevision: "fixed", ContractDigest: "contract", Projection: &runnercontract.Run{Revision: 7, Execution: runnercontract.Execution{AttemptID: "attempt"}, Question: &runnercontract.Question{ID: "question", Text: "Conflicting acceptance requirements"}, Controls: runnercontract.Controls{CanReply: true, Reason: "waiting for native answer"}}}
	c.TestPlan = json.RawMessage(`{"note":"prior test arrangement"}`)
	c.Projection.Final = &runnercontract.Final{Verified: true, Text: "prior verification report"}
	s, ok := materialAdviceSource(c)
	if !ok || s.Occurrence != "attempt:question" || len(s.Facts) != 1 {
		t.Fatal(s, ok)
	}
	c.Projection.Question.Text = "later observation"
	if s.Goal != "Conflicting acceptance requirements" || !strings.Contains(s.Facts[0].Body, "Conflicting acceptance requirements") {
		t.Fatal("source was not frozen")
	}
	for _, reason := range []string{"waiting_input", "blocked", "needs_attention", "permission_required", ""} {
		c.Projection.Controls.Reason = reason
		if _, ok := materialAdviceSource(c); !ok {
			t.Fatal("real pending question depended on diagnostic prose", reason)
		}
		c.Projection.Controls.CanReply = false
		if _, ok := materialAdviceSource(c); ok {
			t.Fatal("non-pending question triggered", reason)
		}
		c.Projection.Controls.CanReply = true
	}
	c.Projection.Question = nil
	if _, ok := materialAdviceSource(c); ok {
		t.Fatal("generic attention without question triggered")
	}
}
func TestContinuationSuggestionAdvisoryLegacyActionsRejectedBeforeBusinessAccess(t *testing.T) {
	// No Server, actor, store or Runner: accidental access would panic.
	p := &runtimeProjectPort{}
	for _, action := range []string{"main_agent_answer", "main_agent_publish", "main_agent_prepare", "main_agent_analyze", "main_agent_suggest"} {
		_, err := p.SubmitContinuationSuggestion(context.Background(), httpcontract.CommandEnvelope{Action: action, Payload: json.RawMessage(`{}`)})
		if err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatal(action, err)
		}
	}
}

type advisoryPublicRunner struct {
	request           runnercontract.Request
	run               runnercontract.Run
	submits, inspects int
}

func (f *advisoryPublicRunner) Submit(_ context.Context, r runnercontract.Request) (runnerclient.Receipt, error) {
	f.request = r
	f.submits++
	var in runnercontract.GenerateRunInput
	if e := json.Unmarshal(r.Input, &in); e != nil {
		return runnerclient.Receipt{}, e
	}
	f.run = runnercontract.Run{RunID: "advisory-run", Revision: 1, InputDigest: "frozen-digest", Workflow: in.Workflow, Correlation: in.Correlation, Status: "completed", StopStatus: "stopped", Execution: runnercontract.Execution{AttemptID: "attempt", SessionID: "session", TurnID: "turn"}, ContentResult: &runnercontract.ContentResult{Status: "succeeded", Text: "请结合原题和已提供的任务材料，继续讨论并确认需要补充的信息。", Source: runnercontract.Final{Verified: true, NativeOutcome: "succeeded", Source: "provider.final", ItemID: "item", CommandID: "command", AttemptID: "attempt", SessionID: "session", TurnID: "turn"}}}
	final := f.run.ContentResult.Source
	final.Text = f.run.ContentResult.Text
	f.run.ContentResult.Source = final
	f.run.Final = &final
	return runnerclient.Receipt{RunID: f.run.RunID, CommandID: "submit", ReceiptStatus: "accepted"}, nil
}
func (f *advisoryPublicRunner) Inspect(_ context.Context, _ string, _ string) (runnerclient.Inspection, error) {
	f.inspects++
	id := f.run.RunID
	return runnerclient.Inspection{Run: &f.run, Command: &runnercontract.Command{RequestID: f.request.RequestID, RunID: &id, Command: "submit"}}, nil
}
func (*advisoryPublicRunner) Call(context.Context, string, []string, *runnercontract.Request) (runnerclient.Response, error) {
	return runnerclient.Response{}, errors.New("unexpected execution")
}

func TestContinuationSuggestionAdvisoryPersistedQuestionAutomaticallyGeneratesWithoutWeb(t *testing.T) {
	ctx := context.Background()
	c, _ := nativeTaskFixture(t)
	prepareStoppedRunRepository(t, c)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	require(t, os.MkdirAll(EtcDir(), 0700))
	require(t, os.WriteFile(filepath.Join(EtcDir(), "one.json"), projectConfigJSON(t, c), 0600))
	directory, runtime := startPresetDirectory(t)
	candidate := seedStoppedPublicRun(t, runtime, c)
	frozen := runtime.config
	require(t, directory.Close())
	store, e := OpenNativeTaskCommandStore(ctx, &c)
	require(t, e)
	defer store.Close()
	candidate, e = store.Candidate(ctx, c.Identity(), candidate.ID)
	require(t, e)
	candidate.Revision++
	candidate.Projection.Execution.AttemptID = "real-owner-attempt"
	candidate.Projection.Question = &runnercontract.Question{ID: "real-owner-question", Text: "请确认这一项任务的验收依据", AllowFreeText: true}
	candidate.Projection.Controls.CanReply = true
	candidate.Projection.Final = &runnercontract.Final{Verified: true, Text: "prior verification report"}
	require(t, store.SaveCandidate(ctx, candidate, candidate.Revision-1, nil))
	raw, e := os.ReadFile("../runnerclient/testdata/content-capabilities.json")
	require(t, e)
	var caps runnerclient.Capabilities
	require(t, json.Unmarshal(raw, &caps))
	cat, e := runnerclient.CompileCatalog(caps)
	require(t, e)
	frozen.runtimeNative = &runnerclient.ConfigurationSnapshot{Catalog: cat, Query: nativefixture.ExecutionConfigurationFixture}
	port := newContinuationSuggestionPort(frozen, store, runtime.runner.client, nil)
	fake := &advisoryPublicRunner{}
	port.service.Runner = fake
	source, ok := materialAdviceSource(candidate)
	if !ok {
		t.Fatal("actual persisted pending question not eligible", candidate.StopRequested)
	}
	id := suggestionbridge.SourceID(c.Identity(), source)
	worker, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); port.run(worker) }()
	t.Cleanup(func() { stop(); <-done })
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for {
		record, err := port.service.Get(ctx, id)
		if err == nil && record.Complete() {
			if record.State != "ready" || record.Suggestion == nil || record.Request == nil || len(record.Request.Context.Materials) != 1 || record.Request.Context.Materials[0].ID != "question" {
				t.Fatal(record)
			}
			break
		}
		select {
		case <-deadline.C:
			stop()
			<-done
			t.Fatal("background did not complete", record, err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	stop()
	<-done
	if fake.submits != 1 {
		t.Fatal("wrong number of model submissions", fake.submits)
	}
	record, e := port.service.Get(ctx, id)
	require(t, e)
	var sent runnercontract.GenerateRunInput
	require(t, json.Unmarshal(fake.request.Input, &sent))
	instruction := sent.Generate.Instruction
	if instruction != record.PreparedText || !strings.Contains(instruction, candidate.Projection.Question.Text) {
		t.Fatal("Runner did not receive the frozen original question")
	}
	for _, extra := range []string{"Frozen implementation plan", "prior verification report"} {
		if strings.Contains(instruction, extra) {
			t.Fatal("unexpected material in Runner answer suggestion", extra)
		}
	}
	before := fake.inspects
	_, e = port.read(ctx, "/main-agent/"+id, nil)
	require(t, e)
	facts, _, e := store.ReadAsyncActivityFacts(ctx, c.Identity(), ports.AsyncActivityRead{Source: "main_agent_advisory", Filter: "summary", Limit: 50})
	require(t, e)
	if len(facts) != 1 {
		t.Fatal(facts)
	}
	activity, e := query.ProjectAsyncActivity(c.Identity(), "instance", facts[0], time.Now())
	require(t, e)
	if activity.State != "succeeded" || !activity.NeedsAttention || activity.Details.Target != id || fake.inspects != before {
		t.Fatal(activity)
	}
	restarted := newContinuationSuggestionPort(frozen, store, runtime.runner.client, nil)
	restarted.service.Runner = fake
	require(t, restarted.tick(ctx))
	if fake.submits != 1 {
		t.Fatal("restart resubmitted same question")
	}
	candidate, e = store.Candidate(ctx, c.Identity(), candidate.ID)
	require(t, e)
	candidate.Revision++
	candidate.Projection.Question = nil
	candidate.Projection.Controls.CanReply = false
	require(t, store.SaveCandidate(ctx, candidate, candidate.Revision-1, nil))
	require(t, restarted.tick(ctx))
	record, e = restarted.service.Get(ctx, id)
	require(t, e)
	if record.SourceActive || record.State != "ready" {
		t.Fatal("resolved source rewrote history", record)
	}
	originalGoal := record.Request.Context.Goal
	candidate.Revision++
	candidate.Projection.Question = &runnercontract.Question{ID: "real-owner-question", Text: "different later display"}
	candidate.Projection.Controls.CanReply = true
	require(t, store.SaveCandidate(ctx, candidate, candidate.Revision-1, nil))
	require(t, restarted.tick(ctx))
	restored, e := restarted.service.Get(ctx, id)
	require(t, e)
	if !restored.SourceActive || restored.State != "ready" || restored.Request.Context.Goal != originalGoal || fake.submits != 1 {
		t.Fatal("returning question replaced or reopened suggestion", restored)
	}
	facts, _, e = store.ReadAsyncActivityFacts(ctx, c.Identity(), ports.AsyncActivityRead{Source: "main_agent_advisory", Filter: "summary", Limit: 50})
	require(t, e)
	activity, e = query.ProjectAsyncActivity(c.Identity(), "instance", facts[0], time.Now())
	require(t, e)
	if !activity.NeedsAttention {
		t.Fatal("returning question did not restore attention")
	}
}

func TestContinuationSuggestionAdvisoryColdAbsenceAndOriginalStops(t *testing.T) {
	for _, kind := range []string{"absent", "cancelled-absent", "failed-stop", "cancelled-stop", "present"} {
		t.Run(kind, func(t *testing.T) {
			c, _ := nativeTaskFixture(t)
			cli := startupFixtureCLI(t)
			ctx := context.Background()
			store, e := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
			require(t, e)
			defer store.Close()
			c.runtimeGenerations = runnerbinding.NewContentQueue(1)
			request := runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "original-advisory", Input: json.RawMessage(`{}`)}
			record := suggestionbridge.Record{ID: strings.Repeat("a", 32), Project: c.Identity(), Revision: 1, Source: suggestionbridge.Source{CandidateID: strings.Repeat("b", 32)}, Runner: cli.BuildPin(), RunnerRequest: &request, Submission: "started", State: "unknown", CreatedAt: time.Now().UTC(), Settled: false}
			record.Cancelled = strings.HasPrefix(kind, "cancelled")
			if kind == "failed-stop" {
				record.State = "failed"
				record.StopRequest = runnerbinding.ContentStop("original-stop", 3)
				record.StopState = "unknown"
			}
			raw, _ := json.Marshal(record)
			require(t, store.SaveAdvisory(ctx, c.Identity(), record.ID, record.Source.CandidateID, raw, 0))
			if strings.Contains(kind, "absent") {
				require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-run-fixture.json"), []byte(`{"fixture_error":"request_not_found"}`), 0600))
			} else {
				writeStartupInspection(t, cli, runnercontract.Run{RunID: "original-run", Revision: 3, InputDigest: "original-digest", Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Correlation: map[string]string{"steerline_project": c.Identity(), "steerline_advisory": record.ID}}, request.RequestID)
				if kind == "cancelled-stop" {
					raw, err := os.ReadFile(filepath.Join(cli.StateDir, "idle-run-fixture.json"))
					require(t, err)
					var original runnercontract.Inspection
					require(t, json.Unmarshal(raw, &original))
					require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-inspections-fixture.json"), mustJSON(map[string]any{request.RequestID: original}), 0600))
				}
				if kind == "failed-stop" {
					raw, err := os.ReadFile(filepath.Join(cli.StateDir, "idle-run-fixture.json"))
					require(t, err)
					var original runnercontract.Inspection
					require(t, json.Unmarshal(raw, &original))
					stop := original
					stop.Command = &runnercontract.Command{CommandID: "stop-command", RequestID: "original-stop", Command: "stop", RunID: &stop.Run.RunID, ReceiptStatus: "accepted", ActionStatus: "pending"}
					require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-inspections-fixture.json"), mustJSON(map[string]any{request.RequestID: original, "original-stop": stop}), 0600))
				}

			}
			stops, e := prepareAdvisoryStops(ctx, &c, store, &cli)
			require(t, e)
			rows, e := advisoryRecords(ctx, store, c.Identity())
			require(t, e)
			saved := rows[0]
			if strings.Contains(kind, "absent") {
				want := "failed"
				if record.Cancelled {
					want = "cancelled"
				}
				if saved.State != want || !saved.Complete() || len(stops) != 0 || saved.RunnerRequest.RequestID != request.RequestID {
					t.Fatal(saved, stops)
				}
				c.runtimeGenerations.UpdateContent("other", "next", time.Now(), false, false)
				if !c.runtimeGenerations.AcquireContent("other", "next") {
					t.Fatal("cold absent generation stranded shared slot")
				}
			} else if kind != "present" {
				if len(stops) != 1 || stops[0].RunID != "original-run" || saved.StopRequest == nil {
					t.Fatal(stops, saved)
				}
				if kind == "failed-stop" && stops[0].Request.RequestID != "original-stop" {
					t.Fatal("replaced unknown stop")
				}
				again, e := prepareAdvisoryStops(ctx, &c, store, &cli)
				require(t, e)
				if len(again) != 1 || again[0].Request.RequestID != stops[0].Request.RequestID {
					t.Fatal("stop identity changed on restart")
				}
			} else if len(stops) != 0 {
				t.Fatal("uncancelled generation was stopped", stops)
			}
			status, e := cli.ServiceStatus(ctx)
			require(t, e)
			if status.Status != "not_running" {
				t.Fatal("pre-start verification started Runner")
			}
		})
	}
}
