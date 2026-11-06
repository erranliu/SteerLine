package environmentadaptation_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	nativefixture "github.com/erranliu/SteerLine/internal/testkit/protocol"
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
	submitFailure    error
	inspectFailure   error
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
		if r.submitFailure != nil {
			return runnerclient.Response{}, r.submitFailure
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
		if r.run.Status != "completed" && r.run.Status != "failed" {
			r.run.Status = "stopped"
		}
		r.run.Revision++
		if r.stopBody != nil {
			r.body = r.stopBody
		}
	}
	return runnerclient.Response{}, nil
}
func (r *runner) Inspect(context.Context, string, string) (runnerclient.Inspection, error) {
	if r.inspectFailure != nil {
		return runnerclient.Inspection{}, r.inspectFailure
	}
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
	if e == nil {
		store.BindExecutionIdentityLookup(func(execution runnercontract.ExecutionInput) (runnercontract.ExecutionIdentity, error) {
			return nativefixture.ExecutionIdentity(execution), nil
		})
	}
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	remote := &runner{}
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	service := &env.Service{Project: "project", Store: store, Runner: remote, Now: func() time.Time { return now }, Prepare: func(context.Context, string) (envmodel.Prepared, error) {
		prepared := envmodel.Prepared{Commit: strings.Repeat("a", 40), Inputs: "basis", ScriptDigest: strings.Repeat("b", 64), Manifest: envmodel.Manifest{Version: 5, Script: envmodel.ScriptPath("ps1"), Interpreter: "powershell.exe", Dependencies: []string{}}, Input: runnercontract.RunInput{Workflow: runnercontract.Workflow{ID: "execute", Version: 1}, Execution: runnercontract.ExecutionInput{Adapter: "orca-terminal", Target: runnercontract.ExecutionTarget{Kind: "orca_environment", Environment: "remote"}, Settings: json.RawMessage(`{"access_mode":"full_access"}`)}}}
		prepared.Identity = nativefixture.ExecutionIdentity(prepared.Input.Execution)
		prepared.DerivedExecution = prepared.Input.Execution
		return prepared, nil
	}}
	return service, remote, store, &now
}
func start(t *testing.T, s *env.Service) envmodel.Record {
	t.Helper()
	id := envmodel.ID()
	r, e := s.Start(ctx, id, json.RawMessage(`{"original":true}`), envmodel.Start{EnvironmentKey: serviceEnvironmentKey(), Profile: "remote", TimeoutSeconds: 60})
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
	p := envmodel.InitialReport(r, 1)
	p.Completed = true
	p.Ready = pass
	p.Message = "project detection completed"
	if !pass {
		p.Message = "tool unavailable"
	}
	return p
}
func setReport(remote *runner, p envmodel.Report) {
	remote.body, _ = json.Marshal(p)
	remote.run.Status = "completed"
	remote.run.Final = &runnercontract.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "original-command", SessionID: "original-session", TurnID: "original-turn"}
}
func observe(t *testing.T, s *env.Service) {
	t.Helper()
	if e := s.Observe(ctx); e != nil {
		t.Fatal(e)
	}
}

