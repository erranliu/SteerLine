package testenv

import (
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

func TestDeliveryRequestCountSeparatesMaterialGenerationWithoutHidingExtraInputs(t *testing.T) {
	material, delivery, environment := "material-run", "delivery-run", "environment-run"
	fixture := controlled.RunnerFixture{
		Runs: map[string]runnercontract.Run{
			environment: {Workflow: runnercontract.Workflow{ID: "development", Version: 2}, Correlation: map[string]string{"environment_activity": "original-adaptation"}},
			material:    {Workflow: runnercontract.Workflow{ID: "generate", Version: 1}},
			delivery:    {Workflow: runnercontract.Workflow{ID: "development", Version: 1}},
		},
		Requests: map[string]runnercontract.Command{
			"environment": {RunID: &environment}, "environment-stop": {RunID: &environment},
			"material": {RunID: &material}, "submit": {RunID: &delivery}, "stop": {RunID: &delivery},
		},
		Bodies: map[string]string{"environment": "adaptation input", "environment-stop": "adaptation stop", "material": "generation input", "submit": "delivery input", "stop": "stop input"},
	}
	requests := deliveryRequestBodies(fixture)
	if len(requests) != 2 || requests["submit"] == "" || requests["stop"] == "" {
		t.Fatal("preparation changed the delivery submit/stop count", requests)
	}
	fixture.Bodies["duplicate"] = "another delivery input"
	fixture.Requests["duplicate"] = runnercontract.Command{RunID: &delivery}
	if len(deliveryRequestBodies(fixture)) != 3 {
		t.Fatal("duplicate delivery input hidden")
	}
	delete(fixture.Bodies, "duplicate")
	fixture.Bodies["unknown"] = "unclassified input"
	if len(deliveryRequestBodies(fixture)) != 3 {
		t.Fatal("unclassified input hidden")
	}
}
