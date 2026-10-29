package environmentadaptation_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation"
	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

var ctx = context.Background()

type runner struct {
	run              *runnercontract.Run
	body             []byte
	submits, stops   int
	resultReads      int
	resultPending    bool
	resultDiagnostic string
	unknown          bool
	stopBody         []byte
	resultErr        bool
	stopReject       bool
	stopUnknown      bool
	inspectRevision  uint64
	inspectErr       bool
}

func (r *runner) Call(_ context.Context, command string, _ []string, q *runnerclient.Request) (runnerclient.Response, error) {
	if command == "submit" {
		r.submits++
		var input runnercontract.RunInput
		_ = json.Unmarshal(q.Input, &input)
		r.run = &runnercontract.Run{RunID: fmt.Sprint("run-", r.submits), Revision: 1, Status: "running", InputDigest: envmodel.Digest(q.Input), Correlation: input.Correlation, Execution: runnercontract.Execution{Branch: fmt.Sprint("head-", r.submits)}}
		if input.ResultFile != nil {
			r.body, _ = base64.StdEncoding.DecodeString(input.ResultFile.InitialContentBase64)
		}
		if r.unknown {
			return runnerclient.Response{}, &runnerclient.Failure{Unknown: true, Code: "unknown"}
		}
	}
	if command == "stop" {
		r.stops++
		if r.stopReject {
			r.stopReject = false
			return runnerclient.Response{}, &runnerclient.Failure{Code: "stale_revision", Exit: 3}
		}
		if r.stopUnknown {
			return runnerclient.Response{}, &runnerclient.Failure{Code: "outcome_unknown", Unknown: true}
		}
		r.run.StopStatus = "stopped"
		r.run.Status = "stopped"
		r.run.Revision++
		if r.stopBody != nil {
			r.body = r.stopBody
		}
	}
	return runnerclient.Response{}, nil
}
func (r *runner) Inspect(context.Context, string, string, bool) (runnerclient.Inspection, error) {
	if r.unknown || r.inspectErr {
		return runnerclient.Inspection{}, errors.New("original request unknown")
	}
	if r.run == nil {
		return runnerclient.Inspection{}, nil
	}
	copy := *r.run
	if r.inspectRevision > 0 {
		copy.Revision = r.inspectRevision
	}
	return runnerclient.Inspection{Run: &copy}, nil
}
func snapshot(contract, run string, body []byte, version uint64, closed bool) runnercontract.ProgressFile {
	encoded := base64.StdEncoding.EncodeToString(body)
	now := time.Now().UTC()
	status := "observing"
	if closed {
		status = "closed"
	}
	return runnercontract.ProgressFile{Contract: contract, RunID: run, Version: version, ObservedAt: &now, Encoding: "base64", ContentBase64: &encoded, SizeBytes: len(body), SHA256: envmodel.Digest(body), CollectionStatus: status}
}
func (r *runner) ProgressFile(context.Context, string) (runnercontract.ProgressFile, error) {
	return snapshot(runnercontract.ProgressContract, r.run.RunID, []byte(`{"test.one":true,"test.two":true,"pr":true}`), 99, false), nil
}
func (r *runner) ResultFile(context.Context, string) (runnercontract.ResultFile, error) {
	r.resultReads++
	if r.resultErr {
		return runnercontract.ResultFile{}, errors.New("file unavailable")
	}
	if r.run.StopStatus != "stopped" || r.resultPending {
		return runnercontract.ResultFile{Contract: runnercontract.ResultContract, RunID: r.run.RunID, Encoding: "base64", CollectionStatus: "pending"}, nil
	}
	if r.resultDiagnostic != "" {
		return runnercontract.ResultFile{Contract: runnercontract.ResultContract, RunID: r.run.RunID, Encoding: "base64", CollectionStatus: "closed", Diagnostic: r.resultDiagnostic}, nil
	}
	return snapshot(runnercontract.ResultContract, r.run.RunID, r.body, 1, true), nil
}
func setup(t *testing.T) (*env.Service, *runner, *sqlite.SQLiteStore, *time.Time) {
	t.Helper()
	store, e := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "steerline.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	remote := &runner{}
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	service := &env.Service{Project: "project", Store: store, Runner: remote, Now: func() time.Time { return now }, Prepare: func(context.Context, string) (envmodel.Prepared, error) {
		return envmodel.Prepared{Commit: strings.Repeat("a", 40), Inputs: "basis", ScriptDigest: strings.Repeat("b", 64), Manifest: envmodel.Manifest{ValidationBase: "validation"}, L2: []string{"one", "two"}, Input: runnercontract.RunInput{Execution: runnercontract.ExecutionInput{Adapter: "orca-terminal", Target: runnercontract.ExecutionTarget{Kind: "orca_environment", Environment: "remote"}}}}, nil
	}}
	return service, remote, store, &now
}
func start(t *testing.T, s *env.Service) envmodel.Record {
	t.Helper()
	id := envmodel.ID()
	r, e := s.Start(ctx, id, json.RawMessage(`{"original":true}`), envmodel.Start{Profile: "remote", TimeoutSeconds: 60}, false)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func read(t *testing.T, s *env.Service, id string) envmodel.Record {
	t.Helper()
	r, e := s.Store.Environment(ctx, s.Project, id)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func report(r envmodel.Record, remote *runner, pass bool) envmodel.Report {
	a := r.Rounds[len(r.Rounds)-1]
	p := envmodel.Report{Version: 1, ActivityID: r.ID, Round: a.Number, Commit: r.Prepared.Commit, ScriptDigest: r.Prepared.ScriptDigest, Completed: true, Checks: []envmodel.Check{{ID: "tool", Status: "passed", Message: "required version available"}}, L2: []envmodel.Item{{ID: "one", Status: "passed"}, {ID: "two", Status: "passed"}}}
	if pass {
		p.PR = envmodel.PR{URL: "https://git/validation/pulls/1", Base: "validation", Head: remote.run.Execution.Branch, ChecksPassed: true, ReviewsPassed: true, Merged: true, MergeCommit: strings.Repeat("c", 40), CleanupCompleted: true}
	} else {
		p.L2[1].Status = "failed"
		p.L2[1].Message = "missing tool"
		p.Message = "tool unavailable"
	}
	return p
}
func setReport(remote *runner, p envmodel.Report) {
	remote.body, _ = json.Marshal(p)
	remote.run.Status = "pr_ready"
	if p.PR.URL == "" {
		remote.run.Status = "failed"
	}
}
func observe(t *testing.T, s *env.Service) {
	t.Helper()
	if e := s.Observe(ctx); e != nil {
		t.Fatal(e)
	}
}

func TestProjectEnvironmentExecutionFinalSnapshotAndBinaryAdmission(t *testing.T) {
	s, remote, _, _ := setup(t)
	r := start(t, s)
	observe(t, s)
	// Progress is fully green while the independent result is still incomplete.
	observe(t, s)
	if read(t, s, r.ID).Status() != "not_ready" {
		t.Fatal("progress admitted task")
	}
	if remote.resultReads != 0 {
		t.Fatal("read result before original Run stopped")
	}
	setReport(remote, report(r, remote, true))
	observe(t, s)
	if read(t, s, r.ID).Status() != "not_ready" || remote.stops != 1 {
		t.Fatal("admitted before stopped final snapshot")
	}
	observe(t, s)
	r = read(t, s, r.ID)
	if r.Status() != "ready" || !r.Settled() || remote.submits != 1 {
		t.Fatalf("%+v", r)
	}
}
func TestProjectEnvironmentExecutionOnlySealedReportDeterminesAdmission(t *testing.T) {
	s, remote, _, _ := setup(t)
	r := start(t, s)
	observe(t, s)
	setReport(remote, report(r, remote, true))
	remote.stopBody, _ = json.Marshal(report(r, remote, false))
	observe(t, s)
	observe(t, s)
	r = read(t, s, r.ID)
	if r.Status() == "ready" || len(r.Rounds) != 2 {
		t.Fatal("ignored final report", r.State, len(r.Rounds))
	}
}
func TestProjectEnvironmentRetryBudgetSurvivesRestart(t *testing.T) {
	s, remote, store, now := setup(t)
	r := start(t, s)
	firstDeadline := r.Rounds[0].Deadline
	for i := 1; i <= 3; i++ {
		observe(t, s)
		r = read(t, s, r.ID)
		setReport(remote, report(r, remote, false))
		observe(t, s)
		observe(t, s)
		r = read(t, s, r.ID)
		s = &env.Service{Project: s.Project, Store: store, Runner: remote, Prepare: s.Prepare, Now: func() time.Time { return *now }}
	}
	if remote.submits != 3 || len(r.Rounds) != 3 || r.State != "failed" || r.Status() != "not_ready" || !r.Rounds[0].Deadline.Equal(firstDeadline) {
		t.Fatalf("budget reset: %+v", r)
	}
	observe(t, s)
	if remote.submits != 3 {
		t.Fatal("fourth submission")
	}
}
func TestProjectEnvironmentExecutionUnknownNeverResubmits(t *testing.T) {
	s, remote, _, now := setup(t)
	r := start(t, s)
	remote.unknown = true
	_ = s.Observe(ctx)
	*now = now.Add(2 * time.Minute)
	for i := 0; i < 3; i++ {
		_ = s.Observe(ctx)
	}
	if remote.submits != 1 || read(t, s, r.ID).Settled() {
		t.Fatal("unknown repeated or released")
	}
	remote.unknown = false
	observe(t, s)
	observe(t, s)
	r = read(t, s, r.ID)
	if remote.stops != 1 || r.Status() == "ready" || r.Settled() || r.State != "unverified" {
		t.Fatal("unknown report/PR must block new work", r.State)
	}
	if _, e := s.Start(ctx, envmodel.ID(), json.RawMessage(`{}`), envmodel.Start{Profile: "remote", TimeoutSeconds: 60}, false); e == nil {
		t.Fatal("new activity overlapped original unknown")
	}
}
func TestProjectEnvironmentExecutionResultQueryFailureCannotAdmitBeforeSeal(t *testing.T) {
	s, remote, _, _ := setup(t)
	r := start(t, s)
	observe(t, s)
	setReport(remote, report(r, remote, true))
	observe(t, s)
	remote.resultErr = true
	if s.Observe(ctx) == nil {
		t.Fatal("missing final silently used cached bytes")
	}
	if read(t, s, r.ID).Status() == "ready" {
		t.Fatal("cached pass admitted")
	}
	remote.resultErr = false
	observe(t, s)
	if read(t, s, r.ID).Status() != "ready" {
		t.Fatal("final read failed to recover")
	}
}

func TestProjectEnvironmentExecutionWaitingInputDoesNotStop(t *testing.T) {
	s, remote, _, _ := setup(t)
	r := start(t, s)
	observe(t, s)
	setReport(remote, report(r, remote, true))
	remote.run.Status = "waiting_input"
	observe(t, s)
	if remote.stops != 0 || remote.resultReads != 0 || read(t, s, r.ID).Status() != "not_ready" {
		t.Fatal("single turn or waiting input prematurely stopped/admitted")
	}
	remote.run.Status = "completed"
	observe(t, s)
	observe(t, s)
	if remote.stops != 1 || read(t, s, r.ID).Status() != "ready" {
		t.Fatal("completed execution failed to stop and seal")
	}
}

func TestProjectEnvironmentRequirementFailureKeepsMixedResultsAndBudget(t *testing.T) {
	s, remote, store, _ := setup(t)
	r := start(t, s)
	for round := 1; round <= 3; round++ {
		observe(t, s)
		r = read(t, s, r.ID)
		p := report(r, remote, false)
		p.Checks = []envmodel.Check{{ID: "git", Status: "passed", Message: "Git available"}, {ID: "unity.editor", Status: "failed", Message: "UNITY_EDITOR_PATH is not set"}}
		for i := range p.L2 {
			p.L2[i].Status = "incomplete"
			p.L2[i].Message = "environment requirements failed; L2 not executed"
		}
		p.PR.CleanupCompleted = true
		setReport(remote, p)
		remote.run.Status = "waiting_input"
		remote.run.Final = &runnercontract.Final{Verified: true, Text: `{"outcome":"blocked"}`}
		observe(t, s)
		observe(t, s)
		s = &env.Service{Project: s.Project, Store: store, Runner: remote, Prepare: s.Prepare, Now: s.Now}
		got := read(t, s, r.ID)
		last := got.Rounds[round-1]
		if last.Report == nil || last.Report.Checks[0].Status != "passed" || last.Report.Checks[1].Status != "failed" || got.Status() != "not_ready" {
			t.Fatal("lost mixed requirement results or admitted failure", got)
		}
	}
	if got := read(t, s, r.ID); got.State != "failed" || !got.Settled() || remote.submits != 3 {
		t.Fatal("three-round requirement failure did not settle", got.State, remote.submits)
	}
}

func TestProjectEnvironmentIncompleteScriptPreservesExecutionFailure(t *testing.T) {
	s, remote, _, _ := setup(t)
	r := start(t, s)
	observe(t, s)
	remote.run.Status = "failed"
	remote.run.Diagnostic = "native readiness unavailable; no input was sent"
	observe(t, s)
	observe(t, s)
	got := read(t, s, r.ID)
	if got.Status() != "not_ready" || !strings.Contains(got.Diagnostic, "验收脚本未完成") || !strings.Contains(got.Diagnostic, "native readiness unavailable") {
		t.Fatal("incomplete initial report hid the execution failure", got.Diagnostic)
	}
}

func TestProjectEnvironmentExecutionPendingSealQueriesOriginalRecord(t *testing.T) {
	s, remote, _, _ := setup(t)
	r := start(t, s)
	observe(t, s)
	setReport(remote, report(r, remote, true))
	observe(t, s)
	remote.resultPending = true
	if s.Observe(ctx) == nil || read(t, s, r.ID).Status() != "not_ready" {
		t.Fatal("pending seal admitted")
	}
	remote.resultPending = false
	observe(t, s)
	if remote.submits != 1 || remote.stops != 1 || remote.resultReads != 2 || read(t, s, r.ID).Status() != "ready" {
		t.Fatal("pending query repeated execution or lost original result")
	}
}

func TestProjectEnvironmentExecutionFailedSealIsPermanentAcrossRestart(t *testing.T) {
	s, remote, store, now := setup(t)
	r := start(t, s)
	observe(t, s)
	setReport(remote, report(r, remote, true))
	remote.resultDiagnostic = "file_missing"
	observe(t, s)
	observe(t, s)
	r = read(t, s, r.ID)
	if r.Status() != "not_ready" || r.State != "unverified" || r.Rounds[0].Report != nil || !strings.Contains(r.Diagnostic, "file_missing") {
		t.Fatal("failed seal lost its diagnostic or admitted", r)
	}
	s = &env.Service{Project: s.Project, Store: store, Runner: remote, Prepare: s.Prepare, Now: func() time.Time { return *now }}
	remote.resultDiagnostic = "" // Even a later different peer response cannot refresh a seal.
	observe(t, s)
	if remote.resultReads != 1 || remote.submits != 1 || read(t, s, r.ID).Status() != "not_ready" {
		t.Fatal("sealed failure refreshed or caused an unsafe new Run")
	}
}

func TestProjectEnvironmentExecutionUnavailableResultCannotBypassDeadline(t *testing.T) {
	s, remote, _, now := setup(t)
	r := start(t, s)
	observe(t, s)
	observe(t, s)
	remote.resultErr = true
	*now = now.Add(time.Minute)
	observe(t, s)
	if remote.stops != 1 || remote.resultReads != 0 || read(t, s, r.ID).Status() != "not_ready" {
		t.Fatal("invalid file bypassed original Run timeout")
	}
}

func TestProjectEnvironmentExecutionStaleRunCannotBypassDeadline(t *testing.T) {
	s, remote, _, now := setup(t)
	r := start(t, s)
	observe(t, s)
	remote.run.Revision = 5
	observe(t, s)
	remote.inspectRevision = 1
	*now = now.Add(time.Minute)
	observe(t, s)
	if remote.stops != 1 || read(t, s, r.ID).Status() != "not_ready" {
		t.Fatal("stale Run suspended the original deadline")
	}
}

func TestProjectEnvironmentExecutionReadOutageCannotSuspendOriginalStop(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			s, remote, _, now := setup(t)
			r := start(t, s)
			observe(t, s)
			observe(t, s)
			if cancelled {
				r = read(t, s, r.ID)
				_, e := s.Stop(ctx, r.ID, envmodel.ID(), r.Revision, json.RawMessage(`{}`))
				if e != nil {
					t.Fatal(e)
				}
			} else {
				*now = now.Add(time.Minute)
			}
			remote.inspectErr = true
			observe(t, s)
			if remote.stops != 1 || remote.submits != 1 || read(t, s, r.ID).Status() != "not_ready" {
				t.Fatal("read outage erased original cleanup obligation")
			}
		})
	}
}
func TestProjectEnvironmentExecutionReportStrictness(t *testing.T) {
	s, remote, _, _ := setup(t)
	r := start(t, s)
	observe(t, s)
	good := report(r, remote, true)
	if !good.Passed("validation", remote.run.Execution.Branch) {
		t.Fatal("complete report rejected")
	}
	httpReport := good
	httpReport.PR.URL = "http://gitea.local/pulls/1"
	if !httpReport.Passed("validation", remote.run.Execution.Branch) {
		t.Fatal("configured intranet HTTP PR rejected")
	}
	badURL := good
	badURL.PR.URL = "https://"
	if badURL.Passed("validation", remote.run.Execution.Branch) {
		t.Fatal("PR URL without host accepted")
	}
	for _, change := range []func(*envmodel.Report){func(p *envmodel.Report) { p.PR.ChecksPassed = false }, func(p *envmodel.Report) { p.PR.ReviewsPassed = false }, func(p *envmodel.Report) { p.PR.Merged = false }, func(p *envmodel.Report) { p.PR.CleanupCompleted = false }, func(p *envmodel.Report) { p.PR.OutcomeUnknown = true }, func(p *envmodel.Report) { p.PR.Head = "other" }, func(p *envmodel.Report) { p.PR.Base = "main" }, func(p *envmodel.Report) { p.PR.MergeCommit = "missing" }} {
		p := good
		change(&p)
		if p.Passed("validation", remote.run.Execution.Branch) {
			t.Fatal("incomplete PR admitted")
		}
	}
	for _, change := range []func(*envmodel.Report){func(p *envmodel.Report) { p.Round++ }, func(p *envmodel.Report) { p.ScriptDigest = "other" }, func(p *envmodel.Report) { p.L2 = p.L2[:1] }, func(p *envmodel.Report) { p.L2[1].ID = "one" }} {
		p := good
		p.L2 = append([]envmodel.Item{}, good.L2...)
		change(&p)
		b, _ := json.Marshal(p)
		if _, e := envmodel.ParseReport(b, r, r.Rounds[0]); e == nil {
			t.Fatal("invalid report accepted", string(b))
		}
	}
	remote.body = []byte(`{"version":1`)
	remote.run.StopStatus = "stopped"
	observe(t, s)
	if read(t, s, r.ID).State != "unverified" {
		t.Fatal("partial JSON released")
	}
}
func TestProjectEnvironmentAdmissionImmutableOriginalAndDeadline(t *testing.T) {
	s, remote, store, now := setup(t)
	r := start(t, s)
	if _, e := s.Start(ctx, r.ID, json.RawMessage(`{"original":false}`), envmodel.Start{Profile: "remote", TimeoutSeconds: 60}, false); e == nil {
		t.Fatal("changed original accepted")
	}
	altered := r
	altered.Revision++
	altered.Rounds = append([]envmodel.Round{}, r.Rounds...)
	altered.Rounds[0].Deadline = altered.Rounds[0].Deadline.Add(time.Second)
	if store.SaveEnvironment(ctx, altered, r.Revision) == nil {
		t.Fatal("deadline mutation persisted")
	}
	*now = now.Add(2 * time.Minute)
	observe(t, s)
	r = read(t, s, r.ID)
	if remote.submits != 0 || r.State != "failed" {
		t.Fatal("expired queued execution submitted")
	}
}

