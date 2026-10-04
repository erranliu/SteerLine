package execution

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
)

type neutralBackend struct{ config Config }

func (b neutralBackend) Identity() (executionbackend.BackendKind, executionbackend.ExecutionProtocol) {
	return executionbackend.BackendLocal, neutralProtocol
}

func (b neutralBackend) Probe(_ context.Context, target executionbackend.ExecutionTarget, agent executionbackend.AgentSpec) (executionbackend.CapabilityReport, error) {
	return capabilityReport(executionbackend.BackendLocal, neutralProtocol, target, agent, "fixture-version-1"), nil
}

func (b neutralBackend) Observe(_ context.Context, ref executionbackend.ExecutionRef) (executionbackend.Observation, error) {
	live := "live"
	if ref.External.Kind == "os" {
		label := resourceprocess.InspectProcess(&ref.External)
		if label == "process absent" {
			live = "exited"
		} else if label != "matching process still exists" {
			return executionbackend.Observation{}, errors.New("original fixture process differs")
		}
	}
	return executionbackend.Observation{Liveness: live, Connection: "connected", ObservedAt: now(), Source: "neutral-fixture"}, nil
}

func (b neutralBackend) InspectWorkspace(ctx context.Context, ref executionbackend.ExecutionRef) (executionbackend.WorkspaceInspection, error) {
	return inspectTestWorkspace(ctx, b.config, ref)
}

func (b neutralBackend) StopExecution(ctx context.Context, ref executionbackend.ExecutionRef, _ executionbackend.Operation, handle executionbackend.StopHandle) (executionbackend.StopReceipt, error) {
	obs, err := b.Observe(ctx, ref)
	if err != nil {
		return executionbackend.StopReceipt{}, err
	}
	if obs.Liveness == "exited" {
		return executionbackend.StopReceipt{Disposition: "confirmed"}, nil
	}
	if handle == nil || handle.Identity() != ref.External {
		return executionbackend.StopReceipt{}, errors.New("original fixture stop handle differs")
	}
	handle.Kill()
	return executionbackend.StopReceipt{Disposition: "accepted"}, nil
}

type neutralLifecycle struct {
	setup executionbackend.LifecycleSetup
}

type neutralCompletionReceipt struct {
	Evidence           executionbackend.TurnEvidence
	Process            executionbackend.ProcessIdentity
	Target             executionbackend.ExecutionTarget
	ExecutionID        string
	AttemptID          string
	ResourceGeneration string
}

func (p *neutralLifecycle) PolicyCapabilities() executionbackend.LifecyclePolicyCapabilities {
	return executionbackend.LifecyclePolicyCapabilities{LinkedWorkspaceWriteRoots: true, ResumeActiveObservation: true, ActiveTurnFeedback: true}
}

func (p *neutralLifecycle) PreflightLifecycle(_ context.Context, setup executionbackend.LifecycleSetup) (executionbackend.CapabilityReport, error) {
	r := capabilityReport(setup.Snapshot.Ref.Backend, setup.Snapshot.Ref.Protocol, setup.Snapshot.Ref.Target, setup.Snapshot.Ref.Agent, "fixture-version-1")
	r.Capabilities[0].Status = executionbackend.CapabilityVerified
	r.Capabilities[0].Evidence = "isolated fake start only; no native permission evidence"
	return r, nil
}

func (p *neutralLifecycle) Next(_ context.Context, s executionbackend.LifecycleSnapshot) (executionbackend.LifecycleCandidate, error) {
	if s.Workspace.Path == "" {
		intent := s.WorkspacePlan
		return executionbackend.LifecycleCandidate{Action: executionbackend.LifecyclePrepareWorkspace, Input: executionbackend.LifecycleInput{Workspace: &intent}}, nil
	}
	if s.PendingTurn == nil {
		return executionbackend.LifecycleCandidate{Done: true}, nil
	}
	turn := *s.PendingTurn
	action := executionbackend.LifecycleStartTurn
	if s.Session.SessionID == "" {
		action = executionbackend.LifecyclePrepareSession
	}
	return executionbackend.LifecycleCandidate{Action: action, Input: executionbackend.LifecycleInput{Turn: &turn}}, nil
}

