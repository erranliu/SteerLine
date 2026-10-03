package steerline

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
)

type neutralBackend struct{ config Config }

func (b neutralBackend) Identity() (executionapi.BackendKind, executionapi.ExecutionProtocol) {
	return executionapi.BackendLocal, neutralProtocol
}
func (b neutralBackend) Probe(_ context.Context, target executionapi.ExecutionTarget, agent executionapi.AgentSpec) (executionapi.CapabilityReport, error) {
	return capabilityReport(executionapi.BackendLocal, neutralProtocol, target, agent, "fixture-version-1"), nil
}
func (b neutralBackend) Observe(_ context.Context, ref executionapi.ExecutionRef) (executionapi.Observation, error) {
	live := "live"
	if ref.External.Kind == "os" {
		label := inspectProcess(&ref.External)
		if label == "process absent" {
			live = "exited"
		} else if label != "matching process still exists" {
			return executionapi.Observation{}, errors.New("original fixture process differs")
		}
	}
	return executionapi.Observation{Liveness: live, Connection: "connected", ObservedAt: now(), Source: "neutral-fixture"}, nil
}
func (b neutralBackend) InspectWorkspace(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error) {
	return inspectTestWorkspace(ctx, b.config, ref)
}
func (b neutralBackend) StopExecution(ctx context.Context, ref executionapi.ExecutionRef, _ executionapi.Operation, handle executionapi.StopHandle) (executionapi.StopReceipt, error) {
	obs, err := b.Observe(ctx, ref)
	if err != nil {
		return executionapi.StopReceipt{}, err
	}
	if obs.Liveness == "exited" {
		return executionapi.StopReceipt{Disposition: "confirmed"}, nil
	}
	if handle == nil || handle.Identity() != ref.External {
		return executionapi.StopReceipt{}, errors.New("original fixture stop handle differs")
	}
	handle.Kill()
	return executionapi.StopReceipt{Disposition: "accepted"}, nil
}

type neutralLifecycle struct{ setup executionapi.LifecycleSetup }

type neutralCompletionReceipt struct {
	Evidence           executionapi.TurnEvidence
	Process            executionapi.ProcessIdentity
	Target             executionapi.ExecutionTarget
	ExecutionID        string
	AttemptID          string
	ResourceGeneration string
}

