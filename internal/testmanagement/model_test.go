package testmanagement

import (
	"encoding/json"
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