func TestProjectEnvironmentDetectionUsesFrozenRunnerExecution(t *testing.T) {
	for _, tc := range []struct {
		name, adapter, target, environment, settings, want string
	}{
		{
			name: "default remote Codex", adapter: "orca-terminal", target: "orca_environment", environment: "remote",
			settings: `{"access_mode":"full_access","transport":{"executable":"orca"},"agent_launch":{"model":"selected","reasoning_effort":"high"}}`,
			want:     `{"access_mode":"full_access","transport":{"executable":"orca"},"agent_launch":{"model":"selected","reasoning_effort":"high"}}`,
		},
		{
			name: "explicit local Codex", adapter: "orca-terminal", target: "local",
			settings: `{"agent":"codex","access_mode":"full_access"}`,
			want:     `{"agent":"codex","access_mode":"full_access"}`,
		},
		{
			name: "local Pi", adapter: "orca-terminal", target: "local",
			settings: `{"agent":"pi","access_mode":"full_access","agent_launch":{"model":"selected"}}`,
			want:     `{"agent":"pi","access_mode":"full_access","agent_launch":{"model":"selected"}}`,
		},
		{
			name: "Codex app server", adapter: "codex-app-server", target: "local",
			settings: `{"executable":"custom-codex","access_mode":"full_access"}`,
			want:     `{"executable":"custom-codex","access_mode":"full_access"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _, _ := setup(t)
			prepare := s.Prepare
			execution := runnercontract.ExecutionInput{
				Adapter:  tc.adapter,
				Target:   runnercontract.ExecutionTarget{Kind: tc.target, Environment: tc.environment},
				Settings: json.RawMessage(tc.settings),
			}
			s.Prepare = func(ctx context.Context, profile string) (envmodel.Prepared, error) {
				prepared, err := prepare(ctx, profile)
				prepared.Input.Execution = execution
				prepared.Identity = nativefixture.ExecutionIdentity(execution)
				prepared.DerivedExecution = execution
				return prepared, err
			}
			key, err := envmodel.Key(execution, nativefixture.ExecutionIdentity(execution))
			if err != nil {
				t.Fatal(err)
			}
			id := envmodel.ID()
			_, err = s.Start(ctx, id, json.RawMessage(`{}`), envmodel.Start{
				EnvironmentKey: key, Profile: "selected", TimeoutSeconds: 60,
			})
			if err != nil {
				t.Fatal(err)
			}
			record := read(t, s, id)
			var submitted runnercontract.RunInput
			if err := json.Unmarshal(record.Rounds[0].Request.Input, &submitted); err != nil {
				t.Fatal(err)
			}
			want, err := runnercontract.CanonicalJSON(json.RawMessage(tc.want))
			if err != nil {
				t.Fatal(err)
			}
			got, err := runnercontract.CanonicalJSON(submitted.Execution.Settings)
			if err != nil || string(got) != string(want) || submitted.Execution.Target != execution.Target {
				t.Fatalf("submitted execution = %+v, settings = %s, want %s", submitted.Execution, got, want)
			}
			for _, original := range []runnercontract.ExecutionInput{record.Execution, record.Prepared.Input.Execution} {
				if originalKey, err := envmodel.Key(original, nativefixture.ExecutionIdentity(original)); err != nil || originalKey != key || string(original.Settings) != tc.settings {
					t.Fatalf("stored development execution changed: %s", original.Settings)
				}
			}
		})
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
	// Only the stopped producer's sealed negative result is consumed; no retry.
	if r.Status() == "ready" || r.State != "failed" || !r.Settled() || len(r.Rounds) != 1 {
		t.Fatal("ignored final report", r.State, len(r.Rounds))
	}
}

func TestProjectEnvironmentSingleInvocationSurvivesRestart(t *testing.T) {
	s, remote, store, now := setup(t)
	r := start(t, s)
	observe(t, s)
	setReport(remote, report(r, remote, false))
	observe(t, s)
	s = &env.Service{Project: s.Project, Store: store, Runner: remote, Now: func() time.Time { return *now }, Prepare: s.Prepare}
	observe(t, s)
	r = read(t, s, r.ID)
	if r.State != "failed" || !r.Settled() || len(r.Rounds) != 1 || remote.submits != 1 {
		t.Fatal(r, remote.submits)
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
	if remote.stops != 1 || r.Status() == "ready" || !r.Settled() || r.State != "failed" {
		t.Fatal("known stopped original must settle without qualification", r.State)
	}
	if _, e := s.Start(ctx, envmodel.ID(), json.RawMessage(`{}`), envmodel.Start{EnvironmentKey: serviceEnvironmentKey(), Profile: "remote", TimeoutSeconds: 60, ExpectedCurrentRevision: 1}); e != nil {
		t.Fatal("explicit new detection rejected after original settled", e)
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
	remote.run.Final = nil
	observe(t, s)
	if remote.stops != 0 || remote.resultReads != 0 || read(t, s, r.ID).Status() != "not_ready" {
		t.Fatal("single turn or waiting input prematurely stopped/admitted")
	}
	setReport(remote, report(r, remote, true))
	observe(t, s)
	observe(t, s)
	if remote.stops != 1 || read(t, s, r.ID).Status() != "ready" {
		t.Fatal("completed execution failed to stop and seal")
	}
}

func TestProjectEnvironmentBlockedFinalStopsOnlyWhenVerified(t *testing.T) {
	s, remote, _, _ := setup(t)
	r := start(t, s)
	observe(t, s)
	setReport(remote, report(r, remote, false))
	remote.run.Status = "blocked"
	remote.run.Final = &runnercontract.Final{Verified: false}
	observe(t, s)
	if remote.stops != 0 || remote.resultReads != 0 {
		t.Fatal("unverified blocked turn must remain open")
	}
	remote.run.Final.Verified = true
	observe(t, s)
	observe(t, s)
	got := read(t, s, r.ID)
	if remote.stops != 1 || got.Status() == "ready" || got.Rounds[0].Report == nil || got.Rounds[0].Result.CollectionStatus != "closed" {
		t.Fatal("verified blocked final must stop and seal the original failed round")
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
	if r.Status() != "not_ready" || r.State != "failed" || !r.Settled() || r.Rounds[0].Report != nil || !strings.Contains(r.Diagnostic, "file_missing") {
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
	base := report(r, remote, true)
	a := r.Rounds[0]
	for _, mutate := range []func(*envmodel.Report){func(p *envmodel.Report) { p.Version = 2 }, func(p *envmodel.Report) { p.Project = "other" }, func(p *envmodel.Report) { p.EnvironmentKey = strings.Repeat("c", 64) }, func(p *envmodel.Report) { p.ActivityID = envmodel.ID() }, func(p *envmodel.Report) { p.Round++ }, func(p *envmodel.Report) { p.Commit = strings.Repeat("c", 40) }, func(p *envmodel.Report) { p.Inputs = "different" }, func(p *envmodel.Report) { p.Context.Target.Environment = "other" }, func(p *envmodel.Report) { p.Message = " " }} {
		p := base
		mutate(&p)
		raw, _ := json.Marshal(p)
		if _, e := envmodel.ParseReport(raw, r, a); e == nil {
			t.Fatal("wrong identity accepted", string(raw))
		}
	}
}
func TestProjectEnvironmentAdmissionImmutableOriginalAndDeadline(t *testing.T) {
	s, remote, store, now := setup(t)
	r := start(t, s)
	if _, e := s.Start(ctx, r.ID, json.RawMessage(`{"original":false}`), envmodel.Start{EnvironmentKey: serviceEnvironmentKey(), Profile: "remote", TimeoutSeconds: 60}); e == nil {
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
		if _, e := s.Start(ctx, envmodel.ID(), json.RawMessage(`{}`), envmodel.Start{EnvironmentKey: serviceEnvironmentKey(), Profile: "remote", TimeoutSeconds: seconds}); e == nil {
			t.Fatal("invalid/overflowing timeout accepted", seconds)
		}
	}
	r, e := s.Start(ctx, envmodel.ID(), json.RawMessage(`{}`), envmodel.Start{EnvironmentKey: serviceEnvironmentKey(), Profile: "remote", TimeoutSeconds: 90000})
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
				if len(r.Rounds) != 1 || !r.Settled() || r.Rounds[0].TimedOut != true || r.Status() == "ready" {
					t.Fatal("timeout failed to consume round", r.State)
				}
			}
		})
	}
}

func serviceEnvironmentKey() string {
	key, _ := envmodel.Key(runnercontract.ExecutionInput{Adapter: "orca-terminal", Target: runnercontract.ExecutionTarget{Kind: "orca_environment", Environment: "remote"}, Settings: json.RawMessage(`{"access_mode":"full_access"}`)}, nativefixture.ExecutionIdentity(runnercontract.ExecutionInput{Adapter: "orca-terminal", Target: runnercontract.ExecutionTarget{Kind: "orca_environment", Environment: "remote"}, Settings: json.RawMessage(`{"access_mode":"full_access"}`)}))
	return key
}

func TestProjectEnvironmentConsumesReadyWithoutReviewingBusinessDetails(t *testing.T) {
	for _, ready := range []bool{true, false} {
		t.Run(fmt.Sprint(ready), func(t *testing.T) {
			s, remote, _, _ := setup(t)
			r := start(t, s)
			observe(t, s)
			p := report(r, remote, ready)
			p.Details = json.RawMessage(`{"checks":[{"id":"project-only","status":"failed"}],"pr":{"url":"not-a-platform-requirement"}}`)
			setReport(remote, p)
			observe(t, s)
			observe(t, s)
			got := read(t, s, r.ID)
			if (got.Status() == "ready") != ready || len(got.Rounds) != 1 || remote.submits != 1 || !sameProjectDetails(got.Rounds[0].Report.Details, p.Details) {
				t.Fatal(got)
			}
		})
	}
}

func sameProjectDetails(a, b json.RawMessage) bool {
	ca, _ := runnercontract.CanonicalJSON(a)
	cb, _ := runnercontract.CanonicalJSON(b)
	return string(ca) == string(cb)
}

func TestProjectReadyCannotOverrideFailedNativeRun(t *testing.T) {
	s, remote, _, _ := setup(t)
	r := start(t, s)
	observe(t, s)
	setReport(remote, report(r, remote, true))
	remote.run.Status = "failed"
	observe(t, s)
	observe(t, s)
	saved := read(t, s, r.ID)
	if !saved.Settled() || saved.Status() == "ready" || !saved.Rounds[0].Report.Ready {
		t.Fatal("project ready overrode native failure", saved)
	}
}
