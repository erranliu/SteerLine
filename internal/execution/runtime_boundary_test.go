package execution

import (
	"context"
	"fmt"
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

type ownedStopResolver struct {
	ref          executionbackend.ExecutionRef
	stops, reads int
	proven       bool
}

func (r *ownedStopResolver) ResolveLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	return ownedStopLifecycle{LifecyclePort: &neutralLifecycle{setup: setup}, owner: r}, nil
}

type ownedStopLifecycle struct {
	executionbackend.LifecyclePort
	owner *ownedStopResolver
}

func (p ownedStopLifecycle) StopLifecycle(_ context.Context, r executionbackend.LifecycleRequest, _ executionbackend.StopHandle) (executionbackend.LifecycleReceipt, error) {
	if r.Operation.Ref == nil || *r.Operation.Ref != p.owner.ref {
		return executionbackend.LifecycleReceipt{}, fmt.Errorf("foreign stop binding")
	}
	p.owner.stops++
	return executionbackend.LifecycleReceipt{Operation: r.Operation, Disposition: executionbackend.OperationUnknown, Diagnostic: "native stop applied but reply lost"}, nil
}
func (p ownedStopLifecycle) Reconcile(_ context.Context, r executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if r.Operation.Ref == nil || *r.Operation.Ref != p.owner.ref || p.owner.stops != 1 {
		return executionbackend.LifecycleReceipt{}, nil, fmt.Errorf("foreign/unapplied native stop")
	}
	p.owner.reads++
	if !p.owner.proven {
		return executionbackend.LifecycleReceipt{Operation: r.Operation, Disposition: executionbackend.OperationUnknown}, nil, nil
	}
	op := r.Operation
	op.Receipt = []byte(`{"state":"stopped","original_resource":true}`)
	return executionbackend.LifecycleReceipt{Operation: op, Disposition: executionbackend.OperationConfirmed, State: executionbackend.LifecycleStateFacts{Stopped: true}, Diagnostic: "original native resource durably stopped"}, nil, nil
}
func TestHostManagedStopRecoveryUsesOriginalLifecycleProof(t *testing.T) {
	for _, scenario := range []string{"sync", "background", "first-discard"} {
		t.Run(scenario, func(t *testing.T) {
			background := scenario == "background"
			s, run := neutralAuditService(t)
			entry, key := ExecutionRegistration{}, ExecutionRegistrationKey{Backend: executionbackend.BackendLocal, Protocol: neutralProtocol, Agent: "fixture"}
			entry = s.runtime.Directory.entries[key]
			entry.HostManagedPermissions = true
			s.runtime.Directory.entries[key] = entry
			identity, err := resourceprocess.CurrentIdentity()
			require(t, err)
			record := s.executionRecord(run)
			record.Process = identity
			setNeutralBinding(t, record, "ws://127.0.0.1:12345", "generation", workerFlowMockSession)
			record.Workspace.Worktree = filepath.Join(t.TempDir(), "worker")
			record.Workspace.Branch, record.Workspace.Base = record.WorkspaceIntent.Branch, record.WorkspaceIntent.BaseCommit
			record.PolicyResources = nil
			record.Target = TargetFor(s.Config, s.runtime.Directory)
			s.refreshRun(run)
			ref := executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendLocal, Protocol: neutralProtocol, Target: TargetFor(s.Config, s.runtime.Directory), Agent: AgentFor(s.Config), RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: record.Workspace.Worktree, WorkspaceBranch: record.Workspace.Branch, BaseCommit: record.Workspace.Base, External: *identity, Generation: "generation"}
			require(t, s.commitWorkerBinding(run, ref))
			owner := &ownedStopResolver{ref: ref}
			s.lifecycleResolver = owner
			process := &recordedStopProcess{identity: *identity, beforeKill: func() { t.Fatal("host-owned stop invoked generic process kill") }}
			if scenario != "first-discard" {
				op, err := s.stopRunExecution(context.Background(), run, &Worker{proc: process})
				require(t, err)
				if op.Status != executionbackend.OperationUnknown || owner.stops != 1 {
					t.Fatalf("native lost reply not preserved: status=%s stops=%d reads=%d diagnostic=%s", op.Status, owner.stops, owner.reads, op.Diagnostic)
				}
				initialReads := owner.reads
				owner.proven = true
				if background {
					require(t, s.reconcileRunStop(context.Background(), run, op))
					saved, err := s.records.loadOperation(op.RunID, op.AttemptID, op.ID)
					require(t, err)
					if saved.Status != executionbackend.OperationConfirmed {
						t.Fatal("background recovery ignored native receipt")
					}
				} else {
					confirmed, err := s.confirmExecutionStop(op)
					require(t, err)
					if !confirmed {
						t.Fatal("confirmation used live OS process instead of original native proof")
					}
				}
				if owner.reads != initialReads+1 || owner.stops != 1 || process.kills != 0 {
					t.Fatal("stop recovery repeated mutation or bypassed owner")
				}
			} else {
				owner.proven = true
			}
			// Discard must settle through the durable native receipt even while the
			// borrowed OS process remains alive, including after losing local handles.
			record = s.executionRecord(run)
			record.Status = "active"
			s.workflowAttempt(run, run.AttemptID).Status = "running"
			require(t, s.commitWorker(run))
			task := nativeTaskRecord(s.domain.Tasks, run.TaskID)
			_, err = s.commandService(s.Config.identity).DiscardNativeTask(context.Background(), s.Config, task.TaskID, newID(), task.Version, "native stopped fixture", "operator")
			require(t, err)
			require(t, s.refreshNativeSnapshot(context.Background()))
			pending := nativeTaskRecord(s.domain.Tasks, run.TaskID)
			if len(pending.StopPending) == 0 {
				t.Fatal("discard fixture did not require original stop receipt")
			}
			if !background && scenario != "first-discard" {
				s.ownedProcesses = map[string]executionbackend.StopHandle{run.RunID: process}
			} else {
				s.ownedProcesses = nil
			}
			require(t, s.reconcileDiscardedExecutionStops(context.Background()))
			require(t, s.refreshNativeSnapshot(context.Background()))
			if pending = nativeTaskRecord(s.domain.Tasks, run.TaskID); len(pending.StopPending) != 0 {
				t.Fatalf("discard ignored native proof after terminal loss: %v; error=%s", pending.StopPending, deref(s.executionRecord(run).Error))
			}
			if owner.stops != 1 || process.kills != 0 {
				t.Fatal("discard repeated native stop or killed another process")
			}

		})
	}
}
