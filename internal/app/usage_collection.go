package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

// UsageCollection reads only the currently owned daemon and known candidate Runs.
// It neither creates a runtime nor discovers a different service instance.
func (d *ApplicationDirectory) UsageCollection(ctx context.Context) (any, error) {
	return d.usageCollection(ctx, true)
}
func (d *ApplicationDirectory) UsageCollectionStatus(ctx context.Context) (any, error) {
	return d.usageCollection(ctx, false)
}
func (d *ApplicationDirectory) usageCollection(ctx context.Context, includeRequirements bool) (executionconfig.UsageCollection, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	d.mu.RLock()
	entries := append([]*catalogEntry(nil), d.entries...)
	stopping := d.stopping
	d.mu.RUnlock()
	out := executionconfig.UsageCollection{Status: "unknown", Requirements: []executionconfig.UsageRequirement{}}
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
	if !includeRequirements {
		return out, nil
	}
	reads := 0
	for _, entry := range entries {
		d.mu.RLock()
		runtime := entry.runtime
		d.mu.RUnlock()
		if runtime == nil || runtime.port == nil {
			continue
		}
		p := runtime.port
		if err = p.enter(); err != nil {
			continue
		}
		candidates, readErr := runtime.store.RunnerCandidates(ctx, p.config.Identity(), "")
		p.users.RUnlock()
		if readErr != nil {
			return out, readErr
		}
		if !readUsageRequirements(ctx, cli, candidates, &reads, &out) {
			return out, nil
		}
	}
	return out, nil
}

// Budget exhaustion limits discovery, not the already verified service or roots.
func readUsageRequirements(ctx context.Context, cli runnerclient.Client, candidates []runnerstate.Candidate, reads *int, out *executionconfig.UsageCollection) bool {
	for _, c := range candidates {
		if c.RunID == "" || !strings.EqualFold(filepath.Clean(c.StateDir), filepath.Clean(cli.StateDir)) {
			continue
		}
		if *reads >= 64 {
			out.Diagnostic = "仅查询前64个Run的授权需求；已核实条目可用，其余尚未查询"
			return false
		}
		*reads++
		usage, queryErr := cli.RunUsage(ctx, c.RunID)
		if queryErr != nil {
			out.Diagnostic = "部分用量需求读取失败，不能据此扩大授权"
			continue
		}
		for _, r := range usage.AccessRequirements {
			if r.SourceRef == "" || r.ObservedAt.IsZero() || !executionconfig.SafeUsageRoot(r.SessionRoot) {
				continue
			}
			out.Requirements = append(out.Requirements, executionconfig.UsageRequirement{RunID: c.RunID, SourceRef: r.SourceRef, Environment: r.Environment, HostID: r.HostID, SessionRoot: r.SessionRoot, ObservedAt: r.ObservedAt})
		}
	}
	return true
}
