package executionconfig

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestSteerAgentModelCatalogueDoesNotClaimExecutionReadiness(t *testing.T) {
	models := []runnercontract.NativeModel{{ID: "gpt-6.1-sol", Efforts: []runnercontract.NativeEffort{{Value: "high"}}}}
	catalog := runnercontract.NativeCatalogResult{State: "ready", Complete: true, ContextUnverified: true, Models: &models}
	setting := cfg.SteerAgent{Model: "gpt-6.1-sol", ReasoningEffort: "high"}
	if err := validateSteerAgentModel(setting, catalog); err != nil {
		t.Fatal("valid directory was mistaken for missing execution qualification", err)
	}
	for _, change := range []func(*runnercontract.NativeCatalogResult){func(v *runnercontract.NativeCatalogResult) { v.Complete = false }, func(v *runnercontract.NativeCatalogResult) { v.State = "loading" }, func(v *runnercontract.NativeCatalogResult) { v.Models = nil }} {
		bad := catalog
		change(&bad)
		if validateSteerAgentModel(setting, bad) == nil {
			t.Fatal("incomplete catalogue enabled settings")
		}
	}
	setting.Model = "missing"
	if validateSteerAgentModel(setting, catalog) == nil {
		t.Fatal("missing model accepted")
	}
	setting.Model = "gpt-6.1-sol"
	setting.ReasoningEffort = "medium"
	if validateSteerAgentModel(setting, catalog) == nil {
		t.Fatal("unpublished effort accepted")
	}
}

func TestSteerAgentObsoleteKeyRejectsBeforeMetadataAndSave(t *testing.T) {
	s, config := fixture(t)
	base, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Home, "execution.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw := encode(t, s, config)
	metadataCalls := 0
	s.ReadCatalog = func(context.Context, cfg.Pin) (*catalog, error) {
		metadataCalls++
		t.Fatal("obsolete key reached metadata")
		return nil, nil
	}
	// Unknown-field validation must reject the old key even when its value is null.
	raw = bytes.Replace(raw, []byte(`"config":{`), []byte(`"config":{"continuation_suggestion":null,`), 1)
	if !bytes.Contains(raw, []byte(`"continuation_suggestion"`)) {
		t.Fatal("fixture did not include obsolete configuration")
	}
	if _, err := s.Save(context.Background(), base.SavedDigest, raw); err == nil {
		t.Fatal("obsolete key was accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) || metadataCalls != 0 {
		t.Fatal("rejected configuration changed saved facts", err, metadataCalls)
	}
}
