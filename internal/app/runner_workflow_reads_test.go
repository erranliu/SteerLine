package app

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

func TestRunnerWorkflowReadProjection(t *testing.T) {
	for _, verdict := range []string{"pass", "fail", "absent", "unknown"} {
		t.Run(verdict, func(t *testing.T) {
			input := runnercontract.RunInput{Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"model":"execution","reasoning_effort":"medium"}`)}, Workflow: runnercontract.Workflow{ID: "development-audited", Version: 1, AuditModel: &runnercontract.ModelSettings{Model: "audit", ReasoningEffort: "high"}}}
			raw, _ := json.Marshal(input)
			c := runnerstate.Candidate{ID: "candidate", RunID: "runner", TaskID: "task", ProfileID: "preset", ProfileName: "Frozen name", ProfileDigest: runnercontract.ConfigurationDigest(input.Execution, input.Workflow), Submit: runnercontract.Request{Input: raw}, SubmitState: "accepted", Participation: "active", Projection: &runnercontract.Run{RunID: "runner", Status: "running", Workflow: input.Workflow, Execution: runnercontract.Execution{Protocol: "codex-app-server/v2", RequestedModel: "audit", RequestedReasoningEffort: "high"}, Materials: []runnercontract.Material{{Kind: "spec", Path: "spec.md", Commit: "fixed", Digest: "source-sha"}}}}
			if verdict != "absent" {
				c.Projection.Audit = &runnercontract.Audit{Model: "audit", ReasoningEffort: "high", RepairCount: 1, Results: []runnercontract.AuditResult{{Step: "spec.audit", SourceReportCommandID: "original-report", CommandID: "audit-command", TurnID: "audit-turn", Verdict: verdict, Failure: func() string {
					if verdict == "fail" {
						return "audit failure"
					}
					return ""
				}(), Findings: []runnercontract.Finding{{Severity: "P2", Message: "finding", Path: "file.go", Line: 3}}}}}
			}
			before, _ := json.Marshal(c)
			rt := taskrunner.Project(ports.RunnerSnapshot{Candidates: []runnerstate.Candidate{c}}, ports.ServiceCheckpoint{})
			detail := productServicePage(&rt).Runs[0]
			history := query.RunnerHistory([]runnerstate.Candidate{c}).WorkflowRuns[0]
			if detail.WorkflowVersion != "1" || detail.Protocol != "codex-app-server/v2" || detail.RunnerWorkflow.ID != input.Workflow.ID || detail.RunnerWorkflow.Version != 1 || history.WorkflowVersion != 1 || history.ExecutionID != c.RunID {
				t.Fatal("version/identity axes mixed", detail, history)
			}
			if detail.ExecutionProfile.Name != "Frozen name" || history.ExecutionProfile.Name != "Frozen name" || detail.ExecutionProfile.ExecutionModel != "execution" || detail.RequestedModel != "audit" || detail.Materials[0].Digest != "source-sha" {
				t.Fatal("request/public observation lost", detail)
			}
			if verdict == "absent" {
				if detail.Audit != nil {
					t.Fatal("invented audit")
				}
			} else {
				a := detail.Audit
				if a.Model != "audit" || a.Results[0].Verdict != verdict || a.Results[0].SourceReportCommandID != "original-report" || a.Results[0].CommandID != "audit-command" || a.Results[0].Findings[0].Line != 3 {
					t.Fatal("audit sources changed", a)
				}
				a.Results[0].Findings[0].Message = "mutated consumer copy"
			}
			after, _ := json.Marshal(c)
			if string(before) != string(after) {
				t.Fatal("product mapping mutated original observation")
			}
		})
	}
}
