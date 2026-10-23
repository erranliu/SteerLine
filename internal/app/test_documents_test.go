package app

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"os"
	"strings"
	"testing"
)

func testPathCatalog(t *testing.T) *runnerclient.Catalog {
	t.Helper()
	raw, e := os.ReadFile("../runnerclient/testdata/content-capabilities.json")
	require(t, e)
	var caps runnerclient.Capabilities
	require(t, json.Unmarshal(raw, &caps))
	cat, e := runnerclient.CompileCatalog(caps)
	require(t, e)
	return cat
}
func TestTMCommittedPlanSnapshot(t *testing.T) {
	snap := testmanagement.Snapshot{Status: "ready", PlanPath: "SteerlineFlow/plans/task.plan.md", Commit: "fixed", GenerationID: "original", Definitions: []testmanagement.Definition{{ID: "one"}}}
	raw, _ := json.Marshal(snap)
	work := taskmodel.FixedWork{FrozenTestPlan: raw, Contract: taskmodel.TaskContract{PlanRef: taskmodel.FileReference{Path: snap.PlanPath, Commit: snap.Commit}}}
	run := runnerclient.RunInput{Execution: runnerclient.ExecutionInput{Adapter: "codex-app-server", Target: runnerclient.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{}`)}, Workflow: runnerclient.Workflow{ID: "development", Version: 2}}
	c := Config{runtimeCatalog: testPathCatalog(t)}
	got, e := c.prepareTestPlan(context.Background(), work, &run)
	require(t, e)
	if string(got) != string(raw) || len(run.Documents.References) != 2 || !strings.Contains(run.Workflow.PromptOverrides["implement.execute"], testmodel.TestingContext(raw)) {
		t.Fatal("system context was not frozen", run)
	}
	for _, ref := range run.Documents.References {
		if ref.Kind != "" || ref.Commit != "" || ref.Content != nil || strings.Contains(ref.Path, "steerline-test") {
			t.Fatal("pretend document path", ref)
		}
	}
	if _, e = (Config{}).prepareTestPlan(context.Background(), work, &run); e == nil {
		t.Fatal("missing published prompts accepted")
	}
	work.FrozenTestPlan = nil
	got, e = c.prepareTestPlan(context.Background(), work, &run)
	require(t, e)
	var missing testmanagement.Snapshot
	require(t, json.Unmarshal(got, &missing))
	if missing.Status != "missing" {
		t.Fatal("missing snapshot fell back to repository arrangement")
	}
}
