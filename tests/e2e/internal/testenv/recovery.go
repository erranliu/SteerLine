package testenv

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
)

func (p *Project) AssertOneDelivery(s Snapshot) {
	if len(s.Runner.Graph.Deliveries) != 1 || len(s.Runner.Candidates) != 1 || s.Runner.Candidates[0].Participation != "completed" {
		p.Scenario.t.Fatal("expected one original candidate delivery")
	}
	for _, task := range s.Page.Tasks {
		if task.TaskID == p.Tasks["b"].ID && (!task.DependencyReady || len(task.Candidates) > 0) {
			p.Scenario.t.Fatal("successor not ready and unstarted")
		}
	}
}
func (p *Project) CallerRequests() map[string]string {
	raw, err := os.ReadFile(filepath.Join(runnerstatepath.Directory(p.Home), "caller-fixture.json"))
	p.Scenario.Must(err)
	var s controlled.RunnerFixture
	p.Scenario.Must(json.Unmarshal(raw, &s))
	return deliveryRequestBodies(s)
}

// Material generation is preparation, separate from the delivery Run's submit
// and stop. Keep every unclassified request so it cannot hide an extra input.
func deliveryRequestBodies(fixture controlled.RunnerFixture) map[string]string {
	requests := map[string]string{}
	for id, body := range fixture.Bodies {
		command := fixture.Requests[id]
		if command.RunID != nil && fixture.Runs[*command.RunID].Workflow.ID == "generate" {
			continue
		}
		requests[id] = body
	}
	return requests
}

// This is independent cleanup after delivery, not a native stop implementation.
func (p *Project) WaitStoppedCandidates() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for {
		snapshot, err := driver.ReadWorkflow(ctx, p.Database, p.ProjectID)
		stopped := err == nil
		for _, c := range snapshot.Candidates {
			if c.SubmitState == "unknown" || c.RunID != "" && (c.Projection == nil || c.Projection.StopStatus != "stopped") {
				stopped = false
			}
		}
		if stopped {
			return
		}
		if ctx.Err() != nil {
			p.Scenario.t.Fatal(fmt.Errorf("public Run stop settlement: %w", ctx.Err()))
		}
		time.Sleep(100 * time.Millisecond)
	}
}
