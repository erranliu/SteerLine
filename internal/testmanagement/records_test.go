package testmanagement

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	m "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func trRecord(id, status, parent string) m.TestRecord {
	d := Definition{ID: "rules", Name: "规则", Level: "L1", Environment: "isolated", Command: "test rules", Check: "actual rules pass"}
	stamp := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	r := Report{ID: strings.Repeat(id, 32), StartedAt: stamp, FinishedAt: stamp.Add(time.Second), Definition: d, Command: d.Command, Repository: "repo", Sequence: 1, Selection: []string{d.ID}, Environment: d.Environment, Definitions: map[string]string{d.ID: Digest(d)}, Results: []Result{{TestID: d.ID, Status: status, Message: "actual result", Log: "summary.json"}}, Evidence: m.ReportEvidence{Path: "summary.json", Bytes: 2, SHA256: ContentDigest([]byte("{}"))}}
	if parent != "" {
		r.RerunOf = strings.Repeat(parent, 32)
	}
	return m.TestRecord{Project: "project", Report: r, EvidenceStatus: "available", Source: m.RecordSource{Kind: "script"}}
}

type trProjectionStore struct {
	trMemory
	evidence  m.Evidence
	fail      bool
	projected int
}

func (s *trProjectionStore) PendingTestEvidence(context.Context, string, string, int) ([]m.Evidence, error) {
	return []m.Evidence{s.evidence}, nil
}
func (s *trProjectionStore) MarkTestEvidenceProjected(_ context.Context, _ string, _ string, count int) error {
	s.projected = count
	return nil
}
func (s *trProjectionStore) SaveTestRecord(ctx context.Context, r m.TestRecord) (m.TestRecord, error) {
	if s.fail {
		return r, errors.New("injected index failure")
	}
	return s.trMemory.SaveTestRecord(ctx, r)
}
func TestTRRunnerProjectionRetryUsesRetainedEvidenceOnly(t *testing.T) {
	r := trRecord("a", "passed", "")
	raw, _ := json.Marshal(r.Report)
	artifact := func(name string, b []byte) m.EvidenceArtifact {
		return m.EvidenceArtifact{Metadata: runnercontract.Artifact{RelativePath: name, SizeBytes: len(b), SHA256: ContentDigest(b)}, Content: b}
	}
	store := &trProjectionStore{fail: true, evidence: m.Evidence{Project: "project", CandidateID: "candidate", RunID: "run", TaskID: "task", Artifacts: []m.EvidenceArtifact{artifact("out/managed-rules.json", raw), artifact("out/summary.json", []byte("{}"))}}}
	service := RecordService{Project: "project", Store: store}
	if e := service.ProjectEvidence(context.Background()); e == nil || store.projected != 0 || len(store.rows) != 0 {
		t.Fatal("failed index advanced progress", e, store.projected)
	}
	store.fail = false
	if e := service.ProjectEvidence(context.Background()); e != nil || store.projected != 2 || len(store.rows) != 1 {
		t.Fatal(e, store.projected, store.rows)
	}
	if got := store.rows[0]; got.Source.Kind != "runner" || got.Source.TaskID != "task" || got.Source.CandidateID != "candidate" || got.EvidenceStatus != "available" {
		t.Fatal(got)
	}
	// No Runner client exists in this fixture: offline/abandoned recovery uses
	// only the original durable evidence and never imports independent history.
	if string(store.evidence.Artifacts[0].Content) != string(raw) {
		t.Fatal("original evidence mutated")
	}
}
func trScope(r m.TestRecord, complete bool, ids ...string) m.TestRecord {
	r.Report.Scope = &m.Scope{Complete: complete}
	for _, id := range ids {
		r.Report.Scope.Items = append(r.Report.Scope.Items, m.ScopeResult{ID: id, Status: r.Report.Results[0].Status, Message: "actual"})
	}
	return r
}
func trView(rows []m.TestRecord, id string) RecordView {
	for _, v := range projectRecordViews(rows) {
		if v.ID == strings.Repeat(id, 32) {
			return v
		}
	}
	panic("missing view")
}
func TestTRRetestSingleReportNoStitching(t *testing.T) {
	failed := trScope(trRecord("1", "failed", ""), true, "a", "b")
	first := trScope(trRecord("2", "passed", "1"), false, "a")
	second := trScope(trRecord("3", "passed", "2"), false, "b")
	second.Report.AfterCommit = strings.Repeat("c", 40)
	rows := []m.TestRecord{second, failed, first}
	if got := trView(rows, "1").Retest; got != "partial" {
		t.Fatal("stitched partial reports", got)
	}
	full := trScope(trRecord("4", "passed", "3"), true, "a", "b")
	rows = append(rows, full)
	if got := trView(rows, "1"); got.Retest != "passed" || got.RetestSourceFixed == nil || *got.RetestSourceFixed {
		t.Fatal("dirty/unknown source must be visible without a commit gate", got)
	}
	later := trScope(trRecord("5", "failed", "4"), true, "a", "b")
	rows = append(rows, later)
	if got := trView(rows, "1").Retest; got != "pending" {
		t.Fatal("old pass hid latest failure", got)
	}
	for _, r := range rows {
		if r.Report.ID == failed.Report.ID && r.Report.Results[0].Status != "failed" {
			t.Fatal("original result mutated")
		}
	}
}
func TestTRRetestUnrelatedChangedMissingEvidenceAndFork(t *testing.T) {
	failed := trRecord("1", "failed", "")
	pass := trRecord("2", "passed", "")
	if trView([]m.TestRecord{pass, failed}, "1").Retest != "pending" {
		t.Fatal("unrelated run closed failure")
	}
	pass.Report.RerunOf = failed.Report.ID
	for _, change := range []string{"definition", "environment", "evidence", "incomplete"} {
		next := pass
		switch change {
		case "definition":
			next.Report.Definitions = map[string]string{"rules": "changed"}
		case "environment":
			next.Report.Environment = "live"
		case "evidence":
			next.EvidenceStatus = "unavailable"
		case "incomplete":
			next.Report.Results = []Result{{TestID: "rules", Status: "incomplete"}}
		}
		if trView([]m.TestRecord{failed, next}, "1").Retest == "passed" {
			t.Fatal("invalid closure", change)
		}
	}
	fork := trRecord("3", "passed", "1")
	if trView([]m.TestRecord{failed, pass, fork}, "1").Retest != "multiple" {
		t.Fatal("merged branches")
	}
	// Clock skew does not reorder an explicit chain.
	pass.Report.StartedAt = failed.Report.StartedAt.Add(-time.Hour)
	if trView([]m.TestRecord{failed, pass}, "1").Retest != "passed" {
		t.Fatal("clock changed relation order")
	}
}

