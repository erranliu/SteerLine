package steerline

import (
	"context"
	"errors"
	"fmt"
)

type LegacyWorkspaceCheckpoint func(path, branch, base string) error

type LegacyDispatchResult struct {
	Worker          *Worker
	Process         ProcessIdentity
	WorkspacePath   string
	WorkspaceBranch string
	BaseRevision    string
	Prepared        bool
	Uncertain       bool
}

// LegacyDispatchAdapter is the compatibility bridge from the current single
// Run orchestration into the stable backend/protocol identity model.
type LegacyDispatchAdapter interface {
	Identity() (backend, protocol string)
	Start(context.Context, Config, Task, *Run, BeadsContext, *WorkspaceIntent, string, LegacyWorkspaceCheckpoint) (LegacyDispatchResult, error)
}

type legacyAdapterRegistry struct {
	adapters map[string]LegacyDispatchAdapter
}

func newLegacyAdapterRegistry() *legacyAdapterRegistry {
	r := &legacyAdapterRegistry{adapters: map[string]LegacyDispatchAdapter{}}
	for _, adapter := range []LegacyDispatchAdapter{localProcessLegacyAdapter{}, orcaTerminalLegacyAdapter{}} {
		backend, protocol := adapter.Identity()
		r.adapters[backendKey(backend, protocol)] = adapter
	}
	return r
}
func (r *legacyAdapterRegistry) Resolve(backend, protocol string) (LegacyDispatchAdapter, error) {
	if r != nil {
		if adapter := r.adapters[backendKey(backend, protocol)]; adapter != nil {
			return adapter, nil
		}
	}
	return nil, &AdapterError{Kind: AdapterUnsupported, Operation: "legacy start", Diagnostic: fmt.Sprintf("no legacy adapter registered for %s/%s; fallback is disabled", backend, protocol)}
}

type localProcessLegacyAdapter struct{}

func (localProcessLegacyAdapter) Identity() (string, string) { return "local_process", "local_process" }
func (localProcessLegacyAdapter) Start(ctx context.Context, c Config, t Task, r *Run, source BeadsContext, intent *WorkspaceIntent, dir string, checkpoint LegacyWorkspaceCheckpoint) (LegacyDispatchResult, error) {
	result := LegacyDispatchResult{}
	workerConfig, err := prepareWorkspace(ctx, c, r, source)
	if err != nil {
		return result, err
	}
	result.Prepared = true
	result.WorkspacePath = intent.Path
	result.WorkspaceBranch = intent.Branch
	result.BaseRevision = intent.BaseCommit
	if checkpoint != nil {
		if err = checkpoint(result.WorkspacePath, result.WorkspaceBranch, result.BaseRevision); err != nil {
			return result, err
		}
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	worker, err := StartWorker(workerConfig, t, r)
	if err != nil {
		return result, err
	}
	result.Worker = worker
	result.Process = worker.proc.Identity()
	return result, nil
}

type orcaTerminalLegacyAdapter struct{}

func (orcaTerminalLegacyAdapter) Identity() (string, string) { return "orca", "orca_terminal_v2" }
func (orcaTerminalLegacyAdapter) Start(ctx context.Context, c Config, t Task, r *Run, source BeadsContext, intent *WorkspaceIntent, dir string, checkpoint LegacyWorkspaceCheckpoint) (LegacyDispatchResult, error) {
	result := LegacyDispatchResult{Uncertain: true}
	if intent == nil {
		return result, errors.New("missing durable workspace intent")
	}
	boundRun := *r
	boundIntent := *intent
	worker, err := startOrcaRun(ctx, c, t, &boundRun, source, &boundIntent, dir)
	if err != nil {
		if boundRun.WorktreePath != nil {
			result.Prepared = true
			result.WorkspacePath = *boundRun.WorktreePath
		}
		if boundRun.WorktreeBranch != nil {
			result.WorkspaceBranch = *boundRun.WorktreeBranch
		}
		if boundRun.BaseCommit != nil {
			result.BaseRevision = *boundRun.BaseCommit
		}
		return result, err
	}
	result.Worker = worker
	result.Prepared = true
	result.Uncertain = false
	result.Process = worker.proc.Identity()
	result.WorkspacePath = deref(boundRun.WorktreePath)
	result.WorkspaceBranch = deref(boundRun.WorktreeBranch)
	result.BaseRevision = deref(boundRun.BaseCommit)
	if checkpoint != nil {
		if err = checkpoint(result.WorkspacePath, result.WorkspaceBranch, result.BaseRevision); err != nil {
			return result, err
		}
	}
	return result, nil
}
