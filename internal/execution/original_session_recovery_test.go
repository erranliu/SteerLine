package execution

import (
	"context"
	"strings"
	"testing"

	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

type unavailableSessionLifecycle struct{ recoveryOnlyLifecycle }

func (unavailableSessionLifecycle) VerifyManualRecovery(context.Context, backend.LifecycleSnapshot) (backend.LifecycleRecoveryEvidence, error) {
	return backend.LifecycleRecoveryEvidence{ProcessAbsent: true}, nil
}

type unavailableSessionResolver struct{}

func (unavailableSessionResolver) ResolveLifecycle(backend.LifecycleSetup) (backend.LifecyclePort, error) {
	return unavailableSessionLifecycle{}, nil
}

func TestRecoveryCannotSelectReplacementSession(t *testing.T) {
	service, run := waitingDecisionService(t)
	t.Cleanup(func() { _ = service.Close() })
	execution := service.executionRecord(run)
	execution.RecoveryRequired, execution.RecoveryState = true, "interrupted"
	beforeExecution, beforeAttempts := recordFingerprint(*execution), len(service.domain.Attempts)
	if err := service.beginWorkerRecoveryAttempt(context.Background(), run, "new_session"); err == nil {
		t.Fatal("replacement session recovery was accepted")
	}
	if recordFingerprint(*execution) != beforeExecution || len(service.domain.Attempts) != beforeAttempts {
		t.Fatal("rejected recovery changed the original execution")
	}
	// A lost session cannot offer a continue option that creates a first session.
	execution.SessionID = ""
	service.setDecisionStatus(run, "superseded")
	service.setRunDecision(run, nil)
	require(t, service.waitWorkerFlow(run, "original session lost"))
	if run.Decision == nil || len(run.Decision.Request.Options) != 0 || !strings.Contains(run.Decision.Request.Context, "manually") || len(service.domain.Attempts) != beforeAttempts {
		t.Fatalf("missing session offered replacement execution: %+v", run.Decision)
	}
}

func TestUnavailableOriginalSessionNeedsManualHandling(t *testing.T) {
	service, run := waitingDecisionService(t)
	t.Cleanup(func() { _ = service.Close() })
	service.lifecycleResolver = unavailableSessionResolver{}
	execution := service.executionRecord(run)
	execution.SessionID = "unavailable-original-session"
	workflow := service.workflowRecord(run)
	execution.Generation, execution.ConfigurationDigest = "resources", workflow.ExecutionProfile.Digest
	execution.Workspace = executionmodel.WorkspaceEvidence{Repository: service.Config.identity, Worktree: run.WorkspaceIntent.Path, Branch: run.WorkspaceIntent.Branch, Base: run.WorkspaceIntent.BaseCommit}
	execution.PolicyResources = neutralPolicyReceipt(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
	execution.RecoveryRequired = true
	originalSession, attempts := execution.SessionID, len(service.domain.Attempts)
	handled, err := service.resumeLifecycleWorkerFlow(context.Background(), nil, run)
	require(t, err)
	if !handled || execution.RecoveryState != "unverified" || run.Decision == nil || len(run.Decision.Request.Options) != 0 || !strings.Contains(run.Decision.Request.Context, "unavailable") || execution.SessionID != originalSession || len(service.domain.Attempts) != attempts {
		t.Fatalf("unavailable original session was replaced or offered continuation: %+v", run.Decision)
	}
}
