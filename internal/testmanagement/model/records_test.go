package model

import (
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestGenerationNeedsAttentionDoesNotChangeSettlement(t *testing.T) {
	for _, tc := range []struct {
		name       string
		generation Generation
		want       bool
	}{
		{"blocked", Generation{State: "accepted", Projection: &runnercontract.Run{Status: "blocked"}}, true},
		{"manual", Generation{State: "accepted", Projection: &runnercontract.Run{Phase: "manual_intervention_required"}}, true},
		{"running", Generation{State: "accepted", Projection: &runnercontract.Run{Status: "running"}}, false},
		{"diagnostic_only", Generation{State: "accepted", Diagnostic: "manual_intervention_required"}, false},
		{"unknown", Generation{State: "unknown", Projection: &runnercontract.Run{Status: "blocked"}}, false},
		{"cancelled", Generation{State: "cancelled", Cancelled: true, Projection: &runnercontract.Run{Status: "blocked"}}, false},
		{"settled", Generation{State: "accepted", Projection: &runnercontract.Run{Status: "blocked", StopStatus: "stopped"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.generation.Settled()
			if got := tc.generation.NeedsAttention(); got != tc.want || tc.generation.Settled() != before {
				t.Fatalf("attention=%t want=%t; settlement changed", got, tc.want)
			}
		})
	}
}