func (p *neutralLifecycle) Execute(ctx context.Context, r executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := executionbackend.LifecycleReceipt{Operation: r.Operation, Disposition: executionbackend.OperationUnknown}
	if err := r.Validate(); err != nil {
		return receipt, nil, err
	}
	if r.Input.Workspace != nil {
		git := workspace.Git{Command: executionbackend.LaunchCommand{Executable: p.setup.GitExecutable, Args: p.setup.GitArgs}, Timeout: p.setup.CommandTimeout, Start: p.setup.ProcessHost.StartManaged}
		value, err := workspace.Prepare(ctx, git, workspace.Repository{Path: p.setup.RepositoryPath, CommonDir: r.Snapshot.Session.ProjectID}, filepath.Dir(r.Input.Workspace.Path), r.Input.Workspace)
		receipt.Disposition, receipt.Workspace = value.Status, value.Inspection
		return receipt, nil, err
	}
	process := r.Snapshot.Ref.External
	if process.Kind == "" {
		process = executionbackend.ProcessIdentity{Kind: "fixture", RuntimeID: r.Snapshot.Session.ExecutionID, Incarnation: r.Snapshot.ResourceGeneration}
	}
	session, generation := r.Snapshot.Session.SessionID, r.Snapshot.Session.Generation
	if session == "" {
		session = workerFlowMockSession
		generation = r.Snapshot.ResourceGeneration
	}
	receipt.Process = &process
	receipt.State.SessionID, receipt.State.SessionGeneration = session, generation
	switch executionbackend.LifecycleAction(r.Operation.Action) {
	case executionbackend.LifecyclePrepareSession:
		receipt.Disposition = executionbackend.OperationConfirmed
	case executionbackend.LifecycleStartTurn:
		receipt.Disposition = executionbackend.OperationAccepted
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
			return os.WriteFile(filepath.Join(filepath.Dir(path), "fixture-completed.json"), mustJSON(neutralCompletionReceipt{Evidence: executionbackend.TurnEvidence{SessionID: session, TurnID: r.Input.Turn.CommandID, CommandID: r.Input.Turn.CommandID, Started: true, Completed: true, Succeeded: true, LastMessage: "neutral turn completed"}, Process: process, Target: r.Snapshot.Ref.Target, ExecutionID: r.Snapshot.Session.ExecutionID, AttemptID: r.Snapshot.Session.AttemptID, ResourceGeneration: r.Snapshot.ResourceGeneration}), 0600)
		}), nil
	case executionbackend.LifecycleDeliverFeedback:
		receipt.Disposition = executionbackend.OperationConfirmed
		receipt.State.InputAccepted = true
	default:
		return receipt, nil, executionbackend.UnsupportedAdapterOperation(r.Operation.Action, "neutral fixture action unsupported")
	}
	return receipt, nil, nil
}

func (p *neutralLifecycle) Reconcile(_ context.Context, r executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	return executionbackend.LifecycleReceipt{Operation: r.Operation, Disposition: executionbackend.OperationUnknown}, nil, nil
}

func (p *neutralLifecycle) ObserveLifecycle(_ context.Context, s executionbackend.LifecycleSnapshot) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	path := filepath.Join(p.setup.RunDirectory, "runs", s.Ref.RunID, "turns", s.Session.CurrentCommandID, "fixture-completed.json")
	if data, err := os.ReadFile(path); err == nil {
		var saved neutralCompletionReceipt
		if err := decode(data, &saved); err != nil {
			return executionbackend.LifecycleReceipt{}, nil, err
		}
		evidence := saved.Evidence
		if saved.Process != s.Ref.External || saved.Target != s.Ref.Target || saved.ExecutionID != s.Session.ExecutionID || saved.AttemptID != s.Session.AttemptID || saved.ResourceGeneration != s.ResourceGeneration || evidence.SessionID != s.Session.SessionID || evidence.TurnID != s.Session.CurrentTurnID || evidence.CommandID != s.Session.CurrentCommandID {
			return executionbackend.LifecycleReceipt{}, nil, errors.New("original fixture completion identity differs")
		}
		return executionbackend.LifecycleReceipt{Disposition: executionbackend.OperationConfirmed, Process: &s.Ref.External, TurnEvidence: &evidence, State: executionbackend.LifecycleStateFacts{SessionID: evidence.SessionID, SessionGeneration: s.Session.Generation, TurnID: evidence.TurnID, TurnCompleted: true}}, nil, nil
	} else if !os.IsNotExist(err) {
		return executionbackend.LifecycleReceipt{}, nil, err
	}
	receipt := executionbackend.LifecycleReceipt{Disposition: executionbackend.OperationConfirmed, Process: &s.Ref.External, State: executionbackend.LifecycleStateFacts{SessionID: s.Session.SessionID, SessionGeneration: s.Session.Generation, TurnID: s.Session.CurrentTurnID, TurnCompleted: s.Session.CurrentTurnID != "" && s.Session.CurrentTurnID == s.Session.LastStoppedTurnID}}
	if receipt.State.TurnCompleted {
		return receipt, nil, nil
	}
	return receipt, newNeutralObserver(s.Ref.External, s.Session.SessionID, s.Session.CurrentTurnID), nil
}

