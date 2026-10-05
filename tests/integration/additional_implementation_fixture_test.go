package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	executionapp "github.com/erranliu/SteerLine/internal/execution"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
)

const additionalKey = "fixture.stream"

const additionalProtocol executionbackend.ExecutionProtocol = "fixture_stream_v1"

// This fake owns only protocol facts and its isolated external ledger. It has
// no Service, Store or Runtime handle and never commits business transitions.
type additionalPort struct {
	setup  executionbackend.LifecycleSetup
	ledger *additionalLedger
}

type additionalLedger struct {
	mu             sync.Mutex
	path           string
	observers      map[string]*additionalObserver
	reads          int
	publicationURL string
}

type additionalSaved struct {
	Session      string                                       `json:"session"`
	Generation   string                                       `json:"generation"`
	Process      executionbackend.ProcessIdentity             `json:"process"`
	Inputs       map[string]executionbackend.TurnInput        `json:"inputs"`
	Receipts     map[string]executionbackend.LifecycleReceipt `json:"receipts"`
	Answers      map[string]executionbackend.FeedbackInput    `json:"answers"`
	Settled      bool                                         `json:"settled"`
	CleanupCount int                                          `json:"cleanup_count"`
	PRCandidates []string                                     `json:"pr_candidates,omitempty"`
}

type additionalBinding struct {
	Session    string                           `json:"session"`
	Generation string                           `json:"generation"`
	Process    executionbackend.ProcessIdentity `json:"process"`
	Target     executionbackend.ExecutionTarget `json:"target"`
}

func (l *additionalLedger) read() (additionalSaved, error) {
	var value additionalSaved
	data, err := os.ReadFile(l.path)
	if os.IsNotExist(err) {
		return additionalSaved{Inputs: map[string]executionbackend.TurnInput{}, Receipts: map[string]executionbackend.LifecycleReceipt{}, Answers: map[string]executionbackend.FeedbackInput{}}, nil
	}
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(data, &value)
	return value, err
}

func (l *additionalLedger) write(value additionalSaved) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return AtomicWrite(l.path, data)
}

func additionalReport(ref executionbackend.ExecutionRef) executionbackend.CapabilityReport {
	return executionbackend.CapabilityReport{Version: 1, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Agent: ref.Agent, PlatformVersion: "isolated-fake-v1", CheckedAt: time.Now().UTC(), Capabilities: []executionbackend.Capability{
		{Name: "start", Status: executionbackend.CapabilityVerified, Evidence: "isolated in-memory fixture; no native permission authentication"},
		{Name: "stop", Status: executionbackend.CapabilityUnsupported},
	}}
}

func (p *additionalPort) Identity() (executionbackend.BackendKind, executionbackend.ExecutionProtocol) {
	return "fixture", additionalProtocol
}

func (p *additionalPort) Probe(_ context.Context, target executionbackend.ExecutionTarget, agent executionbackend.AgentSpec) (executionbackend.CapabilityReport, error) {
	return additionalReport(executionbackend.ExecutionRef{Backend: "fixture", Protocol: additionalProtocol, Target: target, Agent: agent}), nil
}

func (p *additionalPort) Observe(_ context.Context, ref executionbackend.ExecutionRef) (executionbackend.Observation, error) {
	if ref.Target.ID != "fixture:fixed" || ref.Protocol != additionalProtocol {
		return executionbackend.Observation{}, errors.New("fixture target changed")
	}
	return executionbackend.Observation{Liveness: "live", Connection: "connected", ObservedAt: time.Now().UTC(), Source: "isolated-fake"}, nil
}

func (p *additionalPort) InspectWorkspace(ctx context.Context, ref executionbackend.ExecutionRef) (executionbackend.WorkspaceInspection, error) {
	snapshot := p.setup.Snapshot.Clone()
	snapshot.Ref = ref
	return p.InspectLifecycleWorkspace(ctx, snapshot)
}

