package testmanagement

import (
	"encoding/json"
	m "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"strings"
	"testing"
	"time"
)

func historyReport(status string) Report {
	d := Definition{ID: "rules", Name: "Rules", Level: "L1", Environment: "isolated", Command: "test", Check: "actual assertion"}
	at := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	return Report{ID: strings.Repeat("a", 32), Sequence: 1, StartedAt: at, FinishedAt: at.Add(time.Second), Definition: d, Command: "test", Repository: "repo", Environment: d.Environment, Selection: []string{d.ID}, Definitions: map[string]string{d.ID: Digest(d)}, BeforeCommit: strings.Repeat("b", 40), AfterCommit: strings.Repeat("b", 40), BeforeClean: true, AfterClean: true, Evidence: m.ReportEvidence{Path: "summary.json"}, Results: []Result{{TestID: d.ID, Status: status, Message: "actual assertion", Log: "summary.json"}}}
}
func TestTRFileValidationAndSimpleShape(t *testing.T) {
	sha, run, err := ExportRun([]Report{historyReport("failed")})
	if err != nil {
		t.Fatal(err)
	}
	file := RecordFile{Runs: []RecordRun{run}}
	raw, _ := json.Marshal(file)
	if _, err = ParseRecordFile(raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"repository", "branch", "definition", "command", "evidence", "rerun_of", "schema_version"} {
		if strings.Contains(string(raw), `"`+key+`"`) {
			t.Fatal("unnecessary field", key)
		}
	}
	if got, err := RecordCommit(sha + ".json"); err != nil || got != sha {
		t.Fatal(got, err)
	}
	for _, name := range []string{"short.json", "../" + sha + ".json", sha + ".json.bak"} {
		if _, err := RecordCommit(name); err == nil {
			t.Fatal("bad filename", name)
		}
	}
	for _, body := range []string{`{}`, `{"runs":[]}`, `{"runs":null}`, `{"runs":[],"runs":[]}`, strings.Replace(string(raw), "failed", "skipped", 1), strings.Replace(string(raw), `"level":"L1"`, `"level":"L5"`, 1), strings.Replace(string(raw), `"at":`, `"extra":0,"at":`, 1)} {
		if _, err := ParseRecordFile([]byte(body)); err == nil {
			t.Fatal("invalid history", body)
		}
	}
	file.Runs = append(file.Runs, run)
	raw, _ = json.Marshal(file)
	if _, err = ParseRecordFile(raw); err == nil {
		t.Fatal("duplicate run accepted")
	}
}
func TestTRExportFixedCompleteSourceAndRepeat(t *testing.T) {
	for _, mutate := range []func(*Report){func(r *Report) { r.BeforeClean = false }, func(r *Report) { r.AfterClean = false }, func(r *Report) { r.AfterCommit = strings.Repeat("c", 40) }, func(r *Report) { r.BeforeCommit = "" }, func(r *Report) { r.Scope = &m.Scope{Complete: false} }} {
		r := historyReport("passed")
		mutate(&r)
		if _, _, err := ExportRun([]Report{r}); err == nil {
			t.Fatal("unfixed/partial source exported", r)
		}
	}
	if _, _, err := ExportRun(nil); err == nil {
		t.Fatal("empty export")
	}
	first := historyReport("failed")
	_, run, _ := ExportRun([]Report{first})
	var file RecordFile
	if changed, err := file.Append(run); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, err := file.Append(run); err != nil || changed {
		t.Fatal("duplicate export appended", changed, err)
	}
	conflict := run
	conflict.Results = append([]RecordResult{}, run.Results...)
	conflict.Results[0].Status = "passed"
	if _, err := file.Append(conflict); err == nil {
		t.Fatal("conflicting execution silently changed")
	}
	next := historyReport("passed")
	next.StartedAt = next.StartedAt.Add(time.Hour)
	next.FinishedAt = next.FinishedAt.Add(time.Hour)
	_, pass, _ := ExportRun([]Report{next})
	if _, err := file.Append(pass); err != nil {
		t.Fatal(err)
	}
	if len(file.Runs) != 2 || file.Runs[0].Results[0].Status != "failed" {
		t.Fatal("failure erased", file)
	}
	if _, _, err := ExportRun([]Report{first, next}); err == nil {
		t.Fatal("mixed executions accepted")
	}
}
func TestTRQueryGitSnapshotPagingAndFilters(t *testing.T) {
	sha, run, _ := ExportRun([]Report{historyReport("failed")})
	file := RecordFile{Runs: []RecordRun{run}}
	next := run
	next.At = next.At.Add(time.Hour)
	next.Results = []RecordResult{{TestID: "rules", Level: "L1", Environment: "isolated", Status: "passed"}}
	_, _ = file.Append(next)
	snapshot := RecordSnapshot{Commit: strings.Repeat("c", 40), Records: RecordViews(sha, file)}
	page, err := snapshot.List("project", RecordFilter{Limit: 1})
	if err != nil || len(page.Records) != 1 || page.Records[0].Status != "passed" || page.Summary.Total != 2 || page.Summary.Failed != 1 || page.Cursor == "" {
		t.Fatal(page, err)
	}
	last, err := snapshot.List("project", RecordFilter{Limit: 1, Cursor: page.Cursor})
	if err != nil || len(last.Records) != 1 || last.Records[0].Status != "failed" || last.Cursor != "" {
		t.Fatal(last, err)
	}
	for _, f := range []RecordFilter{{Limit: 1, Cursor: page.Cursor, Level: "L2"}, {Limit: 101}, {Level: "L5"}, {Outcome: "skipped"}, {From: "bad"}, {From: "2026-10-24T00:00:00Z", To: "2026-10-23T00:00:00Z"}} {
		if _, err := snapshot.List("project", f); err == nil {
			t.Fatal("invalid filter", f)
		}
	}
	if _, err := snapshot.List("other", RecordFilter{Limit: 1, Cursor: page.Cursor}); err == nil {
		t.Fatal("cross-project cursor")
	}
	snapshot.Commit = strings.Repeat("d", 40)
	if _, err := snapshot.List("project", RecordFilter{Limit: 1, Cursor: page.Cursor}); err == nil {
		t.Fatal("stale Git snapshot cursor")
	}
	filtered, err := snapshot.List("project", RecordFilter{TestID: "rules", Outcome: "failed", Source: sha[:8], Environment: "isolated", From: run.At.Format(time.RFC3339), To: next.At.Format(time.RFC3339)})
	if err != nil || len(filtered.Records) != 1 || filtered.Records[0].Status != "failed" {
		t.Fatal(filtered, err)
	}
}
