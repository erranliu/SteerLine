package runnerclient

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
)

const ServiceBindingContract = "steerrunner-service-binding/v1"

type ServiceBindingCapability struct {
	Contract        string   `json:"contract"`
	BeforeEffects   bool     `json:"before_effects"`
	NoImplicitStart bool     `json:"no_implicit_start"`
	Commands        []string `json:"commands"`
}

type ExpectedServiceBinding struct {
	Contract         string `json:"contract"`
	Protocol         string `json:"protocol"`
	InstanceID       string `json:"instance_id"`
	StateDir         string `json:"state_dir"`
	ExecutableSHA256 string `json:"executable_sha256"`
	Version          string `json:"version"`
	ResourceDigest   string `json:"resource_digest"`
}

// WithCatalog freezes only explicitly published admission coverage. Uncovered
// commands retain their current prequery; they never inherit a partial promise.
func (c Client) WithCatalog(catalog *Catalog) Client {
	if catalog == nil {
		return c
	}
	c.executionWaitSchema = catalog.executionWaitSchema
	c.NativeSessionAdapters = append([]string(nil), catalog.Capabilities.NativeSessionViewer.Adapters...)
	u, p := catalog.Capabilities.RunUsage, catalog.Capabilities.RunCost
	c.RunUsageSupported = u != nil && u.Schema == "run-usage/v1" && u.QueryOnly
	c.RunCostSupported = p != nil && p.Schema == "run-cost-estimate/v1" && p.UsageContract == "run-usage/v1" && p.QueryOnly && p.Basis == "api_equivalent"
	c.RunObservation = catalog.Capabilities.RunObservation != nil && catalog.Capabilities.RunObservation.Contract == RunObservationContract && catalog.Capabilities.RunObservation.SameSnapshot && catalog.Capabilities.RunObservation.MaxEvents >= 100
	c.FullBindingCommands = nil
	if cap := catalog.Capabilities.InstanceBinding.FullBinding; cap != nil && cap.Contract == ServiceBindingContract && cap.BeforeEffects && cap.NoImplicitStart {
		c.FullBindingCommands = append([]string(nil), cap.Commands...)
	}
	return c
}

func (c Client) fullBinding(command string) (string, bool, error) {
	if c.InstanceID == "" {
		return "", false, nil
	}
	covered := false
	for _, candidate := range c.FullBindingCommands {
		covered = covered || candidate == command
	}
	if !covered {
		return "", false, nil
	}
	sha, shaErr := hex.DecodeString(c.SHA256)
	resource, resourceErr := hex.DecodeString(c.ResourceDigest)
	if shaErr != nil || len(sha) != 32 || resourceErr != nil || len(resource) != 32 || c.BuildVersion == "" || !filepath.IsAbs(c.StateDir) {
		return "", true, errors.New("complete Runner service binding is required; cannot downgrade to instance-only admission")
	}
	binding := ExpectedServiceBinding{Contract: ServiceBindingContract, Protocol: Protocol, InstanceID: c.InstanceID, StateDir: c.StateDir, ExecutableSHA256: c.SHA256, Version: c.BuildVersion, ResourceDigest: c.ResourceDigest}
	data, err := json.Marshal(binding)
	return string(data), true, err
}