func (p *additionalPort) PreflightLifecycle(_ context.Context, setup executionbackend.LifecycleSetup) (executionbackend.CapabilityReport, error) {
	if setup.Policy.Filesystem.WriteScope != "none" || setup.Policy.Network != "deny" {
		return executionbackend.CapabilityReport{}, errors.New("fixture supports only read-only, offline policy")
	}
	return additionalReport(setup.Snapshot.Ref), nil
}

func (p *additionalPort) PolicyCapabilities() executionbackend.LifecyclePolicyCapabilities {
	// The injected builder binds a read-only policy envelope with no write
	// roots. Session creation still needs this original resource receipt.
	return executionbackend.LifecyclePolicyCapabilities{LinkedWorkspaceWriteRoots: true, ResumeActiveObservation: true, ActiveTurnFeedback: true}
}

func (p *additionalPort) Next(_ context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleCandidate, error) {
	if snapshot.TaskDelivered {
		p.ledger.mu.Lock()
		value, err := p.ledger.read()
		p.ledger.mu.Unlock()
		if err != nil {
			return executionbackend.LifecycleCandidate{}, err
		}
		if value.CleanupCount != 0 {
			return executionbackend.LifecycleCandidate{Done: true}, nil
		}
		return executionbackend.LifecycleCandidate{Action: executionbackend.LifecycleCleanup, Input: executionbackend.LifecycleInput{Cleanup: &executionbackend.CleanupInput{Resources: []executionbackend.ProcessIdentity{snapshot.Ref.External}}}}, nil
	}
	if snapshot.Workspace.Path == "" {
		intent := snapshot.WorkspacePlan
		return executionbackend.LifecycleCandidate{Action: executionbackend.LifecyclePrepareWorkspace, Input: executionbackend.LifecycleInput{Workspace: &intent}}, nil
	}
	if snapshot.PendingTurn == nil {
		return executionbackend.LifecycleCandidate{Done: true}, nil
	}
	turn := *snapshot.PendingTurn
	action := executionbackend.LifecycleStartTurn
	if snapshot.Session.SessionID == "" {
		action = executionbackend.LifecyclePrepareSession
	}
	return executionbackend.LifecycleCandidate{Action: action, Input: executionbackend.LifecycleInput{Turn: &turn}}, nil
}

