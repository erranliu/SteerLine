package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDocumentDrivenArrangementIsOriginalSelection(t *testing.T) {
	definitions := []Definition{{ID: "z.last", Name: "one", Command: "original", Note: strings.Repeat("完整规则。", 100)}, {ID: "a.first", Name: "two", Command: "second", Note: "独立原因"}}
	plan := Snapshot{Status: "ready", PlanPath: "task.plan.md", Commit: "fixed", Definitions: definitions}
	catalogue := Catalogue{Tests: definitions}
	before, _ := json.Marshal(plan)
	if err := ValidateArrangement(plan, []byte(`{"tests":["z.last","a.first"]}`), catalogue); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"tests":["a.first","z.last"]}`, `{"tests":["z.last"]}`, `{"tests":["z.last","z.last"]}`, `{"tests":["unknown"]}`} {
		if err := ValidateArrangement(plan, []byte(raw), catalogue); err == nil {
			t.Fatal("different arrangement accepted", raw)
		}
	}
	changed := Catalogue{Tests: append([]Definition(nil), definitions...)}
	changed.Tests[0].Note = "changed"
	if err := ValidateArrangement(plan, []byte(`{"tests":["z.last","a.first"]}`), changed); err == nil {
		t.Fatal("changed full definition/note accepted")
	}
	after, _ := json.Marshal(plan)
	if string(before) != string(after) {
		t.Fatal("frozen arrangement mutated")
	}
}
