package execution

import (
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestCapabilityReportRoundTripKeepsUnsupportedStopAndOptionalVersion(t *testing.T) {
	c := workerContextConfigFixture()
	report := capabilityReport(executionbackend.BackendLocal, neutralProtocol, TargetFor(c), AgentFor(c), "fixture-version-1")
	if report.ExpectedAgentVersion != "" || report.AgentVersion != "" {
		t.Fatal("optional agent version fixture must remain empty")
	}
	for i := range report.Capabilities {
		if report.Capabilities[i].Name == "stop" {
			report.Capabilities[i].Status = executionbackend.CapabilityUnsupported
		}
	}
	records := executionRecords{root: t.TempDir(), resources: fixtureResources{}}
	require(t, records.saveCapabilityReport(report))
	restored, err := records.loadCapabilityReport(report)
	require(t, err)
	if restored.ExpectedAgentVersion != "" || restored.AgentVersion != "" || capabilityStatus(restored, "stop") != executionbackend.CapabilityUnsupported || capabilityStatus(restored, "start") != executionbackend.CapabilityNotTested {
		t.Fatal("storage upgraded fixture capability declaration")
	}
}
