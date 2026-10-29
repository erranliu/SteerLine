package testenv

import (
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"github.com/erranliu/SteerLine/tests/e2e/internal/evidence"
	"path/filepath"
)

func (p *Project) AssertStagedFlow(snap Snapshot) {
	p.Scenario.t.Helper()
	if len(snap.Runner.Candidates) != 1 || snap.Runner.Candidates[0].Projection == nil {
		p.Scenario.t.Fatal("missing public Run")
	}
	c := snap.Runner.Candidates[0]
	r := c.Projection
	conversation, err := driver.ReadCandidateConversation(p.Scenario.Context, p.Database, p.ProjectID, c.ID, c.RunID)
	p.Scenario.Must(err)
	p.Scenario.Must(controlled.Write(filepath.Join(p.Scenario.Output, p.Name+"-staged-public-finals.json"), conversation))
	count, err := evidence.CandidateFinalCount(conversation.Events, r.Execution.SessionID)
	p.Scenario.Must(err)
	if r.Workflow.ID != "development" || r.Workflow.Version != 2 || count < 2 {
		p.Scenario.t.Fatal("development/2 needs its implementation/publication verified native finals", count)
	}
}