func (p *neutralLifecycle) PolicyCapabilities() executionapi.LifecyclePolicyCapabilities {
	return executionapi.LifecyclePolicyCapabilities{LinkedWorkspaceWriteRoots: true, ResumeActiveObservation: true, ActiveTurnFeedback: true}
}
func (p *neutralLifecycle) PreflightLifecycle(_ context.Context, setup executionapi.LifecycleSetup) (executionapi.CapabilityReport, error) {
	r := capabilityReport(setup.Snapshot.Ref.Backend, setup.Snapshot.Ref.Protocol, setup.Snapshot.Ref.Target, setup.Snapshot.Ref.Agent, "fixture-version-1")
	r.Capabilities[0].Status = executionapi.CapabilityVerified
	r.Capabilities[0].Evidence = "isolated fake start only; no native permission evidence"
	return r, nil
}
func (p *neutralLifecycle) Next(_ context.Context, s executionapi.LifecycleSnapshot) (executionapi.LifecycleCandidate, error) {
	if s.Workspace.Path == "" {
		intent := s.WorkspacePlan
		return executionapi.LifecycleCandidate{Action: executionapi.LifecyclePrepareWorkspace, Input: executionapi.LifecycleInput{Workspace: &intent}}, nil
	}
	if s.PendingTurn == nil {
		return executionapi.LifecycleCandidate{Done: true}, nil
	}
	turn := *s.PendingTurn
	action := executionapi.LifecycleStartTurn
	if s.Session.SessionID == "" {
		action = executionapi.LifecyclePrepareSession
	}
	return executionapi.LifecycleCandidate{Action: action, Input: executionapi.LifecycleInput{Turn: &turn}}, nil
}
func (p *neutralLifecycle) Execute(ctx context.Context, r executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := executionapi.LifecycleReceipt{Operation: r.Operation, Disposition: executionapi.OperationUnknown}
	if err := r.Validate(); err != nil {
		return receipt, nil, err
	}
	if r.Input.Workspace != nil {
		git := workspace.Git{Command: executionapi.LaunchCommand{Executable: p.setup.GitExecutable, Args: p.setup.GitArgs}, Timeout: p.setup.CommandTimeout, Start: p.setup.ProcessHost.StartManaged}
		value, err := workspace.Prepare(ctx, git, workspace.Repository{Path: p.setup.RepositoryPath, CommonDir: r.Snapshot.Session.ProjectID}, filepath.Dir(r.Input.Workspace.Path), r.Input.Workspace)
		receipt.Disposition, receipt.Workspace = value.Status, value.Inspection
		return receipt, nil, err
	}
	process := r.Snapshot.Ref.External
	if process.Kind == "" {
		process = executionapi.ProcessIdentity{Kind: "fixture", RuntimeID: r.Snapshot.Session.ExecutionID, Incarnation: r.Snapshot.ResourceGeneration}
	}
	session, generation := r.Snapshot.Session.SessionID, r.Snapshot.Session.Generation
	if session == "" {
		session = workerFlowMockSession
		generation = r.Snapshot.ResourceGeneration
	}
	receipt.Process = &process
	receipt.State.SessionID, receipt.State.SessionGeneration = session, generation
	switch executionapi.LifecycleAction(r.Operation.Action) {
	case executionapi.LifecyclePrepareSession:
		receipt.Disposition = executionapi.OperationConfirmed
	case executionapi.LifecycleStartTurn:
		receipt.Disposition = executionapi.OperationAccepted
		receipt.State.TurnID = r.Input.Turn.CommandID
		receipt.State.InputAccepted, receipt.State.TurnStarted = true, true
		// Record only the value actually delivered to this single-step port.
		// Both the received text and serialized request retain the exact original input.
		path := filepath.Join(p.setup.RunDirectory, "runs", r.Operation.RunID, "turns", r.Input.Turn.CommandID, "fixture-input.json")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return receipt, nil, err
		}
		if err := os.WriteFile(filepath.Join(filepath.Dir(path), "prompt.txt"), []byte(r.Input.Turn.Prompt), 0600); err != nil {
			return receipt, nil, err
		}
		if err := os.WriteFile(path, mustJSON(r.Input.Turn.Prompt), 0600); err != nil {
			return receipt, nil, err
		}
		return receipt, newNeutralObserver(process, session, r.Input.Turn.CommandID, func() error {
			return os.WriteFile(filepath.Join(filepath.Dir(path), "fixture-completed.json"), mustJSON(neutralCompletionReceipt{Evidence: executionapi.TurnEvidence{SessionID: session, TurnID: r.Input.Turn.CommandID, CommandID: r.Input.Turn.CommandID, Started: true, Completed: true, Succeeded: true, LastMessage: "neutral turn completed"}, Process: process, Target: r.Snapshot.Ref.Target, ExecutionID: r.Snapshot.Session.ExecutionID, AttemptID: r.Snapshot.Session.AttemptID, ResourceGeneration: r.Snapshot.ResourceGeneration}), 0600)
		}), nil
	case executionapi.LifecycleDeliverFeedback:
		receipt.Disposition = executionapi.OperationConfirmed
		receipt.State.InputAccepted = true
	default:
		return receipt, nil, executionapi.UnsupportedAdapterOperation(r.Operation.Action, "neutral fixture action unsupported")
	}
	return receipt, nil, nil
}
func (p *neutralLifecycle) Reconcile(_ context.Context, r executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	return executionapi.LifecycleReceipt{Operation: r.Operation, Disposition: executionapi.OperationUnknown}, nil, nil
}
func (p *neutralLifecycle) ObserveLifecycle(_ context.Context, s executionapi.LifecycleSnapshot) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	path := filepath.Join(p.setup.RunDirectory, "runs", s.Ref.RunID, "turns", s.Session.CurrentCommandID, "fixture-completed.json")
	if data, err := os.ReadFile(path); err == nil {
		var saved neutralCompletionReceipt
		if err := decode(data, &saved); err != nil {
			return executionapi.LifecycleReceipt{}, nil, err
		}
		evidence := saved.Evidence
		if saved.Process != s.Ref.External || saved.Target != s.Ref.Target || saved.ExecutionID != s.Session.ExecutionID || saved.AttemptID != s.Session.AttemptID || saved.ResourceGeneration != s.ResourceGeneration || evidence.SessionID != s.Session.SessionID || evidence.TurnID != s.Session.CurrentTurnID || evidence.CommandID != s.Session.CurrentCommandID {
			return executionapi.LifecycleReceipt{}, nil, errors.New("original fixture completion identity differs")
		}
		return executionapi.LifecycleReceipt{Disposition: executionapi.OperationConfirmed, Process: &s.Ref.External, TurnEvidence: &evidence, State: executionapi.LifecycleStateFacts{SessionID: evidence.SessionID, SessionGeneration: s.Session.Generation, TurnID: evidence.TurnID, TurnCompleted: true}}, nil, nil
	} else if !os.IsNotExist(err) {
		return executionapi.LifecycleReceipt{}, nil, err
	}
	receipt := executionapi.LifecycleReceipt{Disposition: executionapi.OperationConfirmed, Process: &s.Ref.External, State: executionapi.LifecycleStateFacts{SessionID: s.Session.SessionID, SessionGeneration: s.Session.Generation, TurnID: s.Session.CurrentTurnID, TurnCompleted: s.Session.CurrentTurnID != "" && s.Session.CurrentTurnID == s.Session.LastStoppedTurnID}}
	if receipt.State.TurnCompleted {
		return receipt, nil, nil
	}
	return receipt, newNeutralObserver(s.Ref.External, s.Session.SessionID, s.Session.CurrentTurnID), nil
}
func (p *neutralLifecycle) StopLifecycle(_ context.Context, r executionapi.LifecycleRequest, h executionapi.StopHandle) (executionapi.LifecycleReceipt, error) {
	if h != nil {
		h.Kill()
	}
	return executionapi.LifecycleReceipt{Operation: r.Operation, Disposition: executionapi.OperationAccepted}, nil
}
func (p *neutralLifecycle) CleanupLifecycle(_ context.Context, r executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	return executionapi.LifecycleReceipt{Operation: r.Operation, Disposition: executionapi.OperationConfirmed, State: executionapi.LifecycleStateFacts{WorkerAbsent: true, WritersAbsent: true}}, nil
}
func (p *neutralLifecycle) VerifyManualRecovery(_ context.Context, s executionapi.LifecycleSnapshot) (executionapi.LifecycleRecoveryEvidence, error) {
	absent := false
	if s.Ref.External.Kind == "os" {
		absent = inspectProcess(&s.Ref.External) == "process absent"
	}
	return executionapi.LifecycleRecoveryEvidence{ManagedProcess: true, ProcessAbsent: absent, SessionAvailable: s.Session.SessionID != ""}, nil
}
func (p *neutralLifecycle) BindRecovery(_ context.Context, old, next executionapi.LifecycleSnapshot) (executionapi.ImplementationDataEnvelope, error) {
	return executionapi.NewImplementationDataEnvelope(executionapi.ImplementationDataEnvelope{ImplementationKey: neutralBindingKey, CodecVersion: 1, Purpose: executionapi.ImplementationResourcePurpose, ProjectID: next.Session.ProjectID, ExecutionID: next.Session.ExecutionID, AttemptID: next.Session.AttemptID, Generation: next.ResourceGeneration, ConfigDigest: next.ConfigurationDigest}, mustJSON(neutralBinding{SessionID: old.Session.SessionID, Generation: old.Session.Generation}))
}
func (p *neutralLifecycle) InspectLifecycleWorkspace(ctx context.Context, s executionapi.LifecycleSnapshot) (executionapi.WorkspaceInspection, error) {
	return workspace.InspectLifecycleWorkspace(ctx, p.setup, s)
}

