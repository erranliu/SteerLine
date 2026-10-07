package execution

import (
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestPS2MandatoryLifecycleCapabilitiesKeepFixedIdentity(t *testing.T) {
	ref := executionbackend.ExecutionRef{Backend: "fixture", Protocol: "fixture", Target: executionbackend.ExecutionTarget{ID: "fixed"}, Agent: executionbackend.AgentSpec{Name: "fixture"}}
	report := executionbackend.CapabilityReport{Version: 1, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Agent: ref.Agent, Capabilities: []executionbackend.Capability{{Name: "start", Status: executionbackend.CapabilityVerified}}}
	if err := validateLifecycleStartCapability(ref, report); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*executionbackend.CapabilityReport){
		func(v *executionbackend.CapabilityReport) { v.Target.ID = "sibling" },
		func(v *executionbackend.CapabilityReport) { v.Agent.Name = "other" },
		func(v *executionbackend.CapabilityReport) { v.Capabilities = nil },
		func(v *executionbackend.CapabilityReport) {
			v.Capabilities = []executionbackend.Capability{{Name: "start", Status: executionbackend.CapabilityNotTested}}
		},
	} {
		candidate := report
		change(&candidate)
		if err := validateLifecycleStartCapability(ref, candidate); err == nil {
			t.Fatal("removed optional declarations bypassed necessary capability admission")
		}
	}
}