type trMemory struct {
	RecordStore
	rows             []m.TestRecord
	historyForbidden bool
}

func (s *trMemory) TestRecord(_ context.Context, project, id string) (m.TestRecord, error) {
	for _, r := range s.rows {
		if r.Project == project && r.Report.ID == id {
			return r, nil
		}
	}
	return m.TestRecord{}, m.ErrNotFound
}

func (s *trMemory) SaveTestRecord(_ context.Context, r m.TestRecord) (m.TestRecord, error) {
	s.rows = append(s.rows, r)
	return r, nil
}
func (s *trMemory) TestRecordHistory(_ context.Context, project, id string, upper int64) ([]m.TestRecord, int64, error) {
	if s.historyForbidden {
		panic("registration must not construct history")
	}
	var rows []m.TestRecord
	for _, r := range s.rows {
		if r.Project == project && (id == "" || id == r.Report.Definition.ID) && (upper == 0 || r.Seq <= upper) {
			rows = append(rows, r)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i].Report, rows[j].Report
		return a.StartedAt.After(b.StartedAt) || a.StartedAt.Equal(b.StartedAt) && a.ID > b.ID
	})
	if upper == 0 {
		upper = int64(len(s.rows))
	}
	return rows, upper, nil
}

func TestTRRegisterReconcileChecksEvidenceWithoutHistory(t *testing.T) {
	r := trRecord("a", "failed", "")
	raw, _ := json.Marshal(r.Report)
	store := &trMemory{historyForbidden: true}
	service := RecordService{Project: "project", Store: store}
	ctx := context.Background()
	input := m.RegisterRecord{Report: raw, Summary: []byte("{}")}
	if _, err := service.Register(ctx, input, m.RecordSource{}); err != nil {
		t.Fatal(err)
	}
	input.Report = []byte(strings.Replace(string(raw), `2026-10-21T00:00:00Z`, `2026-10-21T00:00:00.0000000Z`, 1))
	if got, err := service.ReconcileRegistration(ctx, input); err != nil || got.Status != "applied" {
		t.Fatal(got, err)
	}
	input.Summary = []byte("XX")
	if _, err := service.ReconcileRegistration(ctx, input); !errors.Is(err, ErrInvalidRecord) {
		t.Fatal("wrong summary reconciled as success", err)
	}
	input.Summary = []byte("{}")
	input.Report = []byte(strings.TrimSuffix(string(raw), "}") + `,"unknown":true}`)
	if _, err := service.ReconcileRegistration(ctx, input); !errors.Is(err, ErrInvalidRecord) {
		t.Fatal("unknown report field ignored", err)
	}
	changed := r.Report
	changed.Command = "different"
	input.Report, _ = json.Marshal(changed)
	if _, err := service.ReconcileRegistration(ctx, input); !errors.Is(err, ErrRecordConflict) {
		t.Fatal("changed identity matched", err)
	}
	if len(store.rows) != 1 {
		t.Fatal("receipt lookup wrote records")
	}
}

