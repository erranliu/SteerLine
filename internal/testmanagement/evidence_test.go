package testmanagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/testmanagement/model"
	"sort"
	"testing"
	"time"
)

type evidenceMemory struct {
	candidate runnerstate.Candidate
	record    *model.Evidence
	fail      bool
}

func (m *evidenceMemory) RunnerCandidates(context.Context, string, string) ([]runnerstate.Candidate, error) {
	return []runnerstate.Candidate{m.candidate}, nil
}
func (m *evidenceMemory) Evidence(context.Context, string, string) (model.Evidence, error) {
	if m.record == nil {
		return model.Evidence{}, model.ErrNotFound
	}
	raw, _ := json.Marshal(m.record)
	var r model.Evidence
	_ = json.Unmarshal(raw, &r)
	return r, nil
}
func (m *evidenceMemory) SaveEvidence(_ context.Context, r model.Evidence, expected uint64) error {
	if m.fail {
		return errors.New("write refused")
	}
	if m.record != nil && m.record.Revision != expected {
		return errors.New("CAS")
	}
	raw, _ := json.Marshal(r)
	var v model.Evidence
	_ = json.Unmarshal(raw, &v)
	m.record = &v
	return nil
}

type evidenceCLI struct {
	store         *evidenceMemory
	files         map[string][]byte
	artifacts     []model.EvidenceArtifact
	collects      int
	lost, corrupt bool
	staleOnce     bool
	git           runnerclient.WorkspaceGit
}

func (f *evidenceCLI) DiscoverArtifacts(context.Context, string) (runnerclient.ArtifactDiscovery, error) {
	v := runnerclient.ArtifactDiscovery{RunID: "run", Directory: runnercontract.ArtifactDirectory, Candidates: []runnercontract.ArtifactCandidate{}}
	for p, b := range f.files {
		v.Candidates = append(v.Candidates, runnercontract.ArtifactCandidate{RelativePath: p, SizeBytes: int64(len(b))})
	}
	sort.Slice(v.Candidates, func(i, j int) bool { return v.Candidates[i].RelativePath < v.Candidates[j].RelativePath })
	return v, nil
}
func (f *evidenceCLI) ListArtifacts(context.Context, string) (runnerclient.ArtifactList, error) {
	v := runnerclient.ArtifactList{RunID: "run", Artifacts: []runnerclient.Artifact{}}
	for _, a := range f.artifacts {
		v.Artifacts = append(v.Artifacts, a.Metadata)
	}
	return v, nil
}
func (f *evidenceCLI) ReadArtifact(_ context.Context, _ string, id string) (runnerclient.Artifact, []byte, error) {
	for _, a := range f.artifacts {
		if a.Metadata.ArtifactID == id {
			b := a.Content
			if f.corrupt {
				b = []byte("corrupt")
			}
			return a.Metadata, b, nil
		}
	}
	return runnerclient.Artifact{}, nil, errors.New("missing")
}
func (f *evidenceCLI) CollectArtifact(_ context.Context, _ string, request runnerclient.Request) (runnerclient.Artifact, error) {
	found := false
	for _, op := range f.store.record.Collections {
		if op.Request.RequestID == request.RequestID {
			found = true
		}
	}
	if !found {
		panic("external mutation before persistence")
	}
	var in runnercontract.CollectArtifactInput
	_ = json.Unmarshal(request.Input, &in)
	f.collects++
	if f.staleOnce {
		f.staleOnce = false
		return runnerclient.Artifact{}, &runnerclient.Failure{Code: "stale_revision", Exit: 5}
	}
	body := f.files[in.RelativePath]
	a := runnerclient.Artifact{RunID: "run", ArtifactID: fmt.Sprintf("a%d", f.collects), RelativePath: in.RelativePath, SHA256: ContentDigest(body), SizeBytes: len(body), CollectedAt: time.Now().UTC()}
	f.artifacts = append(f.artifacts, model.EvidenceArtifact{Metadata: a, Content: body})
	if f.lost {
		return a, &runnerclient.Failure{Unknown: true, Code: "request_outcome_unknown"}
	}
	return a, nil
}
func (f *evidenceCLI) WorkspaceGit(context.Context, string) (runnerclient.WorkspaceGit, error) {
	return f.git, nil
}
func (f *evidenceCLI) Inspect(_ context.Context, selector, id string, _ bool) (runnerclient.Inspection, error) {
	v := runnerclient.Inspection{Run: &runnerclient.Run{RunID: "run", Revision: 1, InputDigest: "input"}}
	if selector == "request-id" {
		run := "run"
		v.Command = &runnerclient.Command{RequestID: id, Command: "artifacts collect", RunID: &run, ActionStatus: "completed"}
	}
	return v, nil
}
func evidenceFixture(t *testing.T) (*EvidenceService, *evidenceMemory, *evidenceCLI, Snapshot) {
	t.Helper()
	snap := Snapshot{Status: "ready", Definitions: []Definition{definition()}}
	raw, _ := json.Marshal(snap)
	m := &evidenceMemory{candidate: runnerstate.Candidate{Project: "p", ID: "c", RunID: "run", InputDigest: "input", Participation: "active", TestPlan: raw}}
	commit := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	clean := true
	f := &evidenceCLI{store: m, files: map[string][]byte{}, git: runnerclient.WorkspaceGit{RunID: "run", Commit: &commit, Clean: &clean, Verified: true, ObservedAt: time.Now().UTC()}}
	s := &EvidenceService{Project: "p", Store: m, Runner: f}
	publishEvidence(f, "first", 1, "passed")
	return s, m, f, snap
}
func publishEvidence(f *evidenceCLI, id string, sequence uint64, status string) {
	d := definition()
	summary := []byte(`{"actual_assertions":true}`)
	report := Report{ID: ContentDigest([]byte(id))[:32], StartedAt: time.Now().Add(-time.Second), FinishedAt: time.Now(), Definition: d, Command: d.Command, Repository: "repo", Evidence: model.ReportEvidence{Path: "summary.json", Bytes: len(summary), SHA256: ContentDigest(summary)}, Sequence: sequence, Selection: []string{d.ID}, Environment: d.Environment, Definitions: map[string]string{d.ID: Digest(d)}, BeforeCommit: *f.git.Commit, AfterCommit: *f.git.Commit, BeforeClean: true, AfterClean: true, Results: []Result{{TestID: d.ID, Status: status, Message: "actual assertion", Log: "summary.json"}}}
	raw, _ := json.Marshal(report)
	f.files[id+"/managed-test.json"] = raw
	f.files[id+"/summary.json"] = []byte(`{"actual_assertions":true}`)
}
func TestTMEvidenceOriginalMutationAndLatestResult(t *testing.T) {
	s, m, f, snap := evidenceFixture(t)
	ctx := context.Background()
	if err := s.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	v := EvidenceView(snap, *m.record, time.Now())
	if v.Passed != 1 || f.collects != 2 {
		t.Fatal(v, f.collects)
	}
	_ = s.Observe(ctx)
	if f.collects != 2 {
		t.Fatal("duplicate collection")
	}
	publishEvidence(f, "second", 2, "failed")
	_ = s.Observe(ctx)
	v = EvidenceView(snap, *m.record, time.Now())
	if v.Failed != 1 || v.Passed != 0 || len(v.Rows[0].History) != 2 {
		t.Fatal(v)
	}
	f.git.Verified = false
	f.git.Commit = nil
	f.git.Clean = nil
	_ = s.Observe(ctx)
	v = EvidenceView(snap, *m.record, time.Now())
	if v.Passed != 0 || v.Pending != 1 {
		t.Fatal("unknown source counted", v)
	}
}
func TestTMEvidenceUnknownDoesNotResendAndPersistenceFailure(t *testing.T) {
	s, m, f, snap := evidenceFixture(t)
	ctx := context.Background()
	m.fail = true
	if s.Observe(ctx) == nil || f.collects != 0 {
		t.Fatal("mutation before durable write")
	}
	m.fail = false
	f.lost = true
	_ = s.Observe(ctx)
	_ = s.Observe(ctx)
	if f.collects != 2 || EvidenceView(snap, *m.record, time.Now()).Passed != 1 {
		t.Fatal("unknown collection replayed or not recovered", f.collects, m.record)
	}
}
func TestTMEvidenceBadBytesOrMissingLogCannotPass(t *testing.T) {
	for _, mode := range []string{"bytes", "log", "conflict", "stale", "order", "declared-summary"} {
		t.Run(mode, func(t *testing.T) {
			s, m, f, snap := evidenceFixture(t)
			switch mode {
			case "bytes":
				f.corrupt = true
			case "log":
				delete(f.files, "first/summary.json")
			case "conflict":
				publishEvidence(f, "other", 2, "failed")
				var r Report
				_ = json.Unmarshal(f.files["other/managed-test.json"], &r)
				r.ID = ContentDigest([]byte("first"))[:32]
				f.files["other/managed-test.json"], _ = json.Marshal(r)
			case "declared-summary":
				var r Report
				_ = json.Unmarshal(f.files["first/managed-test.json"], &r)
				r.Evidence.SHA256 = ContentDigest([]byte("different summary"))
				f.files["first/managed-test.json"], _ = json.Marshal(r)
			case "stale":
				f.git.ObservedAt = time.Now().Add(-time.Minute)
			case "order":
				publishEvidence(f, "same-order", 1, "failed")
			}
			_ = s.Observe(context.Background())
			if v := EvidenceView(snap, *m.record, time.Now()); v.Passed != 0 {
				t.Fatal(v)
			}
		})
	}
}

