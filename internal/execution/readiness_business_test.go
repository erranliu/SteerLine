package execution

import (
	"context"
	"sync"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

type neutralPreparationResolver struct {
	mode            string
	turn            executionbackend.HostTurnFacts
	inputs          []string
	fact            executionbackend.HostFact
	feedback        []string
	questionPending bool
	newFault        bool
}

type neutralPreparationPort struct {
	neutralOutputPort
	resolver *neutralPreparationResolver
}

func (r *neutralPreparationResolver) ResolveLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	return neutralPreparationPort{neutralOutputPort: neutralOutputPort{LifecyclePort: &neutralLifecycle{Setup: setup.Clone()}}, resolver: r}, nil
}

func (p neutralPreparationPort) CurrentTurn(executionbackend.LifecycleSnapshot) (*executionbackend.HostTurnFacts, error) {
	turn := p.resolver.turn
	return &turn, nil
}

func (p neutralPreparationPort) ObserveHost(context.Context, executionbackend.ProcessIdentity) (executionbackend.HostFact, error) {
	return p.resolver.fact, nil
}

func (p neutralPreparationPort) ObserveLifecycle(_ context.Context, s executionbackend.LifecycleSnapshot) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := executionbackend.LifecycleReceipt{Disposition: executionbackend.OperationConfirmed, Process: &s.Ref.External, State: executionbackend.LifecycleStateFacts{SessionID: s.Session.SessionID, SessionGeneration: s.Session.Generation, TurnID: p.resolver.turn.TurnID, TurnCompleted: p.resolver.turn.Settled, QuestionPending: p.resolver.questionPending}}
	if p.resolver.newFault {
		p.resolver.fact = executionbackend.HostFact{Reason: "tool_initialization_failed", Confidence: "confirmed", Source: "fixture-new-tool-failure", Turn: p.resolver.turn.TurnID}
		receipt.State.TurnCompleted = true
		p.resolver.turn.Settled = true
		p.resolver.turn.Outcome = "succeeded"
	}
	if receipt.State.TurnCompleted {
		receipt.TurnEvidence = &executionbackend.TurnEvidence{Started: true, Completed: true, Succeeded: true, SessionID: s.Session.SessionID, TurnID: p.resolver.turn.TurnID, CommandID: p.resolver.turn.TurnID}
	}
	if !receipt.State.TurnCompleted {
		return receipt, newNeutralHoldObserver(s.Ref.External), nil
	}
	return receipt, nil, nil
}

func (p neutralPreparationPort) ReadWorkerOutput(ctx context.Context, s executionbackend.LifecycleSnapshot, input executionbackend.HostReadInput) (executionbackend.HostOutput, executionbackend.ImplementationDataEnvelope, error) {
	output, envelope, err := p.neutralOutputPort.ReadWorkerOutput(ctx, s, input)
	if p.resolver.newFault {
		fact := p.resolver.fact
		output.Fact = &fact
	}
	return output, envelope, err
}

func (p neutralPreparationPort) ReadProbe(id executionbackend.ProbeIdentity, digest string) (*executionbackend.ProbeReceipt, error) {
	if p.resolver.mode == "missing_receipt" {
		return nil, nil
	}
	return &executionbackend.ProbeReceipt{ProbeIdentity: id, InputSHA256: digest, GitHead: "fixture-committed-head", Read: true, Write: true, Delete: true}, nil
}

func (p neutralPreparationPort) Execute(ctx context.Context, r executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if r.Input.Feedback != nil {
		p.resolver.feedback = append(p.resolver.feedback, r.Input.Feedback.Message)
		return executionbackend.LifecycleReceipt{Operation: r.Operation, Disposition: executionbackend.OperationConfirmed, State: executionbackend.LifecycleStateFacts{InputAccepted: true, TurnID: r.Snapshot.Session.CurrentTurnID}}, nil, nil
	}
	if r.Input.Host != nil {
		return executionbackend.LifecycleReceipt{Operation: r.Operation, Disposition: executionbackend.OperationConfirmed}, nil, nil
	}
	if r.Operation.Action == string(executionbackend.LifecycleStartTurn) && r.Input.Turn != nil {
		p.resolver.inputs = append(p.resolver.inputs, r.Input.Turn.CommandID)
	}
	return p.LifecyclePort.Execute(ctx, r)
}

// The fake returns only already observed probe/settlement facts. The center
// owns every Decision, input intent, business turn and persistence write.