func (p *additionalPort) Execute(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown}
	if err := request.Validate(); err != nil {
		return receipt, nil, err
	}
	if request.Snapshot.Ref.Target.ID != "fixture:fixed" || request.Snapshot.Ref.Protocol != additionalProtocol {
		return receipt, nil, errors.New("fixture refused replacement target")
	}
	if request.Input.Workspace != nil {
		git := workspace.Git{Command: executionbackend.LaunchCommand{Executable: p.setup.GitExecutable, Args: p.setup.GitArgs}, Timeout: p.setup.CommandTimeout, Start: p.setup.ProcessHost.StartManaged}
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
	switch executionbackend.LifecycleAction(request.Operation.Action) {
	case executionbackend.LifecyclePrepareSession:
		if value.Session != "" {
			return receipt, nil, errors.New("fixture session already exists")
		}
		value.Session, value.Generation = "fixture-original-session", request.Snapshot.ResourceGeneration
		value.Process = executionbackend.ProcessIdentity{Kind: "fixture", RuntimeID: "fixture-runtime", Terminal: "fixture-resource", Incarnation: "fixture-original"}
		binding, bindingErr := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{
			ImplementationKey: additionalKey, CodecVersion: 1, Purpose: executionbackend.ImplementationResourcePurpose,
			ProjectID: request.Snapshot.Session.ProjectID, ExecutionID: request.Snapshot.Session.ExecutionID,
			AttemptID: request.Snapshot.Session.AttemptID, Generation: request.Snapshot.ResourceGeneration,
			ConfigDigest: request.Snapshot.ConfigurationDigest,
		}, mustJSON(additionalBinding{Session: value.Session, Generation: value.Generation, Process: value.Process, Target: request.Snapshot.Ref.Target}))
		if bindingErr != nil {
			return receipt, nil, bindingErr
		}
		receipt.Implementation = []executionbackend.ImplementationDataEnvelope{binding}
		receipt.Disposition = executionbackend.OperationConfirmed
		receipt.Process = &value.Process
		receipt.State = executionbackend.LifecycleStateFacts{SessionID: value.Session, SessionGeneration: value.Generation}
	case executionbackend.LifecycleStartTurn:
		if value.Session != request.Snapshot.Session.SessionID || value.Generation != request.Snapshot.Session.Generation {
			return receipt, nil, errors.New("fixture lost original session/generation")
		}
		turn := *request.Input.Turn
		if _, exists := value.Inputs[turn.CommandID]; exists {
			return receipt, nil, errors.New("original input was resent")
		}
		value.Inputs[turn.CommandID] = turn
		if p.ledger.publicationURL != "" {
			value.PRCandidates = []string{p.ledger.publicationURL}
		}
		receipt.Disposition = executionbackend.OperationAccepted
		receipt.Process = &value.Process
		receipt.State = executionbackend.LifecycleStateFacts{SessionID: value.Session, SessionGeneration: value.Generation, TurnID: turn.CommandID, InputAccepted: true, TurnStarted: true}
	case executionbackend.LifecycleDeliverFeedback:
		if request.Snapshot.QuestionRoute == nil || request.Input.Feedback.QuestionID != request.Snapshot.QuestionRoute.QuestionID {
			return receipt, nil, errors.New("fixture answer changed original question")
		}
		value.Answers[request.Input.Feedback.QuestionID] = *request.Input.Feedback
		value.Settled = true
		receipt.Disposition = executionbackend.OperationConfirmed
		receipt.State = executionbackend.LifecycleStateFacts{InputAccepted: true, TurnID: request.Snapshot.Session.CurrentTurnID}
	case executionbackend.LifecycleCleanup:
		if !request.Snapshot.TaskDelivered || !value.Settled || request.Snapshot.Ref.External != value.Process {
			return receipt, nil, errors.New("fixture cleanup lacks original delivered resource identity")
		}
		value.CleanupCount++
		if value.CleanupCount != 1 {
			return receipt, nil, errors.New("fixture cleanup was repeated")
		}
		receipt.Disposition = executionbackend.OperationConfirmed
		receipt.State = executionbackend.LifecycleStateFacts{WorkerAbsent: true, WritersAbsent: true}
	default:
		return receipt, nil, executionbackend.UnsupportedAdapterOperation(request.Operation.Action, "fixture capability is unsupported")
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
	if executionbackend.LifecycleAction(request.Operation.Action) == executionbackend.LifecycleStartTurn {
		return receipt, p.observer(request.Snapshot, value, receipt.State.TurnID), nil
	}
	return receipt, nil, nil
}

func (p *additionalPort) Reconcile(_ context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	p.ledger.mu.Lock()
	defer p.ledger.mu.Unlock()
	p.ledger.reads++
	value, err := p.ledger.read()
	if err != nil {
		return executionbackend.LifecycleReceipt{}, nil, err
	}
	receipt, exists := value.Receipts[request.Operation.ID]
	if !exists {
		return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown}, nil, nil
	}
	if err := receipt.ValidateFor(request); err != nil {
		return receipt, nil, err
	}
	return receipt.Clone(), nil, nil
}