func TestProjectEnvironmentRetryTimeoutInput(t *testing.T) {
	s, _, _, now := setup(t)
	for _, seconds := range []int{0, -1, int(int64(^uint64(0) >> 1))} {
		if _, e := s.Start(ctx, envmodel.ID(), json.RawMessage(`{}`), envmodel.Start{Profile: "remote", TimeoutSeconds: seconds}, false); e == nil {
			t.Fatal("invalid/overflowing timeout accepted", seconds)
		}
	}
	r, e := s.Start(ctx, envmodel.ID(), json.RawMessage(`{}`), envmodel.Start{Profile: "remote", TimeoutSeconds: 90000}, false)
	if e != nil {
		t.Fatal("arbitrary one-day limit", e)
	}
	if !r.Rounds[0].Deadline.Equal(now.Add(90000 * time.Second)) {
		t.Fatal("timeout not frozen as requested")
	}
}
func TestProjectEnvironmentExecutionCancellationDoesNotDispatch(t *testing.T) {
	s, remote, _, _ := setup(t)
	r := start(t, s)
	_, e := s.Stop(ctx, r.ID, envmodel.ID(), r.Revision, json.RawMessage(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	observe(t, s)
	if remote.submits != 0 || !read(t, s, r.ID).Settled() {
		t.Fatal("unsent cancellation dispatched")
	}
}

func TestProjectEnvironmentExecutionStopUnknownAndKnownRejection(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprint(unknown), func(t *testing.T) {
			s, remote, _, _ := setup(t)
			r := start(t, s)
			observe(t, s)
			setReport(remote, report(r, remote, true))
			remote.stopReject = !unknown
			remote.stopUnknown = unknown
			if s.Observe(ctx) == nil {
				t.Fatal("stop failure ignored")
			}
			original := read(t, s, r.ID).Rounds[0].Stop.RequestID
			if unknown {
				for i := 0; i < 2; i++ {
					observe(t, s)
				}
				if remote.stops != 1 || read(t, s, r.ID).Rounds[0].Stop.RequestID != original {
					t.Fatal("unknown stop resent")
				}
			} else {
				observe(t, s)
				observe(t, s)
				r = read(t, s, r.ID)
				if remote.stops != 2 || len(r.Rounds[0].RejectedStops) != 1 || r.Rounds[0].RejectedStops[0].RequestID != original || r.Status() != "ready" {
					t.Fatal("known rejected stop could not recover", r.State)
				}
			}
		})
	}
}
func TestProjectEnvironmentExecutionTimeoutAndCancelDuringPrepare(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprint(cancel), func(t *testing.T) {
			s, remote, _, now := setup(t)
			r := start(t, s)
			observe(t, s)
			remote.run.Phase = "implement"
			remote.resultDiagnostic = "file_missing"
			if cancel {
				r = read(t, s, r.ID)
				_, e := s.Stop(ctx, r.ID, envmodel.ID(), r.Revision, json.RawMessage(`{}`))
				if e != nil {
					t.Fatal(e)
				}
			} else {
				*now = now.Add(time.Minute)
			}
			observe(t, s)
			observe(t, s)
			r = read(t, s, r.ID)
			if cancel {
				if !r.Settled() || r.State != "cancelled" || remote.submits != 1 {
					t.Fatal("cancelled prepare could not settle", r.State)
				}
			} else {
				if len(r.Rounds) != 2 || r.Rounds[0].TimedOut != true || r.Status() == "ready" {
					t.Fatal("timeout failed to consume round", r.State)
				}
			}
		})
	}
}
func TestProjectEnvironmentGenerationNativeResultAndQuota(t *testing.T) {
	for _, verified := range []bool{false, true} {
		t.Run(fmt.Sprint(verified), func(t *testing.T) {
			s, remote, _, _ := setup(t)
			s.PrepareGeneration = func(context.Context, string) (runnercontract.GenerateRunInput, error) {
				return runnercontract.GenerateRunInput{Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}}, Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Generate: runnercontract.GenerateInput{Repository: &runnercontract.GenerateRepository{LocalPath: "fixture", Commit: strings.Repeat("a", 40)}, Materials: []runnercontract.Document{}}}, nil
			}
			acquired, released := 0, 0
			s.AcquireGeneration = func(envmodel.Record) bool { acquired++; return true }
			s.ReleaseGeneration = func(envmodel.Record) { released++ }
			r, e := s.Start(ctx, envmodel.ID(), json.RawMessage(`{}`), envmodel.Start{Profile: "local"}, true)
			if e != nil {
				t.Fatal(e)
			}
			observe(t, s)
			generated := envmodel.Generated{Document: "```steerline-environment\n{\"version\":1,\"script\":\"scripts/verify-environment.sh\",\"interpreter\":\"bash\",\"validation_base\":\"validation\",\"dependencies\":[]}\n```", Script: "echo fixture", Gaps: []string{}}
			body, _ := json.Marshal(generated)
			source := &runnercontract.RepositorySource{LocalPath: "fixture", Commit: strings.Repeat("a", 40), Tree: strings.Repeat("b", 40)}
			remote.run.RepositorySource = source
			remote.run.ContentResult = &runnercontract.ContentResult{Status: "succeeded", JSON: body, RepositorySource: source, Source: runnercontract.Final{Verified: verified, NativeOutcome: "succeeded", CommandID: "cmd", TurnID: "turn"}}
			observe(t, s)
			observe(t, s)
			r = read(t, s, r.ID)
			if (r.State == "generated") != verified || r.Status() != "not_ready" || acquired != 1 || released != 1 {
				t.Fatal("generation/source/quota contract", r.State, acquired, released)
			}
		})
	}
}
