package query

import (
	"encoding/json"
	"testing"
	"time"

	refactoring "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestRefactoringShowsDecisionInstructionForEachOperation(t *testing.T) {
	completedAt := time.Now()
	record := refactoring.Record{
		Reports: []refactoring.Report{{Head: "head", Base: "base", Checks: "checks", CreatedAt: completedAt}},
		Operations: []refactoring.Operation{
			{Purpose: "first", DecisionKind: "execute", DecisionSummary: "selected change", Prompt: "first instruction", Request: runnercontract.Request{Input: json.RawMessage(`{
				"generate":{"materials":[
					{"kind":"file","path":"maintenance/context.json","content":"private first context"}
				]}
			}`)}},
			{Purpose: "implement", Prompt: "execution instruction", Run: &runnercontract.Run{Status: "completed"}},
			{Purpose: "report", PRVersion: "head:base", ReviewChecks: "old checks", CompletedAt: &completedAt},
			{Purpose: "report", PRVersion: "head:base", ReviewChecks: "checks", CompletedAt: &completedAt, Prompt: "review instruction", Request: runnercontract.Request{Input: json.RawMessage(`{
				"generate":{"materials":[
					{"kind":"file","path":"maintenance/context.json","content":"private review context"},
					{"kind":"file","path":"maintenance/pr.diff","content":"private diff"}
				]}
			}`)}},
		},
	}

	view := Refactoring(record, time.Now())
	ops := view["operations"].([]map[string]any)
	if ops[0]["prompt"] != "first instruction" || ops[0]["decision_summary"] != "selected change" || ops[0]["decision_kind"] != "execute" {
		t.Fatalf("first decision operation = %#v", ops[0])
	}
	if ops[1]["workflow"] != "execute" || ops[1]["prompt"] != "execution instruction" || ops[1]["run_status"] != "completed" {
		t.Fatalf("execution operation = %#v", ops[1])
	}
	if _, ok := ops[2]["report_index"]; ok {
		t.Fatalf("outdated review should have no report: %#v", ops[2])
	}
	if ops[3]["workflow"] != "generate" || ops[3]["prompt"] != "review instruction" ||
		ops[3]["report_index"] != 0 {
		t.Fatalf("accepted review operation = %#v", ops[3])
	}
}
