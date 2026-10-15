package testenv

import (
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/tests/e2e/internal/evidence"
)

// AssertRunnerBinding checks caller-visible identity, not Runner-private native permissions.
func (p *Project) AssertRunnerBinding(snapshot Snapshot) {
	p.Scenario.t.Helper()
	if len(snapshot.Runner.Candidates) != 1 {
		p.Scenario.t.Fatal("expected one original Runner candidate")
	}
	c := snapshot.Runner.Candidates[0]
	r := c.Projection
	if c.SubmitState != "accepted" || r == nil || c.RunID != r.RunID || len(r.InputDigest) != 64 || r.Execution.AttemptID == "" || r.Execution.Generation == "" || r.Execution.SessionID == "" || r.Execution.Workspace == "" || r.Execution.Branch == "" {
		p.Scenario.t.Fatal("public Runner binding incomplete", c)
	}
}

func ExecutionSession(e executionmodel.ExecutionRecord) (string, error) { return evidence.Session(e) }