func TestTMEvidenceStaleRevisionPlansFreshRequestOnlyAfterKnownRejection(t *testing.T) {
	s, m, f, snap := evidenceFixture(t)
	f.staleOnce = true
	if err := s.Observe(context.Background()); err != nil {
		t.Fatal(err)
	}
	original := m.record.Collections[0].Request.RequestID
	if m.record.Collections[0].State != "stale" || EvidenceView(snap, *m.record, time.Now()).Passed != 0 {
		t.Fatal(m.record)
	}
	if err := s.Observe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.collects != 3 || m.record.Collections[2].Request.RequestID == original || EvidenceView(snap, *m.record, time.Now()).Passed != 1 {
		t.Fatal(f.collects, m.record)
	}
}

func TestPMSExpiredEvidenceNeverStartsNewCollection(t *testing.T) {
	s, m, f, snap := evidenceFixture(t)
	ctx := context.Background()
	if e := s.Observe(ctx); e != nil {
		t.Fatal(e)
	}
	original := f.collects
	s.AllowCollect = func(context.Context, runnerstate.Candidate) bool { return false }
	publishEvidence(f, "late", 2, "passed")
	if e := s.Observe(ctx); e != nil {
		t.Fatal(e)
	}
	v := EvidenceView(snap, *m.record, time.Now())
	if f.collects != original || v.Passed != 0 || len(v.Rows[0].History) != 1 {
		t.Fatal("expired evidence started collection or counted current pass", v, f.collects)
	}
}
