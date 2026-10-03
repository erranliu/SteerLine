package steerline

import (
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"testing"
)

func TestCapabilityReportRoundTripKeepsUnsupportedStopAndOptionalVersion(t *testing.T) {
	c := workerContextConfigFixture()
	report := capabilityReport(executionapi.BackendLocal, neutralProtocol, targetFor(c), agentFor(c), "fixture-version-1")
	if report.ExpectedAgentVersion != "" || report.AgentVersion != "" {
		t.Fatal("optional agent version fixture must remain empty")
	}
	for i := range report.Capabilities {
		if report.Capabilities[i].Name == "stop" {
			report.Capabilities[i].Status = executionapi.CapabilityUnsupported
		}
	}
	records := executionRecords{root: t.TempDir()}
	require(t, records.saveCapabilityReport(report))
	restored, err := records.loadCapabilityReport(report)
	require(t, err)
	if restored.ExpectedAgentVersion != "" || restored.AgentVersion != "" || capabilityStatus(restored, "stop") != executionapi.CapabilityUnsupported || capabilityStatus(restored, "start") != executionapi.CapabilityNotTested {
		t.Fatal("storage upgraded fixture capability declaration")
	}
}
