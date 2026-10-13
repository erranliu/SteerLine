package execution

import (
	"context"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"

	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func TestRecoveryInspectionReadsBoundRunWithoutServiceResources(t *testing.T) {
	s, run := neutralAuditService(t)
	e, w := s.executionRecord(run), s.workflowRecord(run)
	e.Generation, e.ConfigurationDigest = "resources", w.ExecutionProfile.Digest
	e.Workspace = executionmodel.WorkspaceEvidence{Repository: s.Config.identity, Worktree: run.WorkspaceIntent.Path, Branch: run.WorkspaceIntent.Branch, Base: run.WorkspaceIntent.BaseCommit}
	identity := executionbackend.ProcessIdentity{Kind: "fixture", PID: 42, Created: "original-generation"}
	e.Process = &identity
	e.PolicyResources = neutralPolicyReceipt(w.ExecutionProfile.Digest, e.Workspace.Worktree, e)
	w.StepID, w.Status = executionmodel.StepReport, "reconciling"
	s.refreshRun(run)

	port := &inspectionLifecycle{want: identity}
	dependencies := s.runtime
	entry, err := dependencies.Directory.ResolveIdentity(ExecutionRegistrationKey{Backend: executionbackend.BackendLocal, Protocol: neutralProtocol, Agent: "fixture"})
	require(t, err)
	entry.Lifecycle = func(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
		if setup.OwnedProcess != nil || setup.WorkerTreeAbsent != nil {
			t.Fatal("read-only inspection acquired service-owned process capabilities")
		}
		return port, nil
	}
	dependencies.Directory = neutralDirectoryWithReplacement(t, dependencies.Directory, entry)

	state := s.State
	state.Tasks, state.WorkflowRuns, state.Executions, state.Attempts = s.domain.Tasks, s.domain.WorkflowRuns, s.domain.Executions, s.domain.Attempts
	state.Decisions, state.Operations, state.Deliveries, state.Runs = s.domain.Decisions, s.domain.Operations, s.domain.Deliveries, s.runs
	before := recordFingerprint(state)
	report, err := InspectRecovery(context.Background(), s.Config, dependencies, run.RunID, func(context.Context, Config) (*reconstruction.Runtime, error) {
		return &state, nil
	})
	require(t, err)
	if report.RunID != run.RunID || report.WorkflowStatus != "reconciling" || report.ProcessCheck != "matching process still exists" || report.Workspace == nil || report.Workspace.Worktree != e.Workspace.Worktree || report.NativeClaim == nil || report.NativeClaim.TaskID != run.DomainTaskID {
		t.Fatalf("read-only inspection lost the bound run facts: %+v", report)
	}
	if port.ref.Session.ExecutionID != e.ExecutionID || port.ref.Session.AttemptID != run.AttemptID || port.ref.Ref.External != identity {
		t.Fatal("inspection replaced the original execution identity")
	}
	if recordFingerprint(state) != before {
		t.Fatal("read-only inspection modified the runtime snapshot")
	}
}

func TestRecoveryInspectionReadsNativeClaimFromCenter(t *testing.T) {
	project := "repo:native-recovery"
	task := taskmodel.TaskRecord{ProjectIdentity: project, TaskID: "7", Claimant: "native-claimant", Status: "in_progress"}
	service := Service{Config: Config{identity: project}, State: reconstruction.Runtime{}, domain: reconstruction.Snapshot{Tasks: []taskmodel.TaskRecord{task}}}
	report := RecoveryInspection{}
	service.inspectRecoveryClaim(context.Background(), &reconstruction.RunView{TaskID: "7", DomainTaskID: "7"}, &report)
	if report.NativeClaim == nil || report.NativeClaim.TaskID != task.TaskID || report.NativeClaim.Claimant != task.Claimant || report.ClaimError != "" {
		t.Fatalf("native recovery claim lost its center evidence: %+v", report)
	}
}

func TestRecoveryConfirmationBasisIsBoundedAndPreserved(t *testing.T) {
	basis := strings.Repeat("x", 2001)
	if _, err := validateConfirmationBasis(basis); err == nil || !strings.Contains(err.Error(), "2000 character") {
		t.Fatalf("oversized confirmation was accepted: %v", err)
	}
	if len([]rune(basis)) != 2001 {
		t.Fatal("confirmation input was modified")
	}
}
