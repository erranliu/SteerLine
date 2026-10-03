package steerline

import (
	"bytes"
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"sync"
	"testing"
	"time"
)

type neutralPreparationResolver struct {
	mode            string
	turn            contracts.HostTurnFacts
	inputs          []string
	fact            contracts.HostFact
	feedback        []string
	questionPending bool
	newFault        bool
	loseReceipt     bool
	originalRequest *executionapi.LifecycleRequest
	originalReceipt executionapi.LifecycleReceipt
	reconciles      int
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
func (p neutralPreparationPort) ObserveHost(context.Context, executionapi.ProcessIdentity) (contracts.HostFact, error) {
	return p.resolver.fact, nil
}
func (p neutralPreparationPort) ObserveLifecycle(_ context.Context, s executionapi.LifecycleSnapshot) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := executionapi.LifecycleReceipt{Disposition: executionapi.OperationConfirmed, Process: &s.Ref.External, State: executionapi.LifecycleStateFacts{SessionID: s.Session.SessionID, SessionGeneration: s.Session.Generation, TurnID: p.resolver.turn.TurnID, TurnCompleted: p.resolver.turn.Settled, QuestionPending: p.resolver.questionPending}}
	if p.resolver.newFault {
		p.resolver.fact = contracts.HostFact{Reason: "tool_initialization_failed", Confidence: "confirmed", Source: "fixture-new-tool-failure", Turn: p.resolver.turn.TurnID}
		receipt.State.TurnCompleted = true
		p.resolver.turn.Settled = true
		p.resolver.turn.Outcome = "succeeded"
	}
	if receipt.State.TurnCompleted {
		receipt.TurnEvidence = &executionapi.TurnEvidence{Started: true, Completed: true, Succeeded: true, SessionID: s.Session.SessionID, TurnID: p.resolver.turn.TurnID, CommandID: p.resolver.turn.TurnID}
	}
	if !receipt.State.TurnCompleted {
		return receipt, newNeutralHoldObserver(s.Ref.External), nil
	}
	return receipt, nil, nil
}
func (p neutralPreparationPort) ReadWorkerOutput(ctx context.Context, s executionapi.LifecycleSnapshot, input contracts.HostReadInput) (contracts.HostOutput, executionapi.ImplementationDataEnvelope, error) {
	output, envelope, err := p.neutralOutputPort.ReadWorkerOutput(ctx, s, input)
	if p.resolver.newFault {
		fact := p.resolver.fact
		output.Fact = &fact
	}
	return output, envelope, err
}
func (p neutralPreparationPort) ReadProbe(id contracts.ProbeIdentity, digest string) (*contracts.ProbeReceipt, error) {
	if p.resolver.mode == "missing_receipt" {
		return nil, nil
	}
	return &contracts.ProbeReceipt{ProbeIdentity: id, InputSHA256: digest, GitHead: "fixture-committed-head", Read: true, Write: true, Delete: true}, nil
}
func (p neutralPreparationPort) Reconcile(_ context.Context, r executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	if p.resolver.originalRequest == nil {
		return p.LifecyclePort.Reconcile(context.Background(), r)
	}
	original := p.resolver.originalRequest
	if original.Operation.ID != r.Operation.ID || original.Operation.PayloadDigest != r.Operation.PayloadDigest || !bytes.Equal(original.Operation.Request, r.Operation.Request) || original.Snapshot.Ref != r.Snapshot.Ref {
		return executionapi.LifecycleReceipt{}, nil, errors.New("fixture reconcile changed original operation")
	}
	p.resolver.reconciles++
	receipt := p.resolver.originalReceipt.Clone()
	receipt.Operation = r.Operation
	receipt.Disposition = executionapi.OperationConfirmed
	return receipt, nil, nil
}
func (p neutralPreparationPort) Execute(ctx context.Context, r executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	if p.resolver.loseReceipt && r.Operation.Action == string(executionapi.LifecyclePrepareSession) {
		receipt, observer, err := p.LifecyclePort.Execute(ctx, r)
		if err != nil {
			return receipt, observer, err
		}
		original := r.Clone()
		p.resolver.originalRequest = &original
		p.resolver.originalReceipt = receipt.Clone()
		receipt.Disposition = executionapi.OperationUnknown
		receipt.RequestID = "fixture-original-request"
		p.resolver.loseReceipt = false
		return receipt, observer, nil
	}
	if r.Input.Feedback != nil {
		p.resolver.feedback = append(p.resolver.feedback, r.Input.Feedback.Message)
		return executionapi.LifecycleReceipt{Operation: r.Operation, Disposition: executionapi.OperationConfirmed, State: executionapi.LifecycleStateFacts{InputAccepted: true, TurnID: r.Snapshot.Session.CurrentTurnID}}, nil, nil
	}
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

type neutralHoldObserver struct {
	events chan executionapi.LifecycleEvent
	done   chan error
	handle neutralHandle
	once   sync.Once
}

func newNeutralHoldObserver(process executionapi.ProcessIdentity) *neutralHoldObserver {
	return &neutralHoldObserver{events: make(chan executionapi.LifecycleEvent), done: make(chan error, 1), handle: neutralHandle{identity: process}}
}
func (o *neutralHoldObserver) Events() <-chan executionapi.LifecycleEvent { return o.events }
func (o *neutralHoldObserver) Done() <-chan error                         { return o.done }
func (o *neutralHoldObserver) StopHandle() executionapi.StopHandle        { return o.handle }
func (o *neutralHoldObserver) Close(context.Context) error {
	o.once.Do(func() { close(o.events); close(o.done) })
	return nil
}
