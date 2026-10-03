package steerline

import (
	"context"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"testing"
	"time"
)

type neutralPreparationResolver struct {
	mode   string
	turn   contracts.HostTurnFacts
	inputs []string
}
type neutralPreparationPort struct {
	neutralOutputPort
	resolver *neutralPreparationResolver
}

func (r *neutralPreparationResolver) ResolveLifecycle(setup executionapi.LifecycleSetup) (executionapi.LifecyclePort, error) {
	return neutralPreparationPort{neutralOutputPort: neutralOutputPort{LifecyclePort: &neutralLifecycle{setup: setup.Clone()}}, resolver: r}, nil
}
func (p neutralPreparationPort) CurrentTurn(executionapi.LifecycleSnapshot) (*contracts.HostTurnFacts, error) {
	turn := p.resolver.turn
	return &turn, nil
}
func (p neutralPreparationPort) ReadProbe(id contracts.ProbeIdentity, digest string) (*contracts.ProbeReceipt, error) {
	if p.resolver.mode == "missing_receipt" {
		return nil, nil
	}
	return &contracts.ProbeReceipt{ProbeIdentity: id, InputSHA256: digest, GitHead: "fixture-committed-head", Read: true, Write: true, Delete: true}, nil
}
func (p neutralPreparationPort) Execute(ctx context.Context, r executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	if r.Input.Host != nil {
		return executionapi.LifecycleReceipt{Operation: r.Operation, Disposition: executionapi.OperationConfirmed}, nil, nil
	}
	if r.Operation.Action == string(executionapi.LifecycleStartTurn) && r.Input.Turn != nil {
		p.resolver.inputs = append(p.resolver.inputs, r.Input.Turn.CommandID)
	}
	return p.LifecyclePort.Execute(ctx, r)
}

// The fake returns only already observed probe/settlement facts. The center
// owns every Decision, input intent, business turn and persistence write.
func neutralPreparationService(t *testing.T, mode string) (*Service, *Run, *neutralPreparationResolver) {
	t.Helper()
	s, run := neutralAuditService(t)
	require(t, prepareWorktreeOnly(context.Background(), s.Config, run))
	run.WorktreePath, run.WorktreeBranch, run.BaseCommit = ptr(run.WorkspaceIntent.Path), ptr(run.WorkspaceIntent.Branch), ptr(run.WorkspaceIntent.BaseCommit)
	workflow, e := s.workflowRecords(run)
	inspected, err := inspectTestWorkspace(context.Background(), s.Config, executionapi.ExecutionRef{WorkspacePath: run.WorkspaceIntent.Path, WorkspaceBranch: run.WorkspaceIntent.Branch, BaseCommit: run.WorkspaceIntent.BaseCommit})
	require(t, err)
	applyWorkflowWorkspaceEvidence(run, e, WorkspaceEvidence{Repository: inspected.Repository, Worktree: inspected.Worktree, Branch: inspected.Branch, Base: inspected.Base, Head: inspected.Head, Dirty: inspected.Dirty, VerifiedAt: inspected.CheckedAt})
	e.Process = &executionapi.ProcessIdentity{Kind: "fixture", RuntimeID: "fixture-runtime", Terminal: "fixture-terminal", Incarnation: "fixture-incarnation"}
	setNeutralBinding(t, e, "fixture-original", e.Process.Incarnation, "fixture-session")
	e.PolicyResources = neutralPolicyReceipt(workflow.ExecutionProfile.Digest, e.Workspace.Worktree, e)
	id := newID()
	started := now()
	probe := executionapi.ProbeIdentity{Execution: e.ExecutionID, Turn: id, Nonce: newID(), ProfileDigest: workflow.ExecutionProfile.Digest, Runtime: e.Process.RuntimeID, Terminal: e.Process.Terminal, Incarnation: e.Process.Incarnation, Worktree: e.Workspace.Worktree}
	evidence := contracts.ProbeToolEvidence{SourceIdentity: "fixture-probe-source", CallID: "fixture-call", MessageID: "fixture-result", Command: "fixture-probe", Marker: "fixture-marker", Verified: mode != "oral" && mode != "tool_failure"}
	e.Environment = &contracts.WorkerEnvironment{OriginalTerminal: e.Process, Runtime: e.Process.RuntimeID, LaunchCommand: "fixture launch", WriteRoots: append([]string(nil), e.PolicyResources.WriteRoots...), ProfileDigest: workflow.ExecutionProfile.Digest, Preparation: &contracts.WorkerPreparation{TurnID: id, Nonce: probe.Nonce, StartedAt: started, Deadline: started.Add(2 * time.Minute), Probe: probe, Command: "fixture-probe", InputDigest: "fixture-input", ScriptDigest: "fixture-script", Evidence: evidence}}
	resolver := &neutralPreparationResolver{mode: mode, turn: contracts.HostTurnFacts{TurnID: id, TaskID: "fixture-task", Purpose: "readiness", Outcome: "succeeded", IntentAt: started, Deadline: started.Add(2 * time.Minute), Settled: true, HasDispatch: true}}
	if mode == "failed_settlement" {
		resolver.turn.Outcome = "failed"
	}
	e.PendingTurnID, e.PendingCommandID = id, id
	s.lifecycleResolver = resolver
	s.refreshRun(run)
	require(t, s.commitWorker(run))
	t.Cleanup(func() { s.shutdown() })
	return s, run, resolver
}