func (p *neutralLifecycle) StopLifecycle(_ context.Context, r executionbackend.LifecycleRequest, h executionbackend.StopHandle) (executionbackend.LifecycleReceipt, error) {
	if h != nil {
		h.Kill()
	}
	return executionbackend.LifecycleReceipt{Operation: r.Operation, Disposition: executionbackend.OperationAccepted}, nil
}

func (p *neutralLifecycle) CleanupLifecycle(_ context.Context, r executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	return executionbackend.LifecycleReceipt{Operation: r.Operation, Disposition: executionbackend.OperationConfirmed, State: executionbackend.LifecycleStateFacts{WorkerAbsent: true, WritersAbsent: true}}, nil
}

func (p *neutralLifecycle) VerifyManualRecovery(_ context.Context, s executionbackend.LifecycleSnapshot) (executionbackend.LifecycleRecoveryEvidence, error) {
	absent := false
	if s.Ref.External.Kind == "os" {
		absent = resourceprocess.InspectProcess(&s.Ref.External) == "process absent"
	}
	return executionbackend.LifecycleRecoveryEvidence{ManagedProcess: true, ProcessAbsent: absent, SessionAvailable: s.Session.SessionID != ""}, nil
}

func (p *neutralLifecycle) BindRecovery(_ context.Context, old, next executionbackend.LifecycleSnapshot) (executionbackend.ImplementationDataEnvelope, error) {
	return executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{ImplementationKey: neutralBindingKey, CodecVersion: 1, Purpose: executionbackend.ImplementationResourcePurpose, ProjectID: next.Session.ProjectID, ExecutionID: next.Session.ExecutionID, AttemptID: next.Session.AttemptID, Generation: next.ResourceGeneration, ConfigDigest: next.ConfigurationDigest}, mustJSON(neutralBinding{SessionID: old.Session.SessionID, Generation: old.Session.Generation}))
}

func (p *neutralLifecycle) InspectLifecycleWorkspace(ctx context.Context, s executionbackend.LifecycleSnapshot) (executionbackend.WorkspaceInspection, error) {
	return workspace.InspectLifecycleWorkspace(ctx, p.setup, s)
}

type neutralHandle struct {
	identity executionbackend.ProcessIdentity
}

func (h neutralHandle) Identity() executionbackend.ProcessIdentity { return h.identity }

func (neutralHandle) Kill() {}

func (neutralHandle) Close() {}

type neutralObserver struct {
	events chan executionbackend.LifecycleEvent
	done   chan error
	stop   chan struct{}
	once   sync.Once
	handle neutralHandle
}

func newNeutralObserver(identity executionbackend.ProcessIdentity, session, turn string, completed ...func() error) *neutralObserver {
	o := &neutralObserver{events: make(chan executionbackend.LifecycleEvent, 2), done: make(chan error, 1), stop: make(chan struct{}), handle: neutralHandle{identity}}
	go func() {
		defer close(o.events)
		for _, fact := range []executionbackend.EventFact{{Kind: executionbackend.EventStarted, SessionID: session, StartedTurnID: turn}, {Kind: executionbackend.EventCompleted, Completed: true, Matched: true, Outcome: "completed", LastMessage: "neutral turn completed"}} {
			ack := make(chan error, 1)
			select {
			case o.events <- executionbackend.LifecycleEvent{Fact: fact, Kind: string(fact.Kind), Persisted: ack}:
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

func (o *neutralObserver) Events() <-chan executionbackend.LifecycleEvent { return o.events }

func (o *neutralObserver) Done() <-chan error { return o.done }

func (o *neutralObserver) StopHandle() executionbackend.StopHandle { return o.handle }

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

func (neutralConversationSources) JournalSource(executionbackend.BackendKind, executionbackend.ExecutionProtocol) string {
	return ""
}

func (neutralConversationSources) JournalSegment(executionbackend.BackendKind, executionbackend.ExecutionProtocol, string) string {
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

func (neutralConversationSources) ReadHistory(context.Context, contracts.ConversationHistoryReadRequest, func(*exec.Cmd) (executionbackend.StopHandle, error)) (contracts.ConversationHistoryBatch, error) {
	return contracts.ConversationHistoryBatch{}, errors.New("fixture history unsupported")
}

var _ = time.Second
