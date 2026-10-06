package execution

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
)

type recordedStopProcess struct {
	identity   executionbackend.ProcessIdentity
	kills      int
	beforeKill func()
}

func TestWorkerBindingRollsBackWithItsCheckpoint(t *testing.T) {
	s, run := neutralAuditService(t)
	identity, err := resourceprocess.CurrentIdentity()
	require(t, err)
	execution := s.executionRecord(run)
	execution.Process = identity
	setNeutralBinding(t, execution, "ws://127.0.0.1:12345", "generation", workerFlowMockSession)
	execution.Workspace.Worktree = filepath.Join(t.TempDir(), "worker")
	execution.Workspace.Branch, execution.Workspace.Base = execution.WorkspaceIntent.Branch, execution.WorkspaceIntent.BaseCommit
	execution.PolicyResources = neutralPolicyReceipt(s.workflowRecord(run).ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
	s.refreshRun(run)
	ref := executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendLocal, Protocol: neutralProtocol, Target: TargetFor(s.Config, s.runtime.Directory), Agent: AgentFor(s.Config), RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: execution.Workspace.Worktree, WorkspaceBranch: execution.Workspace.Branch, BaseCommit: execution.Workspace.Base, External: *identity, Generation: "generation"}
	_, err = testSQLite(s).db.Exec(`CREATE TRIGGER reject_binding_checkpoint BEFORE INSERT ON projection BEGIN SELECT RAISE(FAIL,'binding checkpoint failure'); END`)
	require(t, err)
	if err := s.commitWorkerBinding(run, ref); err == nil {
		t.Fatal("binding committed without its checkpoint")
	}
	if _, err := s.records.loadBinding(run.RunID, run.AttemptID); !os.IsNotExist(err) {
		t.Fatalf("failed worker receipt left a binding: %v", err)
	}
	snapshot, err := readServiceSnapshot(context.Background(), s.domainStore, s.Config.identity)
	require(t, err)
	if snapshot.Executions[0].Process != nil || s.executionRecord(run).Process != nil {
		t.Fatal("failed binding receipt became an authoritative worker")
	}
}

func (p *recordedStopProcess) Identity() executionbackend.ProcessIdentity { return p.identity }

func (p *recordedStopProcess) Kill() { p.beforeKill(); p.kills++ }

func (p *recordedStopProcess) Close() {}

func TestExecutionStopIntentAndUnknownReceiptUseOriginalBinding(t *testing.T) {
	s, run := neutralAuditService(t)
	ctx := context.Background()
	identity, err := resourceprocess.CurrentIdentity()
	require(t, err)
	execution := s.executionRecord(run)
	execution.Process = identity
	setNeutralBinding(t, execution, "ws://127.0.0.1:12345", "generation", workerFlowMockSession)
	execution.Workspace.Worktree = filepath.Join(t.TempDir(), "worker")
	execution.Workspace.Branch, execution.Workspace.Base = execution.WorkspaceIntent.Branch, execution.WorkspaceIntent.BaseCommit
	execution.PolicyResources = neutralPolicyReceipt(s.workflowRecord(run).ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
	s.refreshRun(run)
	ref := executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendLocal, Protocol: neutralProtocol, Target: TargetFor(s.Config, s.runtime.Directory), Agent: AgentFor(s.Config), RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: execution.Workspace.Worktree, WorkspaceBranch: execution.Workspace.Branch, BaseCommit: execution.Workspace.Base, External: *identity, Generation: "generation"}
	require(t, s.commitWorkerBinding(run, ref))
	records := s.records
	process := &recordedStopProcess{identity: *identity, beforeKill: func() {
		op, err := records.loadOperation(run.RunID, run.AttemptID, "stop-"+run.AttemptID)
		require(t, err)
		if op.Status != executionbackend.OperationInFlight || op.Ref == nil || *op.Ref != ref {
			t.Fatal("stop reached the platform before its original intent was committed")
		}
	}}
	op, err := s.stopRunExecution(ctx, run, &Worker{proc: process})
	require(t, err)
	if op.Status != executionbackend.OperationAccepted || process.kills != 1 {
		t.Fatal("stop did not preserve its accepted receipt")
	}
	_, err = s.stopRunExecution(ctx, run, &Worker{proc: process})
	require(t, err)
	if process.kills != 1 {
		t.Fatal("original stop request was resent")
	}
	if confirmed, err := s.confirmExecutionStop(op); err == nil || confirmed {
		t.Fatal("live original process was treated as a confirmed stop")
	}
	saved, err := records.loadOperation(run.RunID, run.AttemptID, op.ID)
	require(t, err)
	if saved.Status != executionbackend.OperationUnknown || saved.Ref == nil || *saved.Ref != ref {
		t.Fatal("unknown stop lost its fixed identity")
	}
}