func TestTRRegisterJournalKeepsUnknownAttemptAndPersistsBeforeSend(t *testing.T) {
	dir := t.TempDir()
	record := trRecord("a", "failed", "")
	raw, _ := json.Marshal(record.Report)
	items := []RegistrationFile{{ID: record.Report.ID, Path: "managed-rules.json", Input: m.RegisterRecord{Report: raw}}}
	calls := 0
	send := func(_ m.RegisterRecord, request string) RegistrationResult {
		calls++
		body, err := os.ReadFile(filepath.Join(dir, "registration.json"))
		if err != nil {
			t.Fatal(err)
		}
		var receipts []FileRegistrationReceipt
		if err = Decode(body, &receipts); err != nil {
			t.Fatal(err)
		}
		last := receipts[len(receipts)-1]
		if last.RequestID != request || last.Status != "unknown" {
			t.Fatal("sent before durable request", last)
		}
		if calls == 1 {
			return RegistrationResult{Status: "unknown", Diagnostic: "connection lost"}
		}
		return RegistrationResult{Status: "registered", Result: json.RawMessage(`{"id":"` + record.Report.ID + `"}`)}
	}
	first, err := RegisterFiles(dir, items, send)
	if err != nil || first[0].Status != "unknown" {
		t.Fatal(first, err)
	}
	second, err := RegisterFiles(dir, items, send)
	if err != nil || len(second) != 1 || second[0].Status != "registered" || second[0].RequestID == first[0].RequestID {
		t.Fatal(second, err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "registration.json"))
	var receipts []FileRegistrationReceipt
	_ = Decode(body, &receipts)
	if len(receipts) != 2 || receipts[0].RequestID != first[0].RequestID || receipts[0].Status != "unknown" {
		t.Fatal("explicit replay erased original request", receipts)
	}
	items[0].Error = errors.New("invalid local file")
	rejected, err := RegisterFiles(dir, items, send)
	if err != nil || rejected[0].Path != "managed-rules.json" || rejected[0].Status != "rejected" || calls != 2 {
		t.Fatal(rejected, err, calls)
	}
	_ = os.WriteFile(filepath.Join(dir, "registration.json"), []byte("invalid journal"), 0600)
	if _, err = RegisterFiles(dir, items, send); err == nil || calls != 2 {
		t.Fatal("unreadable journal allowed submission", err, calls)
	}
}

type trBatchStore struct {
	trMemory
	pending []m.Evidence
	marked  map[string]int
}

