package testmanagement

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/testmanagement/model"
	"testing"
	"time"
)

type progressMemory struct {
	ProgressStore
	candidates []runnerstate.Candidate
	rows       map[string]model.Progress
}

func (m *progressMemory) RunnerCandidates(context.Context, string, string) ([]runnerstate.Candidate, error) {
	return m.candidates, nil
}
func (m *progressMemory) Progress(_ context.Context, _, id string) (model.Progress, error) {
	if r, ok := m.rows[id]; ok {
		return r, nil
	}
	return model.Progress{}, model.ErrNotFound
}
func (m *progressMemory) SaveProgress(_ context.Context, r model.Progress, version uint64) error {
	if m.rows[r.CandidateID].Revision != version {
		return errors.New("CAS")
	}
	m.rows[r.CandidateID] = r
	return nil
}

type progressFixture struct {
	values   map[string]runnercontract.ProgressFile
	failures map[string]error
	calls    int
}

func (f *progressFixture) ProgressFile(_ context.Context, id string) (runnercontract.ProgressFile, error) {
	f.calls++
	return f.values[id], f.failures[id]
}
func progressSample(run, body string, version uint64) runnercontract.ProgressFile {
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	return runnercontract.ProgressFile{Contract: runnercontract.ProgressContract, RunID: run, Version: version, ObservedAt: &now, Encoding: "base64", ContentBase64: &encoded, SizeBytes: len(body), SHA256: ContentDigest([]byte(body)), CollectionStatus: "observing"}
}

func TestProgressStrictMapAndFrozenTargets(t *testing.T) {
	s := Snapshot{Status: "ready", Definitions: []Definition{{ID: "one"}, {ID: "two"}}}
	i, err := InitialProgress(s)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(i.InitialContentBase64)
	fields, err := ParseProgress(raw, s)
	if err != nil || fields["test.one"] || len(fields) != 2 {
		t.Fatal(fields, err)
	}
	for _, raw := range []string{`null`, `[]`, `{"test.one":true}`, `{"test.one":null,"test.two":false}`, `{"test.one":1,"test.two":false}`, `{"test.one":true,"test.one":false,"test.two":true}`, `{"test.one":true,"test.two":false,"plan.extra":true}`} {
		if _, err := ParseProgress([]byte(raw), s); err == nil {
			t.Fatal("invalid progress accepted", raw)
		}
	}
}

func TestProgressObservationIsolationRecoveryAndRerun(t *testing.T) {
	ctx := context.Background()
	plan := Snapshot{Status: "ready", Definitions: []Definition{{ID: "one"}}}
	raw, _ := json.Marshal(plan)
	m := &progressMemory{rows: map[string]model.Progress{}, candidates: []runnerstate.Candidate{{Project: "p", ID: "a", RunID: "run-a", InputDigest: "input-a", TestPlan: raw}, {Project: "p", ID: "b", RunID: "run-b", InputDigest: "input-b", TestPlan: raw}}}
	f := &progressFixture{values: map[string]runnercontract.ProgressFile{"run-a": progressSample("run-a", `{"test.one":true}`, 2), "run-b": progressSample("run-b", `{"test.one":false}`, 1)}, failures: map[string]error{}}
	s := ProgressService{Project: "p", Store: m, Runner: f}
	if err := s.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if ProgressView(plan, m.rows["a"]).Passed != 1 || ProgressView(plan, m.rows["b"]).Passed != 0 {
		t.Fatal("cross Run progress")
	}
	before := m.rows["a"].Revision
	if err := s.Observe(ctx); err != nil || m.rows["a"].Revision != before {
		t.Fatal("unchanged polling writes", err)
	}
	f.failures["run-a"] = errors.New("transport unavailable")
	f.values["run-b"] = progressSample("run-b", `{"test.one":true}`, 2)
	if err := s.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if m.rows["a"].Diagnostic == "" || ProgressView(plan, m.rows["a"]).Passed != 1 || ProgressView(plan, m.rows["b"]).Passed != 1 {
		t.Fatal("one failed query erased or blocked progress")
	}
	delete(f.failures, "run-a")
	f.values["run-a"] = progressSample("run-a", `{"test.one":false}`, 3)
	s = ProgressService{Project: "p", Store: m, Runner: f}
	if err := s.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if m.rows["a"].Diagnostic != "" || ProgressView(plan, m.rows["a"]).Pending != 1 {
		t.Fatal("rerun did not reset original pass")
	}
	f.values["run-a"] = progressSample("run-a", `{"test.one":true}`, 4)
	_ = s.Observe(ctx)
	f.values["run-a"] = progressSample("run-a", `{"test.one":null}`, 5)
	_ = s.Observe(ctx)
	if m.rows["a"].ParsedVersion != 4 || m.rows["a"].Diagnostic == "" || ProgressView(plan, m.rows["a"]).Passed != 1 {
		t.Fatal("invalid content replaced valid map")
	}
	f.values["run-a"] = progressSample("run-a", `{"test.one":false}`, 3)
	_ = s.Observe(ctx)
	if m.rows["a"].Snapshot.Version != 5 || m.rows["a"].ParsedVersion != 4 {
		t.Fatal("late version overwrote progress")
	}
	f.values["run-a"] = progressSample("run-a", `{"test.one":true}`, 6)
	p := f.values["run-a"]
	p.CollectionStatus = "closed"
	f.values["run-a"] = p
	_ = s.Observe(ctx)
	// Resource stop can settle while the business status is still running.
	m.candidates[0].Projection = &runnercontract.Run{Status: "running", StopStatus: "stopped"}
	f.failures["run-a"] = errors.New("file removed after close")
	_ = s.Observe(ctx)
	if m.rows["a"].Diagnostic != "" || m.rows["a"].Snapshot.Version != 6 {
		t.Fatal("closed snapshot re-read")
	}
	m.candidates[0].Projection.Status = "running"
	m.candidates[0].Projection.StopStatus = "active"
	delete(f.failures, "run-a")
	f.values["run-a"] = progressSample("run-a", `{"test.one":false}`, 7)
	_ = s.Observe(ctx)
	if m.rows["a"].ParsedVersion != 7 || ProgressView(plan, m.rows["a"]).Passed != 0 {
		t.Fatal("explicit original Run resume remained frozen")
	}
}