type neutralHandle struct{ identity executionapi.ProcessIdentity }

func (h neutralHandle) Identity() executionapi.ProcessIdentity { return h.identity }
func (neutralHandle) Kill()                                    {}
func (neutralHandle) Close()                                   {}

type neutralObserver struct {
	events chan executionapi.LifecycleEvent
	done   chan error
	stop   chan struct{}
	once   sync.Once
	handle neutralHandle
}

func newNeutralObserver(identity executionapi.ProcessIdentity, session, turn string, completed ...func() error) *neutralObserver {
	o := &neutralObserver{events: make(chan executionapi.LifecycleEvent, 2), done: make(chan error, 1), stop: make(chan struct{}), handle: neutralHandle{identity}}
	go func() {
		defer close(o.events)
		for _, fact := range []executionapi.EventFact{{Kind: executionapi.EventStarted, SessionID: session, StartedTurnID: turn}, {Kind: executionapi.EventCompleted, Completed: true, Matched: true, Outcome: "completed", LastMessage: "neutral turn completed"}} {
			ack := make(chan error, 1)
			select {
			case o.events <- executionapi.LifecycleEvent{Fact: fact, Kind: string(fact.Kind), Persisted: ack}:
			case <-o.stop:
				return
			}
			select {
			case err := <-ack:
				if err != nil {
					o.done <- err
					return
				}
			case <-o.stop:
				return
			}
		}
		if len(completed) > 0 {
			o.done <- completed[0]()
			return
		}
		o.done <- nil
	}()
	return o
}
func (o *neutralObserver) Events() <-chan executionapi.LifecycleEvent { return o.events }
func (o *neutralObserver) Done() <-chan error                         { return o.done }
func (o *neutralObserver) StopHandle() executionapi.StopHandle        { return o.handle }
func (o *neutralObserver) Close(context.Context) error {
	o.once.Do(func() { close(o.stop) })
	return nil
}