func neutralPreparationService(t *testing.T, mode string) (*Service, *reconstructiontypes.RunView, *neutralPreparationResolver) {
	t.Helper()
	s, run := neutralAuditService(t)
	require(t, prepareWorktreeOnly(context.Background(), s.Config, s.runtime.Directory, s.runtime.ProcessHost, run))
	run.WorktreePath, run.WorktreeBranch, run.BaseCommit = ptr(run.WorkspaceIntent.Path), ptr(run.WorkspaceIntent.Branch), ptr(run.WorkspaceIntent.BaseCommit)
	workflow, e := s.workflowRecords(run)
	inspected, err := inspectTestWorkspace(context.Background(), s.Config, executionbackend.ExecutionRef{WorkspacePath: run.WorkspaceIntent.Path, WorkspaceBranch: run.WorkspaceIntent.Branch, BaseCommit: run.WorkspaceIntent.BaseCommit})
	require(t, err)
	applyWorkflowWorkspaceEvidence(run, e, executionmodel.WorkspaceEvidence{Repository: inspected.Repository, Worktree: inspected.Worktree, Branch: inspected.Branch, Base: inspected.Base, Head: inspected.Head, Dirty: inspected.Dirty, VerifiedAt: inspected.CheckedAt})
	e.Process = &executionbackend.ProcessIdentity{Kind: "fixture", RuntimeID: "fixture-runtime", Terminal: "fixture-terminal", Incarnation: "fixture-incarnation"}
	setNeutralBinding(t, e, "fixture-original", e.Process.Incarnation, "fixture-session")
	e.PolicyResources = neutralPolicyReceipt(workflow.ExecutionProfile.Digest, e.Workspace.Worktree, e)

	id := newID()
	started := now()
	probe := executionbackend.ProbeIdentity{Execution: e.ExecutionID, Turn: id, Nonce: newID(), ProfileDigest: workflow.ExecutionProfile.Digest, Runtime: e.Process.RuntimeID, Terminal: e.Process.Terminal, Incarnation: e.Process.Incarnation, Worktree: e.Workspace.Worktree}
	evidence := executionbackend.ProbeToolEvidence{SourceIdentity: "fixture-probe-source", CallID: "fixture-call", MessageID: "fixture-result", Command: "fixture-probe", Marker: "fixture-marker", Verified: mode != "oral" && mode != "tool_failure"}
	e.Environment = &executionmodel.WorkerEnvironment{OriginalTerminal: e.Process, Runtime: e.Process.RuntimeID, LaunchCommand: "fixture launch", WriteRoots: append([]string(nil), e.PolicyResources.WriteRoots...), ProfileDigest: workflow.ExecutionProfile.Digest, Preparation: &executionmodel.WorkerPreparation{TurnID: id, Nonce: probe.Nonce, StartedAt: started, Deadline: started.Add(2 * time.Minute), Probe: probe, Command: "fixture-probe", InputDigest: "fixture-input", ScriptDigest: "fixture-script", Evidence: evidence}}
	resolver := &neutralPreparationResolver{mode: mode, turn: executionbackend.HostTurnFacts{TurnID: id, TaskID: "fixture-task", Purpose: "readiness", Outcome: "succeeded", IntentAt: started, Deadline: started.Add(2 * time.Minute), Settled: true, HasDispatch: true}}
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
				require(t, s.blockWorkerEnvironment(run, executionbackend.HostFact{Reason: "tool_initialization_failed", Confidence: "confirmed", Source: "fixture-tool", Turn: resolver.turn.TurnID}))
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
					if workflow.Status != "waiting_input" || e.Environment.Blocker == nil {
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

type neutralHandle struct {
	identity executionbackend.ProcessIdentity
}

func (h neutralHandle) Identity() executionbackend.ProcessIdentity { return h.identity }
func (neutralHandle) Kill()                                        {}
func (neutralHandle) Close()                                       {}

type neutralHoldObserver struct {
	events chan executionbackend.LifecycleEvent
	done   chan error
	handle neutralHandle
	once   sync.Once
}

func newNeutralHoldObserver(process executionbackend.ProcessIdentity) *neutralHoldObserver {
	return &neutralHoldObserver{events: make(chan executionbackend.LifecycleEvent), done: make(chan error, 1), handle: neutralHandle{identity: process}}
}

func (o *neutralHoldObserver) Events() <-chan executionbackend.LifecycleEvent { return o.events }

func (o *neutralHoldObserver) Done() <-chan error { return o.done }

func (o *neutralHoldObserver) StopHandle() executionbackend.StopHandle { return o.handle }

func (o *neutralHoldObserver) Close(context.Context) error {
	o.once.Do(func() { close(o.events); close(o.done) })
	return nil
}
