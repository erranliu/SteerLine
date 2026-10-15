package testenv

import (
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
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
	items := map[string]bool{}
	for _, event := range conversation.Events {
		if event.Type != api.ConversationMessageComplete || event.Phase != api.MessagePhaseFinalAnswer || event.SourceRole != api.ConversationSourceCanonical {
			continue
		}
		if event.SessionID != r.Execution.SessionID || event.TurnID == "" || event.ItemKey == "" || event.Body == nil {
			p.Scenario.Must(errors.New("public canonical final lost its original native provenance"))
		}
		items[event.ItemKey] = true
	}
	if len(items) != 3 {
		p.Scenario.t.Fatal("development workflow needs three distinct verified native finals", len(items))
	}
	p.Scenario.Must(controlled.Write(filepath.Join(p.Scenario.Output, p.Name+"-staged-public-finals.json"), conversation))
}
