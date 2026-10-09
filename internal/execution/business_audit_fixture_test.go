package execution

import (
	"context"
	"os"
	"os/exec"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func neutralAuditService(t *testing.T, hosts ...string) (*Service, *reconstructiontypes.RunView) {
	return neutralAuditServiceWithConfig(t, nil, hosts...)
}

func neutralAuditServiceWithConfig(t *testing.T, configure func(*Config), hosts ...string) (*Service, *reconstructiontypes.RunView) {
	t.Helper()
	c, commit, store := openDAGFixture(t)
	c.WorkflowProtocol, c.Host, c.Agent = executionmodel.Version, "local", "fixture"
	if configure != nil {
		configure(&c)
	}

	setNeutralProfile(t, &c, neutralSettings{Executable: os.Args[0]})
	dependencies, catalog := neutralFixtureRuntime(c)
	if len(hosts) > 0 && hosts[0] != "local" {
		host := hosts[0]
		key := ExecutionRegistrationKey{Backend: executionbackend.BackendLocal, Protocol: neutralProtocol, Agent: "fixture"}
		entry := dependencies.Directory.entries[key]
		entry.Host = host
		dependencies.Directory.entries[key] = entry
		dependencies.Directory.routes[directoryRoute(host, "fixture")] = key
		c.Host = host
		catalog.Base = c
		catalog.Profiles[0].Host = host
		var resolveErr error
		c, _, resolveErr = catalog.ResolveExecutionProfile(c.selectedProfileID, "")
		require(t, resolveErr)
		rememberNeutralCatalog(c, catalog)
		dependencies, _ = neutralFixtureRuntime(c)
	}
	require(t, validateWorkflowConfig(c, dependencies.Directory))
	require(t, c.PrepareWorktreeRoot())
	ctx := context.Background()
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("fixture push: %v: %s", err, out)
	}
	commands := newTestCommandService(store, c)
	if _, err := commands.CreateTaskBatch(ctx, c, taskBatchFixture(t, c, commit, batchTasks(8), nil), newID(), 1, "operator"); err != nil {
		t.Fatal(err)
	}
	authority, err := store.NativeAuthority(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	if err := store.transactRecords(ctx, func(tx *fixtureTransaction) error {
		return tx.UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "8", RequestID: newID(), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Explicit: true}); err != nil {
		t.Fatal(err)
	}
	service, err := newNeutralService(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	run := service.find("8")
	if run == nil {
		t.Fatal("claimed run missing")
	}
	return service, run
}

type neutralReceiptResolver struct {
	entered         chan struct{}
	confirmedAnswer bool
}

func (r neutralReceiptResolver) ResolveLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	return neutralReceiptPort{LifecyclePort: &neutralLifecycle{Setup: setup}, resolver: r}, nil
}

type neutralReceiptPort struct {
	executionbackend.LifecyclePort
	resolver neutralReceiptResolver
}

func (p neutralReceiptPort) PolicyCapabilities() executionbackend.LifecyclePolicyCapabilities {
	capabilities := p.LifecyclePort.PolicyCapabilities()
	if p.resolver.entered != nil {
		capabilities.ResumeActiveObservation = false
	}
	return capabilities
}

func (p neutralReceiptPort) Reconcile(ctx context.Context, r executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if p.resolver.entered != nil {
		close(p.resolver.entered)
		<-ctx.Done()
		return executionbackend.LifecycleReceipt{Disposition: executionbackend.OperationUnknown}, nil, ctx.Err()
	}
	if p.resolver.confirmedAnswer {
		return executionbackend.LifecycleReceipt{Operation: r.Operation, Disposition: executionbackend.OperationConfirmed, State: executionbackend.LifecycleStateFacts{InputAccepted: true, TurnID: r.Snapshot.Session.CurrentTurnID}}, nil, nil
	}
	return executionbackend.LifecycleReceipt{Operation: r.Operation, Disposition: executionbackend.OperationUnknown}, nil, nil
}
