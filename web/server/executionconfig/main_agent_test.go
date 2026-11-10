package executionconfig

import (
	"testing"

	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestMainAgentModelCatalogueDoesNotClaimExecutionReadiness(t *testing.T) {
	models := []runnercontract.NativeModel{{ID: "gpt-6.1-sol", Efforts: []runnercontract.NativeEffort{{Value: "high"}}}}
	catalog := runnercontract.NativeCatalogResult{State: "ready", Complete: true, ContextUnverified: true, Models: &models}
	setting := cfg.MainAgent{Model: "gpt-6.1-sol", ReasoningEffort: "high"}
	if err := validateMainAgentModel(setting, catalog); err != nil {
		t.Fatal("valid directory was mistaken for missing execution qualification", err)
	}
	for _, change := range []func(*runnercontract.NativeCatalogResult){func(v *runnercontract.NativeCatalogResult) { v.Complete = false }, func(v *runnercontract.NativeCatalogResult) { v.State = "loading" }, func(v *runnercontract.NativeCatalogResult) { v.Models = nil }} {
		bad := catalog
		change(&bad)
		if validateMainAgentModel(setting, bad) == nil {
			t.Fatal("incomplete catalogue enabled settings")
		}
	}
	setting.Model = "missing"
	if validateMainAgentModel(setting, catalog) == nil {
		t.Fatal("missing model accepted")
	}
	setting.Model = "gpt-6.1-sol"
	setting.ReasoningEffort = "medium"
	if validateMainAgentModel(setting, catalog) == nil {
		t.Fatal("unpublished effort accepted")
	}
}
