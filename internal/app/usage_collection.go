package app

import (
	"context"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/executionconfig"
)

// UsageCollection reads the effective policy of the currently owned daemon.
// It neither creates a runtime nor discovers a different service instance.
func (d *ApplicationDirectory) UsageCollection(ctx context.Context) (any, error) {
	return d.usageCollection(ctx)
}
func (d *ApplicationDirectory) UsageCollectionStatus(ctx context.Context) (any, error) {
	return d.usageCollection(ctx)
}
func (d *ApplicationDirectory) usageCollection(ctx context.Context) (executionconfig.UsageCollection, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	d.mu.RLock()
	stopping := d.stopping
	d.mu.RUnlock()
	out := executionconfig.UsageCollection{Status: "unknown"}
	if stopping {
		return out, errors.New("application stopping")
	}
	d.runner.mu.Lock()
	cli := d.runner.client
	d.runner.mu.Unlock()
	if cli.InstanceID == "" {
		return out, errors.New("Runner binding unavailable")
	}
	info, err := cli.ServiceStatus(ctx)
	if err != nil {
		return out, err
	}
	if info.Status != "ready" || info.InstanceID != cli.InstanceID || info.ExecutableSHA256 != cli.SHA256 || info.ResourceDigest != cli.ResourceDigest || info.Version != cli.BuildVersion {
		return out, errors.New("current Runner service identity differs")
	}
	policy, err := executionconfig.DecodeUsagePolicy(info.UsagePolicy)
	if err != nil || info.UsagePolicyDigest == "" || info.UsagePolicyRevision == "" {
		return out, errors.New("Runner effective usage policy unavailable")
	}
	out.Status = "ready"
	out.Policy = policy
	out.InstanceID = info.InstanceID
	out.PolicyDigest = info.UsagePolicyDigest
	out.PolicyRevision = info.UsagePolicyRevision
	return out, nil
}
