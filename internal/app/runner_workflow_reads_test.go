package app

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

func TestRunnerWorkflowReadProjection(t *testing.T) {
	for _, observation := range []string{"complete", "failed", "absent", "unknown"} {
		t.Run(observation, func(t *testing.T) {
			input := runnercontract.RunInput{Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"model":"execution","reasoning_effort":"medium"}`)}, Workflow: runnercontract.Workflow{ID: "development-audited", Version: 2, AuditModel: &runnercontract.ModelSettings{Model: "audit", ReasoningEffort: "high"}}}
			raw, _ := json.Marshal(input)
			c := runnerstate.Candidate{ID: "candidate", RunID: "runner", TaskID: "task", ProfileID: "preset", ProfileName: "Frozen name", ProfileDigest: runnercontract.ConfigurationDigest(input.Execution, input.Workflow), Submit: runnercontract.Request{Input: raw}, SubmitState: "accepted", Participation: "active", Projection: &runnercontract.Run{RunID: "runner", Status: "running", Workflow: input.Workflow, Execution: runnercontract.Execution{Protocol: "codex-app-server/v2", RequestedModel: "audit", RequestedReasoningEffort: "high"}, Materials: []runnercontract.Material{{Kind: "spec", Path: "spec.md", Commit: "fixed", Digest: "source-sha"}}}}
			if observation != "absent" {
				var count *int
				if observation != "unknown" {
					n := 1
					count = &n
				}
				failure := ""
				if observation == "failed" {
					failure = "audit failure"
				}
				c.Projection.Audit = &runnercontract.Audit{Model: "audit", ReasoningEffort: "high", Results: []runnercontract.AuditResult{{Step: "audit.code.execute", SourceResultID: "implementation-result", ImplementationSource: runnercontract.AuditSource{SessionID: "session", CommandID: "implementation-command", TurnID: "implementation-turn", AttemptID: "attempt", InputVersion: 1, FinalDigest: "digest"}, ResultID: "audit-result", CommandID: "audit-command", TurnID: "audit-turn", PreHead: "pre", PostHead: "post", FixedCount: count, FixedIssues: []runnercontract.FixedIssue{{Severity: "P2", Issue: "issue", Repair: "repair", Evidence: "evidence"}}, RemainingIssues: []runnercontract.RemainingIssue{{Severity: "P3", Issue: "suggestion", Reason: "optional"}}, Failure: failure}}}
			}
			before, _ := json.Marshal(c)
			rt := taskrunner.Project(ports.RunnerSnapshot{Candidates: []runnerstate.Candidate{c}}, hostcontract.ServiceCheckpoint{})
			detail := productServicePage(&rt).Runs[0]
			history := query.RunnerHistory([]runnerstate.Candidate{c}).WorkflowRuns[0]
			if detail.WorkflowVersion != "2" || detail.Protocol != "codex-app-server/v2" || detail.RunnerWorkflow.ID != input.Workflow.ID || detail.RunnerWorkflow.Version != 2 || history.WorkflowVersion != 2 || history.ExecutionID != c.RunID {
				t.Fatal("version/identity axes mixed", detail, history)
			}
			if detail.ExecutionProfile.Name != "Frozen name" || history.ExecutionProfile.Name != "Frozen name" || detail.ExecutionProfile.ExecutionModel != "execution" || detail.RequestedModel != "audit" || detail.Materials[0].Digest != "source-sha" {
				t.Fatal("request/public observation lost", detail)
			}
			if observation == "absent" {
				if detail.Audit != nil {
					t.Fatal("invented audit")
				}
			} else {
				a := detail.Audit
				expected, _ := json.Marshal(c.Projection.Audit)
				actual, _ := json.Marshal(a)
				if string(expected) != string(actual) {
					t.Fatal("audit sources changed", string(expected), string(actual))
				}
				if (a.Results[0].FixedCount == nil) != (observation == "unknown") {
					t.Fatal("invented repair count", a)
				}
				a.Results[0].FixedIssues[0].Issue = "mutated consumer copy"
				a.Results[0].RemainingIssues[0].Reason = "mutated consumer copy"
				if a.Results[0].FixedCount != nil {
					*a.Results[0].FixedCount = 99
				}

			}
			after, _ := json.Marshal(c)
			if string(before) != string(after) {
				t.Fatal("product mapping mutated original observation")
			}
		})
	}
}
