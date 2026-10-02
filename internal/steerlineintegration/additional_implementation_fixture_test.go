package steerlineintegration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
)

const additionalKey = "fixture.stream"
const additionalProtocol executionapi.ExecutionProtocol = "fixture_stream_v1"

// This fake owns only protocol facts and its isolated external ledger. It has
// no Service, Store or Runtime handle and never commits business transitions.
type additionalPort struct {
	setup  executionapi.LifecycleSetup
	ledger *additionalLedger
}
type additionalLedger struct {
	mu        sync.Mutex
	path      string
	observers map[string]*additionalObserver
	reads     int
}
type additionalSaved struct {
	Session    string                                   `json:"session"`
	Generation string                                   `json:"generation"`
	Process    executionapi.ProcessIdentity             `json:"process"`
	Inputs     map[string]executionapi.TurnInput        `json:"inputs"`
	Receipts   map[string]executionapi.LifecycleReceipt `json:"receipts"`
	Answers    map[string]executionapi.FeedbackInput    `json:"answers"`
	Settled    bool                                     `json:"settled"`
}

func (l *additionalLedger) read() (additionalSaved, error) {
	var value additionalSaved
	data, err := os.ReadFile(l.path)
	if os.IsNotExist(err) {
		return additionalSaved{Inputs: map[string]executionapi.TurnInput{}, Receipts: map[string]executionapi.LifecycleReceipt{}, Answers: map[string]executionapi.FeedbackInput{}}, nil
	}
	if err != nil {
		return value, err
	}
	err = contracts.DecodeJSON(data, &value)
	return value, err
}
func (l *additionalLedger) write(value additionalSaved) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return AtomicWrite(l.path, data)
}
func additionalReport(ref executionapi.ExecutionRef) executionapi.CapabilityReport {
	return executionapi.CapabilityReport{Version: 1, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Agent: ref.Agent, PlatformVersion: "isolated-fake-v1", CheckedAt: time.Now().UTC(), Capabilities: []executionapi.Capability{
		{Name: "start", Status: executionapi.CapabilityVerified, Evidence: "isolated in-memory fixture; no native permission authentication"},
		{Name: "stop", Status: executionapi.CapabilityUnsupported},
	}}
}
func (p *additionalPort) Identity() (executionapi.BackendKind, executionapi.ExecutionProtocol) {
	return "fixture", additionalProtocol
}
func (p *additionalPort) Probe(_ context.Context, target executionapi.ExecutionTarget, agent executionapi.AgentSpec) (executionapi.CapabilityReport, error) {
	return additionalReport(executionapi.ExecutionRef{Backend: "fixture", Protocol: additionalProtocol, Target: target, Agent: agent}), nil
}
func (p *additionalPort) Observe(_ context.Context, ref executionapi.ExecutionRef) (executionapi.Observation, error) {
	if ref.Target.ID != "fixture:fixed" || ref.Protocol != additionalProtocol {
		return executionapi.Observation{}, errors.New("fixture target changed")
	}
	return executionapi.Observation{Liveness: "live", Connection: "connected", ObservedAt: time.Now().UTC(), Source: "isolated-fake"}, nil
}
func (p *additionalPort) InspectWorkspace(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error) {
	snapshot := p.setup.Snapshot.Clone()
	snapshot.Ref = ref
	return p.InspectLifecycleWorkspace(ctx, snapshot)
}
func (p *additionalPort) PreflightLifecycle(_ context.Context, setup executionapi.LifecycleSetup) (executionapi.CapabilityReport, error) {
	if setup.Policy.Filesystem.WriteScope != "none" || setup.Policy.Network != "deny" {
		return executionapi.CapabilityReport{}, errors.New("fixture supports only read-only, offline policy")
	}
	return additionalReport(setup.Snapshot.Ref), nil
}
func (p *additionalPort) PolicyCapabilities() executionapi.LifecyclePolicyCapabilities {
	return executionapi.LifecyclePolicyCapabilities{ResumeActiveObservation: true, ActiveTurnFeedback: true}
}
func (p *additionalPort) Next(_ context.Context, snapshot executionapi.LifecycleSnapshot) (executionapi.LifecycleCandidate, error) {
	if snapshot.Workspace.Path == "" {
		intent := snapshot.WorkspacePlan
		return executionapi.LifecycleCandidate{Action: executionapi.LifecyclePrepareWorkspace, Input: executionapi.LifecycleInput{Workspace: &intent}}, nil
	}
	if snapshot.PendingTurn == nil {
		return executionapi.LifecycleCandidate{Done: true}, nil
	}
	turn := *snapshot.PendingTurn
	action := executionapi.LifecycleStartTurn
	if snapshot.Session.SessionID == "" {
		action = executionapi.LifecyclePrepareSession
	}
	return executionapi.LifecycleCandidate{Action: action, Input: executionapi.LifecycleInput{Turn: &turn}}, nil
}
func (p *additionalPort) Execute(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationUnknown}
	if err := request.Validate(); err != nil {
		return receipt, nil, err
	}
	if request.Snapshot.Ref.Target.ID != "fixture:fixed" || request.Snapshot.Ref.Protocol != additionalProtocol {
		return receipt, nil, errors.New("fixture refused replacement target")
	}
	if request.Input.Workspace != nil {
		git := workspace.Git{Command: executionapi.LaunchCommand{Executable: p.setup.GitExecutable, Args: p.setup.GitArgs}, Timeout: p.setup.CommandTimeout, Start: p.setup.ProcessHost.StartManaged}
		result, err := workspace.Prepare(ctx, git, workspace.Repository{Path: p.setup.RepositoryPath, CommonDir: request.Snapshot.Session.ProjectID}, filepath.Dir(request.Input.Workspace.Path), request.Input.Workspace)
		receipt.Disposition, receipt.Workspace = result.Status, result.Inspection
		return receipt, nil, err
	}
	p.ledger.mu.Lock()
	defer p.ledger.mu.Unlock()
	value, err := p.ledger.read()
	if err != nil {
		return receipt, nil, err
	}
	if _, exists := value.Receipts[request.Operation.ID]; exists {
		return receipt, nil, errors.New("mutation offered twice instead of reconciliation")
	}
	switch executionapi.LifecycleAction(request.Operation.Action) {
	case executionapi.LifecyclePrepareSession:
		if value.Session != "" {
			return receipt, nil, errors.New("fixture session already exists")
		}
		value.Session, value.Generation = "fixture-original-session", request.Snapshot.ResourceGeneration
		value.Process = executionapi.ProcessIdentity{Kind: "fixture", RuntimeID: "fixture-runtime", Terminal: "fixture-resource", Incarnation: "fixture-original"}
		receipt.Disposition = executionapi.OperationConfirmed
		receipt.Process = &value.Process
		receipt.State = executionapi.LifecycleStateFacts{SessionID: value.Session, SessionGeneration: value.Generation}
	case executionapi.LifecycleStartTurn:
		if value.Session != request.Snapshot.Session.SessionID || value.Generation != request.Snapshot.Session.Generation {
			return receipt, nil, errors.New("fixture lost original session/generation")
		}
		turn := *request.Input.Turn
		if _, exists := value.Inputs[turn.CommandID]; exists {
			return receipt, nil, errors.New("original input was resent")
		}
		value.Inputs[turn.CommandID] = turn
		receipt.Disposition = executionapi.OperationAccepted
		receipt.Process = &value.Process
		receipt.State = executionapi.LifecycleStateFacts{SessionID: value.Session, SessionGeneration: value.Generation, TurnID: turn.CommandID, InputAccepted: true, TurnStarted: true}
	case executionapi.LifecycleDeliverFeedback:
		if request.Snapshot.QuestionRoute == nil || request.Input.Feedback.QuestionID != request.Snapshot.QuestionRoute.QuestionID {
			return receipt, nil, errors.New("fixture answer changed original question")
		}
		value.Answers[request.Input.Feedback.QuestionID] = *request.Input.Feedback
		receipt.Disposition = executionapi.OperationConfirmed
		receipt.State = executionapi.LifecycleStateFacts{InputAccepted: true, TurnID: request.Snapshot.Session.CurrentTurnID}
	default:
		return receipt, nil, executionapi.UnsupportedAdapterOperation(request.Operation.Action, "fixture capability is unsupported")
	}
	value.Receipts[request.Operation.ID] = receipt.Clone()
	if err := p.ledger.write(value); err != nil {
		return receipt, nil, err
	}
	if request.Input.Feedback != nil {
		if observer := p.ledger.observers[request.Snapshot.Session.ExecutionID]; observer != nil {
			observer.finish()
		}
		return receipt, nil, nil
	}
	if executionapi.LifecycleAction(request.Operation.Action) == executionapi.LifecycleStartTurn {
		return receipt, p.observer(request.Snapshot, value, receipt.State.TurnID), nil
	}
	return receipt, nil, nil
}
func (p *additionalPort) Reconcile(_ context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	p.ledger.mu.Lock()
	defer p.ledger.mu.Unlock()
	p.ledger.reads++
	value, err := p.ledger.read()
	if err != nil {
		return executionapi.LifecycleReceipt{}, nil, err
	}
	receipt, exists := value.Receipts[request.Operation.ID]
	if !exists {
		return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationUnknown}, nil, nil
	}
	if err := receipt.ValidateFor(request); err != nil {
		return receipt, nil, err
	}
	return receipt.Clone(), nil, nil
}
func (p *additionalPort) ObserveLifecycle(_ context.Context, snapshot executionapi.LifecycleSnapshot) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	p.ledger.mu.Lock()
	defer p.ledger.mu.Unlock()
	p.ledger.reads++
	value, err := p.ledger.read()
	if err != nil {
		return executionapi.LifecycleReceipt{}, nil, err
	}
	if snapshot.Session.SessionID != value.Session || snapshot.Session.Generation != value.Generation || snapshot.Ref.External != value.Process {
		return executionapi.LifecycleReceipt{}, nil, errors.New("fixture observation replaced original identity")
	}
	if _, exists := value.Inputs[snapshot.Session.CurrentTurnID]; !exists {
		return executionapi.LifecycleReceipt{}, nil, errors.New("fixture observation has no persistent original input")
	}
	return executionapi.LifecycleReceipt{Disposition: executionapi.OperationConfirmed, Process: &value.Process, State: executionapi.LifecycleStateFacts{SessionID: value.Session, SessionGeneration: value.Generation, TurnID: snapshot.Session.CurrentTurnID}}, p.observer(snapshot, value, snapshot.Session.CurrentTurnID), nil
}
func (p *additionalPort) VerifyManualRecovery(_ context.Context, snapshot executionapi.LifecycleSnapshot) (executionapi.LifecycleRecoveryEvidence, error) {
	p.ledger.mu.Lock()
	defer p.ledger.mu.Unlock()
	p.ledger.reads++
	value, err := p.ledger.read()
	if err != nil || value.Session != snapshot.Session.SessionID || value.Process != snapshot.Ref.External {
		return executionapi.LifecycleRecoveryEvidence{}, errors.New("fixture recovery cannot verify original identity")
	}
	return executionapi.LifecycleRecoveryEvidence{SessionAvailable: true, ManagedProcess: true}, nil
}
func (p *additionalPort) BindRecovery(context.Context, executionapi.LifecycleSnapshot, executionapi.LifecycleSnapshot) (executionapi.ImplementationDataEnvelope, error) {
	return executionapi.ImplementationDataEnvelope{}, errors.New("fixture does not replace sessions")
}
func (p *additionalPort) InspectLifecycleWorkspace(ctx context.Context, snapshot executionapi.LifecycleSnapshot) (executionapi.WorkspaceInspection, error) {
	return workspace.InspectLifecycleWorkspace(ctx, p.setup, snapshot)
}
func (p *additionalPort) StopLifecycle(_ context.Context, request executionapi.LifecycleRequest, _ executionapi.StopHandle) (executionapi.LifecycleReceipt, error) {
	return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationRejected}, errors.New("fixture cancellation is unsupported")
}
func (p *additionalPort) CleanupLifecycle(_ context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationConfirmed, State: executionapi.LifecycleStateFacts{WorkerAbsent: true, WritersAbsent: true}}, nil
}