func (s *trBatchStore) PendingTestEvidence(context.Context, string, string, int) ([]m.Evidence, error) {
	return s.pending, nil
}
func (s *trBatchStore) MarkTestEvidenceProjected(_ context.Context, _, id string, count int) error {
	s.marked[id] = count
	return nil
}
func (s *trBatchStore) SaveTestRecord(ctx context.Context, r m.TestRecord) (m.TestRecord, error) {
	if r.Report.RerunOf != "" {
		if _, err := s.TestRecord(ctx, r.Project, r.Report.RerunOf); err != nil {
			return r, errors.New("parent not registered")
		}
	}
	return s.trMemory.SaveTestRecord(ctx, r)
}
func TestTRRunnerUnorderedReportsAndIndependentCandidateProgress(t *testing.T) {
	artifact := func(name string, b []byte) m.EvidenceArtifact {
		return m.EvidenceArtifact{Metadata: runnercontract.Artifact{RelativePath: name, SizeBytes: len(b), SHA256: ContentDigest(b)}, Content: b}
	}
	parent := trRecord("a", "failed", "")
	child := trRecord("b", "passed", "a")
	a, _ := json.Marshal(parent.Report)
	b, _ := json.Marshal(child.Report)
	bad := m.Evidence{CandidateID: "bad", Artifacts: []m.EvidenceArtifact{artifact("out/managed-invalid.json", []byte("invalid"))}}
	valid := m.Evidence{CandidateID: "valid", Artifacts: []m.EvidenceArtifact{artifact("a/managed-child.json", b), artifact("a/summary.json", []byte("{}")), artifact("z/managed-parent.json", a), artifact("z/summary.json", []byte("{}"))}}
	store := &trBatchStore{pending: []m.Evidence{bad, valid}, marked: map[string]int{}}
	service := RecordService{Project: "project", Store: store}
	if err := service.ProjectEvidence(context.Background()); err == nil {
		t.Fatal("bad candidate lost diagnostic")
	}
	if len(store.rows) != 2 || store.rows[0].Report.ID != parent.Report.ID || store.rows[1].Report.ID != child.Report.ID || store.marked["valid"] != 4 || store.marked["bad"] != 0 {
		t.Fatal(store.rows, store.marked)
	}
	// An artifact-only cycle cannot hang the observer or mark itself projected.
	parent.Report.RerunOf = child.Report.ID
	a, _ = json.Marshal(parent.Report)
	store.pending = []m.Evidence{{CandidateID: "cycle", Artifacts: []m.EvidenceArtifact{artifact("a/managed-child.json", b), artifact("z/managed-parent.json", a)}}}
	if err := service.ProjectEvidence(context.Background()); err == nil || store.marked["cycle"] != 0 {
		t.Fatal("cycle accepted", err)
	}
}