func TestWorkerEnvironmentER1ProbeAndSettlementGate(t *testing.T) {
	for _, mode := range []string{"success", "tool_failure", "oral", "missing_receipt", "failed_settlement"} {
		t.Run(mode, func(t *testing.T) {
			s, run, resolver := neutralPreparationService(t, mode)
			workflow, e := s.workflowRecords(run)
			if mode == "tool_failure" {
				require(t, s.blockWorkerEnvironment(run, contracts.HostFact{Reason: "tool_initialization_failed", Confidence: "confirmed", Source: "fixture-tool", Turn: resolver.turn.TurnID}))
			} else {
				require(t, s.finishWorkerPreparation(context.Background(), run, e, &resolver.turn, resolver.turn.Outcome))
			}
			workflow, e = s.workflowRecords(run)
			if mode == "success" {
				if !e.Environment.Ready || !e.Environment.Preparation.Verified || e.TurnNumber != 0 || e.CurrentTurnID != "" || len(resolver.inputs) != 1 || resolver.inputs[0] == resolver.turn.TurnID {
					t.Fatalf("preparation leaked business state or lost its separate input boundary: workflow=%+v execution=%+v inputs=%v", workflow, e, resolver.inputs)
				}
				worker := s.workers[run.RunID]
				if worker == nil {
					t.Fatal("business input has no independent observer")
				}
				event := <-worker.Events
				require(t, s.eventForRun(run, event))
				if e.TurnNumber != 1 || e.CurrentTurnID != resolver.inputs[0] {
					t.Fatal("business start was not independently observed")
				}
			} else {
				if e.Environment.Ready || e.TurnNumber != 0 || len(resolver.inputs) != 0 {
					t.Fatal("failed readiness dispatched business")
				}
				if mode == "tool_failure" || mode == "failed_settlement" {
					if workflow.Phase != "waiting_decision" || e.Environment.Blocker == nil {
						t.Fatal("confirmed readiness failure lost its center decision")
					}
				}
			}
		})
	}
}

func TestManagedFrozenResourceEvidenceIsVerifiedBeforeInput(t *testing.T) {
	s, run, _ := neutralPreparationService(t, "success")
	e := s.executionRecord(run)
	e.Environment.WriteRoots = append(e.Environment.WriteRoots, "C:/")
	if err := s.verifyHostExecutionEvidence(context.Background(), run, e); err == nil {
		t.Fatal("changed writable roots were accepted")
	}
}