type neutralConversationSources struct{}

func (neutralConversationSources) MapConversation(contracts.ConversationInput) (contracts.ConversationMappedBatch, error) {
	return contracts.ConversationMappedBatch{}, nil
}
func (neutralConversationSources) RestoreMapper(contracts.ConversationMapperSnapshot) contracts.ConversationMapper {
	return nil
}
func (neutralConversationSources) JournalSources() []string { return nil }
func (neutralConversationSources) JournalSource(executionapi.BackendKind, executionapi.ExecutionProtocol) string {
	return ""
}
func (neutralConversationSources) JournalSegment(executionapi.BackendKind, executionapi.ExecutionProtocol, string) string {
	return ""
}
func (neutralConversationSources) JournalGeneration(string, string) (uint64, error) { return 0, nil }
func (neutralConversationSources) LiveJournalPath(context.Context, contracts.ConversationJournalPathRequest) (string, error) {
	return "", nil
}
func (neutralConversationSources) DiscoverHistory(context.Context, contracts.ConversationDiscovery) (contracts.ConversationDiscoveryResult, error) {
	return contracts.ConversationDiscoveryResult{}, nil
}
func (neutralConversationSources) DiscoverJournals(context.Context, contracts.ConversationDiscovery, func(string) (string, error)) ([]contracts.ConversationJournalSource, error) {
	return nil, nil
}
func (neutralConversationSources) ReadHistory(context.Context, contracts.ConversationHistoryReadRequest, func(*exec.Cmd) (executionapi.StopHandle, error)) (contracts.ConversationHistoryBatch, error) {
	return contracts.ConversationHistoryBatch{}, errors.New("fixture history unsupported")
}

var _ = time.Second
