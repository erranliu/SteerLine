package steerline

import (
	"context"
	"os"
	"os/exec"
	"testing"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func neutralAuditService(t *testing.T) (*Service, *Run) {
	return neutralAuditServiceWithRequirements(t, workspacePolicyFixture(), executionapi.ExecutionRequirements{}, "")
}

func neutralAuditServiceWithRequirements(t *testing.T, policy executionapi.ExecutionPolicy, requirements executionapi.ExecutionRequirements, tokenEnv string) (*Service, *Run) {
	t.Helper()
	c, commit, store := openDAGFixture(t)
	c.WorkflowProtocol, c.Host, c.Agent, c.Claimant = WorkerFlowVersion, "local", "fixture", "operator"
	c.ExecutionPolicy = policy
	c.ExecutionRequirements = requirements
	if tokenEnv != "" {
		c.PR.TokenEnv = tokenEnv
	}
	setNeutralProfile(t, &c, neutralSettings{Executable: os.Args[0]})
	require(t, validateWorkflowConfig(&c, nil))
	require(t, c.prepareWorktreeRoot())
	ctx := context.Background()
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("fixture push: %v: %s", err, out)
	}
	commands := CommandService{Store: store, Project: c.Identity()}
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

func neutralAuditServiceWithPolicy(t *testing.T, p executionapi.ExecutionPolicy) (*Service, *Run) {
	return neutralAuditServiceWithRequirements(t, p, executionapi.ExecutionRequirements{}, "")
}

type neutralReceiptResolver struct {
	entered         chan struct{}
	confirmedAnswer bool
}

func (r neutralReceiptResolver) ResolveLifecycle(setup executionapi.LifecycleSetup) (executionapi.LifecyclePort, error) {
	return neutralReceiptPort{LifecyclePort: &neutralLifecycle{setup: setup}, resolver: r}, nil
}

type neutralReceiptPort struct {
	executionapi.LifecyclePort
	resolver neutralReceiptResolver
}

func (p neutralReceiptPort) ObserveLifecycle(ctx context.Context, s executionapi.LifecycleSnapshot) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	if p.resolver.entered != nil {
		close(p.resolver.entered)
		<-ctx.Done()
		return executionapi.LifecycleReceipt{Disposition: executionapi.OperationUnknown}, nil, ctx.Err()
	}
	return p.LifecyclePort.ObserveLifecycle(ctx, s)
}
func (p neutralReceiptPort) Reconcile(_ context.Context, r executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	if p.resolver.confirmedAnswer {
		return executionapi.LifecycleReceipt{Operation: r.Operation, Disposition: executionapi.OperationConfirmed, State: executionapi.LifecycleStateFacts{InputAccepted: true, TurnID: r.Snapshot.Session.CurrentTurnID}}, nil, nil
	}
	return executionapi.LifecycleReceipt{Operation: r.Operation, Disposition: executionapi.OperationUnknown}, nil, nil
}