func (p *additionalPort) ObserveLifecycle(_ context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	p.ledger.mu.Lock()
	defer p.ledger.mu.Unlock()
	p.ledger.reads++
	value, err := p.ledger.read()
	if err != nil {
		return executionbackend.LifecycleReceipt{}, nil, err
	}
	if snapshot.Session.SessionID != value.Session || snapshot.Session.Generation != value.Generation || snapshot.Ref.External != value.Process {
		return executionbackend.LifecycleReceipt{}, nil, errors.New("fixture observation replaced original identity")
	}
	if len(snapshot.Implementation) != 1 {
		return executionbackend.LifecycleReceipt{}, nil, errors.New("fixture observation lost original resource binding")
	}
	envelope := snapshot.Implementation[0]
	if envelope.ImplementationKey != additionalKey || envelope.CodecVersion != 1 {
		return executionbackend.LifecycleReceipt{}, nil, errors.New("fixture resource binding codec changed")
	}
	if err := envelope.ValidateResourceIdentity(snapshot.Session.ProjectID, snapshot.Session.ExecutionID, snapshot.Session.AttemptID, snapshot.ResourceGeneration, snapshot.ConfigurationDigest); err != nil {
		return executionbackend.LifecycleReceipt{}, nil, err
	}
	var binding additionalBinding
	if err := json.Unmarshal(envelope.Payload, &binding); err != nil {
		return executionbackend.LifecycleReceipt{}, nil, err
	}
	if binding.Session != value.Session || binding.Generation != value.Generation || binding.Process != value.Process || binding.Target != snapshot.Ref.Target {
		return executionbackend.LifecycleReceipt{}, nil, errors.New("fixture original binding payload changed")
	}
	if _, exists := value.Inputs[snapshot.Session.CurrentTurnID]; !exists {
		return executionbackend.LifecycleReceipt{}, nil, errors.New("fixture observation has no persistent original input")
	}
	if value.Settled {
		turn := snapshot.Session.CurrentTurnID
		return executionbackend.LifecycleReceipt{Disposition: executionbackend.OperationConfirmed, Process: &value.Process,
			State:        executionbackend.LifecycleStateFacts{SessionID: value.Session, SessionGeneration: value.Generation, TurnID: turn, TurnCompleted: true},
			TurnEvidence: &executionbackend.TurnEvidence{SourcePath: p.ledger.path, SessionID: value.Session, TurnID: turn, CommandID: turn, Started: true, Completed: true, Succeeded: true, PRCandidates: append([]string(nil), value.PRCandidates...)}}, nil, nil
	}
	return executionbackend.LifecycleReceipt{Disposition: executionbackend.OperationConfirmed, Process: &value.Process, State: executionbackend.LifecycleStateFacts{SessionID: value.Session, SessionGeneration: value.Generation, TurnID: snapshot.Session.CurrentTurnID}}, p.observer(snapshot, value, snapshot.Session.CurrentTurnID), nil
}

func (p *additionalPort) VerifyManualRecovery(_ context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleRecoveryEvidence, error) {
	p.ledger.mu.Lock()
	defer p.ledger.mu.Unlock()
	p.ledger.reads++
	value, err := p.ledger.read()
	if err != nil || value.Session != snapshot.Session.SessionID || value.Process != snapshot.Ref.External {
		return executionbackend.LifecycleRecoveryEvidence{}, errors.New("fixture recovery cannot verify original identity")
	}
	return executionbackend.LifecycleRecoveryEvidence{SessionAvailable: true, ManagedProcess: true}, nil
}

func (p *additionalPort) BindRecovery(context.Context, executionbackend.LifecycleSnapshot, executionbackend.LifecycleSnapshot) (executionbackend.ImplementationDataEnvelope, error) {
	return executionbackend.ImplementationDataEnvelope{}, errors.New("fixture does not replace sessions")
}

func (p *additionalPort) InspectLifecycleWorkspace(ctx context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.WorkspaceInspection, error) {
	return workspace.InspectLifecycleWorkspace(ctx, p.setup, snapshot)
}

func (p *additionalPort) StopLifecycle(_ context.Context, request executionbackend.LifecycleRequest, _ executionbackend.StopHandle) (executionbackend.LifecycleReceipt, error) {
	return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationRejected}, errors.New("fixture cancellation is unsupported")
}

func (p *additionalPort) CleanupLifecycle(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	receipt, observer, err := p.Execute(ctx, request)
	if observer != nil {
		_ = observer.Close(ctx)
	}
	return receipt, err
}

type additionalHandle struct {
	identity executionbackend.ProcessIdentity
}

func (h additionalHandle) Identity() executionbackend.ProcessIdentity { return h.identity }

func (additionalHandle) Kill() {}

func (additionalHandle) Close() {}

type additionalObserver struct {
	events     chan executionbackend.LifecycleEvent
	done       chan error
	closed     chan struct{}
	completed  chan struct{}
	once       sync.Once
	handle     additionalHandle
	turn       string
	finishOnce sync.Once
}

func (o *additionalObserver) Events() <-chan executionbackend.LifecycleEvent { return o.events }