type additionalHandle struct{ identity executionapi.ProcessIdentity }

func (h additionalHandle) Identity() executionapi.ProcessIdentity { return h.identity }
func (additionalHandle) Kill()                                    {}
func (additionalHandle) Close()                                   {}

type additionalObserver struct {
	events     chan executionapi.LifecycleEvent
	done       chan error
	closed     chan struct{}
	completed  chan struct{}
	once       sync.Once
	handle     additionalHandle
	turn       string
	finishOnce sync.Once
}

func (o *additionalObserver) Events() <-chan executionapi.LifecycleEvent { return o.events }
func (o *additionalObserver) Done() <-chan error                         { return o.done }
func (o *additionalObserver) StopHandle() executionapi.StopHandle        { return o.handle }
func (o *additionalObserver) Close(context.Context) error {
	o.once.Do(func() { close(o.closed) })
	return nil
}
func (o *additionalObserver) finish() {
	o.finishOnce.Do(func() { close(o.completed) })
}
func (p *additionalPort) observer(snapshot executionapi.LifecycleSnapshot, value additionalSaved, turn string) *additionalObserver {
	o := &additionalObserver{events: make(chan executionapi.LifecycleEvent, 8), done: make(chan error, 1), closed: make(chan struct{}), completed: make(chan struct{}), handle: additionalHandle{value.Process}, turn: turn}
	p.ledger.observers[snapshot.Session.ExecutionID] = o
	route, err := executionapi.NewImplementationDataEnvelope(executionapi.ImplementationDataEnvelope{ImplementationKey: additionalKey, CodecVersion: 1, Purpose: executionapi.ImplementationQuestionPurpose, ProjectID: snapshot.Session.ProjectID, WorkflowRunID: snapshot.Session.WorkflowRunID, ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID, Generation: value.Generation, TurnID: turn, QuestionID: "fixture-question"}, []byte(`{"channel":"fixture"}`))
	if err != nil {
		o.done <- err
		close(o.events)
		return o
	}
	question := executionapi.Question{ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID, Generation: value.Generation, TurnID: turn, ID: "fixture-choice", Header: "Fixture", Question: "Continue the original input?", Options: []executionapi.DecisionOption{{ID: "continue", Label: "Continue"}}, AllowFreeText: true}
	go func() {
		defer close(o.events)
		for _, fact := range []executionapi.EventFact{{Kind: executionapi.EventStarted, SessionID: value.Session, StartedTurnID: turn}, {Kind: executionapi.EventQuestion, QuestionRoute: &route, Questions: []executionapi.Question{question}}} {
			ack := make(chan error, 1)
			select {
			case o.events <- executionapi.LifecycleEvent{Fact: fact, Persisted: ack}:
			case <-o.closed:
				return
			}
			select {
			case err := <-ack:
				if err != nil {
					o.done <- err
					return
				}
			case <-o.closed:
				return
			}
		}
		if _, answered := value.Answers[route.QuestionID]; answered {
			o.finish()
		}
		select {
		case <-o.completed:
		case <-o.closed:
			return
		}
		ack := make(chan error, 1)
		select {
		case o.events <- executionapi.LifecycleEvent{Fact: executionapi.EventFact{Kind: executionapi.EventCompleted, Completed: true, Matched: true, Outcome: "completed"}, Persisted: ack}:
		case <-o.closed:
			return
		}
		select {
		case err := <-ack:
			o.done <- err
		case <-o.closed:
			return
		}
	}()
	return o
}

