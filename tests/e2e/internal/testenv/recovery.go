package testenv

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"os"
	"path/filepath"
)

func (p *Project) AssertOneDelivery(s Snapshot) {
	if len(s.Runner.Graph.Deliveries) != 1 || len(s.Runner.Candidates) != 1 || s.Runner.Candidates[0].Participation != "completed" {
		p.Scenario.t.Fatal("expected one original candidate delivery")
	}
	for _, task := range s.Page.Tasks {
		if task.TaskID == p.Tasks["b"].ID && (!task.DependencyReady || len(task.WorkflowRuns) > 0) {
			p.Scenario.t.Fatal("successor not ready and unstarted")
		}
	}
}
func (p *Project) CallerRequests() map[string]string {
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(p.Database), "runner", "caller-fixture.json"))
	p.Scenario.Must(err)
	var s controlled.RunnerFixture
	p.Scenario.Must(json.Unmarshal(raw, &s))
	return s.Bodies
}
