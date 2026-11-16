package query

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	refactoring "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestRefactoringShowsLatestDecisionRulesAndExecutionPrompts(t *testing.T) {
	record := refactoring.Record{
		Operations: []refactoring.Operation{
			{Purpose: "first", Request: runnercontract.Request{Input: json.RawMessage(`{
				"generate":{"materials":[
					{"kind":"file","path":"maintenance/rules.md","content":"first rules"},
					{"kind":"file","path":"maintenance/strategy.md","content":"strategy"}
				]}
			}`)}},
			{Purpose: "report", Prompt: "review instruction", Request: runnercontract.Request{Input: json.RawMessage(`{
				"generate":{"materials":[
					{"kind":"file","path":"maintenance/rules.md","content":"review rules  \r\n"},
					{"kind":"file","path":"maintenance/context.json","content":"private review context"},
					{"kind":"file","path":"maintenance/pr.diff","content":"private diff"}
				]}
			}`)}},
			{Purpose: "implement", Prompt: "execution instruction"},
		},
	}

	view := Refactoring(record, time.Now())
	wantRules := map[string]string{"rules": "review rules  \r\n"}
	if !reflect.DeepEqual(view["decision_rules"], wantRules) {
		t.Fatalf("decision rules = %#v, want %#v", view["decision_rules"], wantRules)
	}
	ops := view["operations"].([]map[string]any)
	if ops[1]["workflow"] != "generate" || ops[1]["prompt"] != "review instruction" {
		t.Fatalf("review operation = %#v", ops[1])
	}
	if ops[2]["workflow"] != "execute" || ops[2]["prompt"] != "execution instruction" {
		t.Fatalf("execution operation = %#v", ops[2])
	}

	record.Operations = record.Operations[:1]
	wantRules = map[string]string{"rules": "first rules", "strategy": "strategy"}
	if got := Refactoring(record, time.Now())["decision_rules"]; !reflect.DeepEqual(got, wantRules) {
		t.Fatalf("first decision rules = %#v, want %#v", got, wantRules)
	}
}