func (o *additionalObserver) Done() <-chan error { return o.done }

func (o *additionalObserver) StopHandle() executionbackend.StopHandle { return o.handle }

func (o *additionalObserver) Close(context.Context) error {
	o.once.Do(func() { close(o.closed) })
	return nil
}

func (o *additionalObserver) finish() {
	o.finishOnce.Do(func() { close(o.completed) })
}

func (p *additionalPort) observer(snapshot executionbackend.LifecycleSnapshot, value additionalSaved, turn string) *additionalObserver {
	o := &additionalObserver{events: make(chan executionbackend.LifecycleEvent, 8), done: make(chan error, 1), closed: make(chan struct{}), completed: make(chan struct{}), handle: additionalHandle{value.Process}, turn: turn}
	p.ledger.observers[snapshot.Session.ExecutionID] = o
	route, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{ImplementationKey: additionalKey, CodecVersion: 1, Purpose: executionbackend.ImplementationQuestionPurpose, ProjectID: snapshot.Session.ProjectID, WorkflowRunID: snapshot.Session.WorkflowRunID, ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID, Generation: value.Generation, TurnID: turn, QuestionID: "fixture-question"}, []byte(`{"channel":"fixture"}`))
	if err != nil {
		o.done <- err
		close(o.events)
		return o
	}
	question := executionbackend.Question{ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID, Generation: value.Generation, TurnID: turn, ID: "fixture-choice", Header: "Fixture", Question: "Continue the original input?", Options: []executionbackend.DecisionOption{{ID: "continue", Label: "Continue"}}, AllowFreeText: true}
	go func() {
		defer close(o.events)
		for _, fact := range []executionbackend.EventFact{{Kind: executionbackend.EventStarted, SessionID: value.Session, StartedTurnID: turn}, {Kind: executionbackend.EventQuestion, QuestionRoute: &route, Questions: []executionbackend.Question{question}}} {
			ack := make(chan error, 1)
			select {
			case o.events <- executionbackend.LifecycleEvent{Fact: fact, Kind: string(fact.Kind), Status: "observed", Title: "Fixture " + string(fact.Kind), Persisted: ack}:
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
		case o.events <- executionbackend.LifecycleEvent{Fact: executionbackend.EventFact{Kind: executionbackend.EventCompleted, Completed: true, Matched: true, Outcome: "completed", PRCandidates: append([]string(nil), value.PRCandidates...)}, Kind: "completed", Status: "completed", Title: "Fixture completed", Persisted: ack}:
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

func (additionalSources) MapConversation(executionvalues.ConversationInput) (executionvalues.ConversationMappedBatch, error) {
	return executionvalues.ConversationMappedBatch{}, nil
}

func (additionalSources) RestoreMapper(executionvalues.ConversationMapperSnapshot) executionapp.ConversationMapper {
	return nil
}

func (additionalSources) JournalSources() []string { return nil }

func (additionalSources) JournalSource(executionbackend.BackendKind, executionbackend.ExecutionProtocol) string {
	return ""
}

func (additionalSources) JournalSegment(executionbackend.BackendKind, executionbackend.ExecutionProtocol, string) string {
	return ""
}

func (additionalSources) JournalGeneration(string, string) (uint64, error) { return 0, nil }

func (additionalSources) LiveJournalPath(context.Context, executionapp.ConversationJournalPathRequest) (string, error) {
	return "", nil
}

func (additionalSources) DiscoverHistory(context.Context, executionapp.ConversationDiscovery) (executionapp.ConversationDiscoveryResult, error) {
	return executionapp.ConversationDiscoveryResult{}, nil
}

func (additionalSources) DiscoverJournals(context.Context, executionapp.ConversationDiscovery, func(string) (string, error)) ([]executionapp.ConversationJournalSource, error) {
	return nil, nil
}

func (additionalSources) ReadHistory(context.Context, executionapp.ConversationHistoryReadRequest, func(*exec.Cmd) (executionbackend.StopHandle, error)) (executionapp.ConversationHistoryBatch, error) {
	return executionapp.ConversationHistoryBatch{}, fmt.Errorf("fixture history reading is unsupported")
}
