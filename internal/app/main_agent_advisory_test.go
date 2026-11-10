package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/mainagentbridge"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	nativefixture "github.com/erranliu/SteerLine/internal/testkit/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

func TestMainAgentAdvisorySourceIsExplicitAndFreezesOwnerFacts(t *testing.T) {
	c := runnerstate.Candidate{ID: strings.Repeat("a", 32), TaskID: "1", Participation: "active", BaseRevision: "fixed", ContractDigest: "contract", Projection: &runnercontract.Run{Revision: 7, Execution: runnercontract.Execution{AttemptID: "attempt"}, Question: &runnercontract.Question{ID: "question", Text: "Conflicting acceptance requirements"}, Controls: runnercontract.Controls{CanReply: true, Reason: "waiting for native answer"}}}
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
func TestMainAgentAdvisoryLegacyActionsRejectedBeforeBusinessAccess(t *testing.T) {
	// No Server, actor, store or Runner: accidental access would panic.
	p := &runtimeProjectPort{}
	for _, action := range []string{"main_agent_answer", "main_agent_publish", "main_agent_prepare", "main_agent_analyze", "main_agent_suggest"} {
		_, err := p.SubmitMainAgent(context.Background(), httpcontract.CommandEnvelope{Action: action, Payload: json.RawMessage(`{}`)})
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

func TestMainAgentAdvisoryPersistedQuestionAutomaticallyGeneratesWithoutWeb(t *testing.T) {
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
	require(t, store.SaveCandidate(ctx, candidate, candidate.Revision-1, nil))
	raw, e := os.ReadFile("../runnerclient/testdata/content-capabilities.json")
	require(t, e)
	var caps runnerclient.Capabilities
	require(t, json.Unmarshal(raw, &caps))
	cat, e := runnerclient.CompileCatalog(caps)
	require(t, e)
	frozen.runtimeNative = &runnerclient.ConfigurationSnapshot{Catalog: cat, Query: nativefixture.ExecutionConfigurationFixture}
	port := newMainAgentPort(frozen, store, runtime.runner.client, nil)
	fake := &advisoryPublicRunner{}
	port.service.Runner = fake
	source, ok := materialAdviceSource(candidate)
	if !ok {
		t.Fatal("actual persisted pending question not eligible", candidate.StopRequested)
	}
	id := mainagentbridge.SourceID(c.Identity(), source)
	worker, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); port.run(worker) }()
	t.Cleanup(func() { stop(); <-done })
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for {
		record, err := port.service.Get(ctx, id)
		if err == nil && record.Complete() {
			if record.State != "ready" || record.Suggestion == nil || record.Request == nil || len(record.Request.Context.Materials) < 3 {
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
	if !strings.Contains(record.PreparedText, candidate.Projection.Question.Text) || !strings.Contains(record.PreparedText, "Frozen implementation plan") {
		t.Fatal("real frozen owner materials absent")
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
	restarted := newMainAgentPort(frozen, store, runtime.runner.client, nil)
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

func TestMainAgentAdvisoryColdAbsenceAndOriginalStops(t *testing.T) {
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
			record := mainagentbridge.Record{ID: strings.Repeat("a", 32), Project: c.Identity(), Revision: 1, Source: mainagentbridge.Source{CandidateID: strings.Repeat("b", 32)}, Runner: cli.BuildPin(), RunnerRequest: &request, Submission: "started", State: "unknown", CreatedAt: time.Now().UTC(), Settled: false}
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
