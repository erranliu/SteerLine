package sqlite

import (
	"context"
	m "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func trStored(id string) m.TestRecord {
	stamp := time.Now().UTC()
	d := m.Definition{ID: "rules", Name: "rules", Level: "L1", Environment: "isolated", Command: "test", Check: "assert"}
	return m.TestRecord{Project: "project", Report: m.Report{ID: strings.Repeat(id, 32), Selection: []string{"rules"}, Definition: d, StartedAt: stamp, FinishedAt: stamp, Results: []m.Result{{TestID: "rules", Status: "failed", Message: "actual", Log: "summary.json"}}}, Source: m.RecordSource{Kind: "script"}, EvidenceStatus: "unavailable", Diagnostic: "not uploaded", ObservedAt: stamp}
}
func TestTRRegisterAtomicIdempotenceAndProjectParent(t *testing.T) {
	s, e := OpenSQLiteStore(filepath.Join(t.TempDir(), "records.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	r := trStored("1")
	first, e := s.SaveTestRecord(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.SaveTestRecord(ctx, r)
	if e != nil || again.Seq != first.Seq {
		t.Fatal("duplicate record", again, e)
	}
	changed := r
	changed.Report.Command = "changed"
	if _, e = s.SaveTestRecord(ctx, changed); e == nil {
		t.Fatal("immutable report overwritten")
	}
	child := trStored("2")
	child.Report.RerunOf = r.Report.ID
	child.Project = "other"
	if _, e = s.SaveTestRecord(ctx, child); e == nil {
		t.Fatal("cross-project parent accepted")
	}
	child.Project = r.Project
	child.Report.Definition.ID = "another"
	if _, e = s.SaveTestRecord(ctx, child); e == nil {
		t.Fatal("cross-set parent accepted")
	}
	child.Report.Definition.ID = "rules"
	if _, e = s.SaveTestRecord(ctx, child); e != nil {
		t.Fatal(e)
	}
	r.Summary = []byte("evidence")
	r.EvidenceStatus = "available"
	r.Diagnostic = ""
	if _, e = s.SaveTestRecord(ctx, r); e != nil {
		t.Fatal(e)
	}
	r.Summary = nil
	r.EvidenceStatus = "unavailable"
	r.Diagnostic = "late no evidence"
	if _, e = s.SaveTestRecord(ctx, r); e != nil {
		t.Fatal(e)
	}
	got, e := s.TestRecord(ctx, "project", r.Report.ID)
	if e != nil || got.EvidenceStatus != "available" || string(got.Summary) != "evidence" {
		t.Fatal("late unavailable removed evidence", got, e)
	}
	rows, upper, e := s.TestRecordHistory(ctx, "project", "rules", 0)
	if e != nil || len(rows) != 2 || upper < 1 {
		t.Fatal(rows, upper, e)
	}
	if _, e = s.TestRecord(ctx, "other", r.Report.ID); e == nil {
		t.Fatal("foreign project read")
	}
	r.Source = m.RecordSource{Kind: "runner", TaskID: "task", CandidateID: "candidate", RunID: "run"}
	if _, e = s.SaveTestRecord(ctx, r); e != nil {
		t.Fatal(e)
	}
	r.Source.Kind = "script"
	if _, e = s.SaveTestRecord(ctx, r); e != nil {
		t.Fatal(e)
	}
	got, e = s.TestRecord(ctx, r.Project, r.Report.ID)
	if e != nil || got.Source.Kind != "runner" || got.Source.TaskID != "task" {
		t.Fatal("script replay erased verified origin", got, e)
	}
	r.Source = m.RecordSource{Kind: "runner", CandidateID: "other", RunID: "other"}
	if _, e = s.SaveTestRecord(ctx, r); e == nil {
		t.Fatal("conflicting verified Runner binding accepted")
	}
}

func TestTRQueryNewestDefinitionAndFractionalTimeOrdering(t *testing.T) {
	s, e := OpenSQLiteStore(filepath.Join(t.TempDir(), "records.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	first := trStored("f")
	first.Report.StartedAt = time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	if _, e = s.SaveTestRecord(ctx, first); e != nil {
		t.Fatal(e)
	}
	later := trStored("a")
	later.Report.StartedAt = first.Report.StartedAt.Add(time.Millisecond)
	later.Report.Definition.Name = "Latest definition"
	if _, e = s.SaveTestRecord(ctx, later); e != nil {
		t.Fatal(e)
	}
	rows, _, e := s.TestRecordHistory(ctx, "project", "rules", 0)
	if e != nil || len(rows) != 2 || rows[0].Report.ID != later.Report.ID {
		t.Fatal("fractional seconds sorted lexically", rows, e)
	}
	definitions, e := s.TestRecordDefinitions(ctx, "project")
	if e != nil || len(definitions) != 1 || definitions[0].Name != "Latest definition" {
		t.Fatal(definitions, e)
	}
}
func TestTRRunnerStoredProjectionSurvivesCandidateAbandonment(t *testing.T) {
	s, project, tasks := nativeAuditStore(t)
	ctx := context.Background()
	body := []byte(`{"run_id":"run","input_digest":"input","participation":"abandoned"}`)
	_, e := s.db.ExecContext(ctx, `INSERT INTO runner_candidates(project_identity,candidate_id,task_id,revision,request_id,run_id,payload) VALUES(?,?,?,1,?,?,?)`, project, "c", tasks[0].TaskID, "request", "run", body)
	if e != nil {
		t.Fatal(e)
	}
	ev := m.Evidence{Project: project, CandidateID: "c", RunID: "run", InputDigest: "input", Revision: 1, Artifacts: []m.EvidenceArtifact{{Content: []byte("original")}}}
	if e = s.SaveEvidence(ctx, ev, 0); e != nil {
		t.Fatal(e)
	}
	rows, e := s.PendingTestEvidence(ctx, project, "", 20)
	if e != nil || len(rows) != 1 || rows[0].TaskID != tasks[0].TaskID {
		t.Fatal(rows, e)
	}
	if e = s.MarkTestEvidenceProjected(ctx, project, "c", 1); e != nil {
		t.Fatal(e)
	}
	ev.Revision++
	ev.Git.Diagnostic = "Git unavailable"
	if e = s.SaveEvidence(ctx, ev, 1); e != nil {
		t.Fatal(e)
	}
	rows, e = s.PendingTestEvidence(ctx, project, "", 20)
	if e != nil || len(rows) != 0 {
		t.Fatal("Git refresh replayed artifacts", rows, e)
	}
	ev.Revision++
	ev.Artifacts = append(ev.Artifacts, m.EvidenceArtifact{Content: []byte("new")})
	if e = s.SaveEvidence(ctx, ev, 2); e != nil {
		t.Fatal(e)
	}
	rows, e = s.PendingTestEvidence(ctx, project, "", 20)
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	// Rotating the bounded traversal must move past a candidate that cannot
	// be marked complete, and then wrap to retry it after the rest of the set.
	body = []byte(`{"run_id":"run2","input_digest":"input2","participation":"abandoned"}`)
	_, e = s.db.ExecContext(ctx, `INSERT INTO runner_candidates(project_identity,candidate_id,task_id,revision,request_id,run_id,payload) VALUES(?,?,?,1,?,?,?)`, project, "d", tasks[0].TaskID, "request2", "run2", body)
	if e != nil {
		t.Fatal(e)
	}
	other := m.Evidence{Project: project, CandidateID: "d", RunID: "run2", InputDigest: "input2", Revision: 1, Artifacts: []m.EvidenceArtifact{{Content: []byte("other")}}}
	if e = s.SaveEvidence(ctx, other, 0); e != nil {
		t.Fatal(e)
	}
	rows, e = s.PendingTestEvidence(ctx, project, "c", 1)
	if e != nil || len(rows) != 1 || rows[0].CandidateID != "d" {
		t.Fatal("bad earlier candidate starved later evidence", rows, e)
	}
	rows, e = s.PendingTestEvidence(ctx, project, "d", 1)
	if e != nil || len(rows) != 1 || rows[0].CandidateID != "c" {
		t.Fatal("traversal did not wrap", rows, e)
	}
}