func TestTRRetestEnvironmentChangeTakesPrecedenceOverDefinitionDigest(t *testing.T) {
	original := trRecord("a", "failed", "")
	next := trRecord("b", "passed", "a")
	next.Report.Environment = "controlled"
	next.Report.Definition.Environment = "controlled"
	next.Report.Definitions = map[string]string{next.Report.Definition.ID: Digest(next.Report.Definition)}
	if got := retestAgainst(original, next); got != "environment_changed" {
		t.Fatal(got)
	}
}
func TestTRQueryFiltersCursorAndFullRelations(t *testing.T) {
	fail := trRecord("1", "failed", "")
	pass := trRecord("2", "passed", "1")
	fail.Seq = 1
	pass.Seq = 2
	store := &trMemory{rows: []m.TestRecord{fail, pass}}
	s := RecordService{Project: "project", Store: store}
	ctx := context.Background()
	p, e := s.List(ctx, m.RecordFilter{Limit: 1})
	if e != nil || len(p.Records) != 1 || p.Cursor == "" || p.Summary.Total != 2 {
		t.Fatal(p, e)
	}
	fresh := trRecord("3", "failed", "")
	fresh.Seq = 3
	fresh.Report.StartedAt = fresh.Report.StartedAt.Add(time.Hour)
	store.rows = append(store.rows, fresh)
	next, e := s.List(ctx, m.RecordFilter{Limit: 1, Cursor: p.Cursor})
	if e != nil || len(next.Records) != 1 || next.Records[0].ID != fail.Report.ID || next.Records[0].Retest != "passed" || next.Upper != 2 {
		t.Fatal("cursor lost boundary/full relation", next, e)
	}
	if _, e = s.List(ctx, m.RecordFilter{Limit: 1, Cursor: p.Cursor, Environment: "other"}); e == nil {
		t.Fatal("foreign filter cursor accepted")
	}
	other := RecordService{Project: "other", Store: store}
	if _, e = other.List(ctx, m.RecordFilter{Limit: 1, Cursor: p.Cursor}); e == nil {
		t.Fatal("foreign project cursor accepted")
	}
	p, e = s.List(ctx, m.RecordFilter{Outcome: "failed", Retest: "passed"})
	if e != nil || len(p.Records) != 1 || p.Summary.Total != 3 {
		t.Fatal("outcome/retest filter altered summary or relation", p, e)
	}
	if _, e = s.List(ctx, m.RecordFilter{From: "2026-10-22T00:00:00Z", To: "2026-10-21T00:00:00Z"}); e == nil {
		t.Fatal("invalid time interval")
	}
}
func TestTRRegisterValidationAndUnavailableSummary(t *testing.T) {
	r := trRecord("1", "failed", "")
	raw, _ := json.Marshal(r.Report)
	store := &trMemory{}
	s := RecordService{Project: "project", Store: store}
	ctx := context.Background()
	v, e := s.Register(ctx, m.RegisterRecord{Report: raw}, m.RecordSource{})
	if e != nil || v.EvidenceStatus != "unavailable" || v.Report.Results[0].Status != "failed" {
		t.Fatal(v, e)
	}
	if _, e = s.Register(ctx, m.RegisterRecord{Report: raw, Summary: []byte("XX")}, m.RecordSource{}); e == nil {
		t.Fatal("wrong digest accepted")
	}
	if _, e = s.Register(ctx, m.RegisterRecord{Report: raw, Summary: []byte("{}")}, m.RecordSource{}); e != nil {
		t.Fatal(e)
	}
	r.Report.Scope = &m.Scope{Items: []m.ScopeResult{{ID: "same", Status: "passed"}, {ID: "same", Status: "passed"}}}
	raw, _ = json.Marshal(r.Report)
	if _, e = ParseReport(raw); e == nil {
		t.Fatal("duplicate/mismatched scope")
	}
}
func TestTRRegisterDirectoryBoundsAndPaths(t *testing.T) {
	dir := t.TempDir()
	r := trRecord("1", "failed", "")
	raw, _ := json.Marshal(r.Report)
	if e := os.WriteFile(filepath.Join(dir, "managed-rules.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	items, e := ReadRegistrationDirectory(dir)
	if e != nil || len(items) != 1 || items[0].Error != nil || items[0].Input.Diagnostic == "" {
		t.Fatal(items, e)
	}
	if e = os.WriteFile(filepath.Join(dir, "summary.json"), []byte("{}"), 0600); e != nil {
		t.Fatal(e)
	}
	items, e = ReadRegistrationDirectory(dir)
	if e != nil || string(items[0].Input.Summary) != "{}" {
		t.Fatal(items, e)
	}
	if e = os.WriteFile(filepath.Join(dir, "summary.json"), make([]byte, m.MaxSummaryBytes+1), 0600); e != nil {
		t.Fatal(e)
	}
	items, e = ReadRegistrationDirectory(dir)
	if e != nil || items[0].Error != nil || items[0].Input.Diagnostic == "" {
		t.Fatal("large evidence lost record", items, e)
	}
	r.Report.Evidence.Path = "../secret"
	raw, _ = json.Marshal(r.Report)
	_ = os.WriteFile(filepath.Join(dir, "managed-rules.json"), raw, 0600)
	items, e = ReadRegistrationDirectory(dir)
	if e != nil || items[0].Error == nil {
		t.Fatal("escape accepted", items, e)
	}
	if _, e = ReadRegistrationDirectory("relative"); e == nil {
		t.Fatal("relative root accepted")
	}
	// A hard link is a plain file according to Lstat; it still must not read
	// an external file through the explicitly supplied evidence directory.
	r.Report.Evidence.Path = "summary.json"
	raw, _ = json.Marshal(r.Report)
	_ = os.WriteFile(filepath.Join(dir, "managed-rules.json"), raw, 0600)
	_ = os.Remove(filepath.Join(dir, "summary.json"))
	external := filepath.Join(t.TempDir(), "original.json")
	if e = os.WriteFile(external, []byte("{}"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Link(external, filepath.Join(dir, "summary.json")); e != nil {
		t.Fatal(e)
	}
	items, e = ReadRegistrationDirectory(dir)
	if e != nil || items[0].Error == nil {
		t.Fatal("linked evidence accepted", items, e)
	}
}
