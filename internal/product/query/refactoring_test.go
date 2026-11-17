package query

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	refactoring "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestRefactoringShowsDecisionRulesForEachOperation(t *testing.T) {
	completedAt := time.Now()
	record := refactoring.Record{
		Reports: []refactoring.Report{{Head: "head", Base: "base", Checks: "checks", CreatedAt: completedAt}},
		Operations: []refactoring.Operation{
			{Purpose: "first", DecisionKind: "execute", DecisionSummary: "selected change", Request: runnercontract.Request{Input: json.RawMessage(`{
				"generate":{"materials":[
					{"kind":"file","path":"maintenance/rules.md","content":"first rules"},
					{"kind":"file","path":"maintenance/strategy.md","content":"strategy"}
				]}
			}`)}},
			{Purpose: "implement", Prompt: "execution instruction", Run: &runnercontract.Run{Status: "completed"}},
			{Purpose: "report", PRVersion: "head:base", ReviewChecks: "old checks", CompletedAt: &completedAt},
			{Purpose: "report", PRVersion: "head:base", ReviewChecks: "checks", CompletedAt: &completedAt, Prompt: "review instruction", Request: runnercontract.Request{Input: json.RawMessage(`{
				"generate":{"materials":[
					{"kind":"file","path":"maintenance/rules.md","content":"review rules  \r\n"},
					{"kind":"file","path":"maintenance/context.json","content":"private review context"},
					{"kind":"file","path":"maintenance/pr.diff","content":"private diff"}
				]}
			}`)}},
		},
	}

	view := Refactoring(record, time.Now())
	ops := view["operations"].([]map[string]any)
	wantRules := map[string]string{"rules": "first rules", "strategy": "strategy"}
	if !reflect.DeepEqual(ops[0]["decision_rules"], wantRules) || ops[0]["decision_summary"] != "selected change" || ops[0]["decision_kind"] != "execute" {
		t.Fatalf("first decision operation = %#v", ops[0])
	}
	if ops[1]["workflow"] != "execute" || ops[1]["prompt"] != "execution instruction" || ops[1]["run_status"] != "completed" {
		t.Fatalf("execution operation = %#v", ops[1])
	}
	if _, ok := ops[2]["report_index"]; ok {
		t.Fatalf("outdated review should have no report: %#v", ops[2])
	}
	wantRules = map[string]string{"rules": "review rules  \r\n"}
	if ops[3]["workflow"] != "generate" || ops[3]["prompt"] != "review instruction" ||
		!reflect.DeepEqual(ops[3]["decision_rules"], wantRules) || ops[3]["report_index"] != 0 {
		t.Fatalf("accepted review operation = %#v", ops[3])
	}
}