// Sources are rebuilt independently on restart. This implementation has no
// journal/history stream; its lifecycle facts still use the production mapper
// and event commit ports. It declares this limitation instead of inventing data.
type additionalSources struct{}

func (additionalSources) MapConversation(contracts.ConversationInput) (contracts.ConversationMappedBatch, error) {
	return contracts.ConversationMappedBatch{}, nil
}
func (additionalSources) RestoreMapper(contracts.ConversationMapperSnapshot) contracts.ConversationMapper {
	return nil
}
func (additionalSources) JournalSources() []string { return nil }
func (additionalSources) JournalSource(executionapi.BackendKind, executionapi.ExecutionProtocol) string {
	return ""
}
func (additionalSources) JournalSegment(executionapi.BackendKind, executionapi.ExecutionProtocol, string) string {
	return ""
}
func (additionalSources) JournalGeneration(string, string) (uint64, error) { return 0, nil }
func (additionalSources) LiveJournalPath(context.Context, contracts.ConversationJournalPathRequest) (string, error) {
	return "", nil
}
func (additionalSources) DiscoverHistory(context.Context, contracts.ConversationDiscovery) (contracts.ConversationDiscoveryResult, error) {
	return contracts.ConversationDiscoveryResult{}, nil
}
func (additionalSources) DiscoverJournals(context.Context, contracts.ConversationDiscovery, func(string) (string, error)) ([]contracts.ConversationJournalSource, error) {
	return nil, nil
}
func (additionalSources) ReadHistory(context.Context, contracts.ConversationHistoryReadRequest, func(*exec.Cmd) (executionapi.StopHandle, error)) (contracts.ConversationHistoryBatch, error) {
	return contracts.ConversationHistoryBatch{}, fmt.Errorf("fixture history reading is unsupported")
}
