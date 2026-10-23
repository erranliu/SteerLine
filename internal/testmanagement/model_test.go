package testmanagement

import (
	"encoding/json"
	"strings"
	"testing"
)

func definition() Definition {
	return Definition{ID: "unit.rules", Name: "规则", Level: "L1", Environment: "isolated", Command: "test-rules", Check: "全部规则通过"}
}
func TestTMDocuments(t *testing.T) {
	d := definition()
	raw, _ := json.Marshal(Catalogue{Tests: []Definition{d}})
	c, err := ParseCatalogue(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"tests":["unit.rules","unit.rules"]}`, `{"tests":["unknown"]}`, `{"tests":[],"unexpected":true}`, `{"tests":[],"tests":["unit.rules"]}`, `{"tests":null}`} {
		if _, err = Select([]byte(body), c); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	selected, err := Select([]byte(`{"tests":[]}`), c)
	if err != nil || selected == nil || len(selected) != 0 {
		t.Fatalf("empty: %v %v", selected, err)
	}
	if _, err = ParseCatalogue([]byte(`{"tests":[{"id":"x"}]}`)); err == nil {
		t.Fatal("accepted missing required fields")
	}
	for _, name := range []string{"../x.plan.md", "C:/x.plan.md", "docs\\x.plan.md", "x.md", "/x.plan.md"} {
		if _, err = ArrangementPath(name); err == nil {
			t.Fatal("unsafe Plan path", name)
		}
	}
	if name, err := ArrangementPath("SteerlineFlow/plans/task.plan.md"); err != nil || name != "SteerlineFlow/plans/task.tests.json" {
		t.Fatal(name, err)
	}
}
func TestTMResultsLatestAndSource(t *testing.T) {
	d := definition()
	commit := strings.Repeat("a", 40)
	s := Snapshot{Status: "ready", Definitions: []Definition{d}}
	pass := Report{ID: "old", Sequence: 1, Selection: []string{d.ID}, Environment: d.Environment, Definitions: map[string]string{d.ID: Digest(d)}, BeforeCommit: commit, AfterCommit: commit, BeforeClean: true, AfterClean: true, Results: []Result{{TestID: d.ID, Status: "passed", Log: "old-log"}}}
	fail := pass
	fail.ID = "new"
	fail.Sequence = 2
	fail.Results = []Result{{TestID: d.ID, Status: "failed", Message: "assertion", Log: "new-log"}}
	view := Project(s, []Report{fail, pass}, commit, true, true)
	if view.Failed != 1 || view.Passed != 0 || len(view.Rows[0].History) != 2 || view.Rows[0].Log != "new-log" {
		t.Fatalf("late old pass overrode failure: %+v", view)
	}
	for _, mutation := range []func(*Report){func(r *Report) { r.BeforeClean = false }, func(r *Report) { r.AfterCommit = strings.Repeat("b", 40) }, func(r *Report) { r.Environment = "live" }, func(r *Report) { r.Definitions = map[string]string{d.ID: "changed"} }} {
		r := pass
		mutation(&r)
		v := Project(s, []Report{r}, commit, true, true)
		if v.Passed != 0 || v.Pending != 1 || v.Rows[0].Status != "stale" {
			t.Fatalf("invalid source counted: %+v", v)
		}
	}
	if v := Project(s, []Report{pass}, commit, true, false); v.Passed != 0 {
		t.Fatal("unknown current source passed")
	}
	if v := Project(Snapshot{Status: "ready", Definitions: []Definition{}}, nil, "", false, false); v.Total != 0 || v.Passed != 0 {
		t.Fatal("empty fabricated success")
	}
}
func TestTMReportValidation(t *testing.T) {
	r := historyReport("failed")
	raw, _ := json.Marshal(r)
	if _, err := ParseReport(raw); err != nil {
		t.Fatal(err)
	}
	r.Results[0].Message = ""
	raw, _ = json.Marshal(r)
	if _, err := ParseReport(raw); err == nil {
		t.Fatal("failure without reason admitted")
	}
	r.Results[0].Status = "skipped"
	raw, _ = json.Marshal(r)
	if _, err := ParseReport(raw); err == nil {
		t.Fatal("unexpected status admitted")
	}
	r.Results = nil
	raw, _ = json.Marshal(r)
	if _, err := ParseReport(raw); err == nil {
		t.Fatal("missing coverage admitted")
	}
}
