package orcaworker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func (l *ExecutionLifecycle) MapConversation(input contracts.ConversationInput) (contracts.ConversationMappedBatch, error) {
	return MapTerminalConversation(input)
}

func MapTerminalConversation(input contracts.ConversationInput) (contracts.ConversationMappedBatch, error) {
	if input.Source != "orca-terminal-v2" {
		return contracts.ConversationMappedBatch{}, nil
	}
	event := input.Event
	if event.ProjectID == "" || event.ExecutionID == "" || event.AttemptID == "" {
		return contracts.ConversationMappedBatch{}, errors.New("terminal conversation has no fixed execution identity")
	}
	segment := input.Segment
	if segment == "" {
		segment = "orca-terminal-v2-unsegmented"
	}
	event.Version, event.Source, event.SourceRole, event.SourceSegment = contracts.ConversationVersion, "orca-terminal-v2", contracts.ConversationSourceFallback, segment
	event.SourceIdentity, event.SourceCursor, event.SessionID = input.Identity, input.Cursor, input.Session
	event.ExternalRef = hostExternalRef(input.Process)
	event.SessionQuality, event.TurnQuality, event.ItemQuality = contracts.IdentityUnknown, contracts.IdentityUnknown, contracts.IdentityUnknown
	event.SourceEventQuality, event.SourceSeq, event.ReceivedAt = contracts.IdentityDerived, input.SourceSeq, time.Now().UTC()
	eventID, quality := input.Event.SourceEventID, contracts.IdentityDerived
	if eventID == "" {
		eventID, quality = "unidentified:"+newHostSourceID(), contracts.IdentityUnknown
	}
	event.SourceEventID, event.SourceEventQuality = eventID, quality
	event.Phase, event.Status = contracts.MessagePhaseUnknown, "observed"
	event.Evidence = map[string]string{"capture": "terminal-read", "turn_boundary": "unknown", "message_phase": "unknown", "source_changed": fmt.Sprint(input.SourceChanged)}
	event.Diagnostic = input.Diagnostic
	if input.Session != "" {
		event.SessionQuality = contracts.IdentityDerived
	}
	write := contracts.ConversationWrite{Event: event}
	if input.Gap {
		event.Type, event.Status = contracts.ConversationGap, "gap"
		if len(input.Raw) > 0 {
			event.ItemID, event.ItemQuality = eventID, contracts.IdentityDerived
			event.ItemKey = terminalItemKey(event)
			event.Attributes = map[string]any{"content_kind": "terminal_observation", "source_changed": input.SourceChanged}
			write = contracts.ConversationWrite{Event: event, Body: append([]byte(nil), input.Raw...), BodyMode: "snapshot", Raw: append([]byte(nil), input.Raw...), BodySource: "orca-terminal-stream"}
		}
		write.Event = event
	} else if input.InputWait {
		event.Type, event.Status = contracts.ConversationInputWait, "waiting"
		event.Attributes = map[string]any{"wait_origin": "host", "business_decision": false}
		event.Evidence["wait_source"] = "host.terminal.agent-wait"
		write.Event = event
	} else {
		event.Type, event.ItemID, event.ItemQuality = contracts.ConversationObservation, eventID, contracts.IdentityDerived
		event.ItemKey = terminalItemKey(event)
		write = contracts.ConversationWrite{Event: event, Body: append([]byte(nil), input.Raw...), BodyMode: "snapshot", Raw: append([]byte(nil), input.Raw...), BodySource: "orca-terminal-stream"}
	}
	checkpoint := contracts.ConversationCheckpoint{ProjectID: event.ProjectID, ExecutionID: event.ExecutionID, AttemptID: event.AttemptID,
		Source: event.Source, SourceSegment: segment, SourceIdentity: input.Identity, Generation: input.Generation, Cursor: input.Cursor, LocalSeq: input.SourceSeq}
	return contracts.ConversationMappedBatch{Writes: []contracts.ConversationWrite{write}, Checkpoint: checkpoint}, nil
}

func newHostSourceID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}

func terminalItemKey(event contracts.ConversationEvent) string {
	if event.ItemID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(event.ItemIdentity()))
	return hex.EncodeToString(sum[:])
}

func hostExternalRef(identity *executionbackend.ProcessIdentity) string {
	if identity == nil {
		return ""
	}
	if identity.Kind == "orca" && identity.Terminal != "" {
		return "terminal:" + identity.Terminal + "@" + identity.Incarnation
	}
	if identity.Kind == "os" && identity.PID > 0 {
		return fmt.Sprintf("process:%d@%s", identity.PID, identity.Created)
	}
	return ""
}

func (l *ExecutionLifecycle) ObserveHost(ctx context.Context, identity executionbackend.ProcessIdentity) (executionbackend.HostFact, error) {
	return ObserveHost(ctx, l.client.MetaCall, workerTerminalIdentity(identity))
}

func (l *ExecutionLifecycle) PlatformVersion(ctx context.Context) (string, error) {
	if l.orca.Executable == "" || l.client.config.Output == nil {
		return "", errors.New("host implementation has no configured version probe")
	}
	data, err := l.client.Raw(ctx, positiveDuration(l.config.CommandTimeout, 30*time.Second), "--version")
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(data))
	if version == "" {
		return "", errors.New("host implementation version probe returned no version")
	}
	return version, nil
}

func (l *ExecutionLifecycle) InspectAgentInput(ctx context.Context, identity executionbackend.ProcessIdentity) (executionbackend.HostInputObservation, error) {
	fact, ready, err := InspectAgentInput(ctx, l.client.MetaCall, workerTerminalIdentity(identity))
	return executionbackend.HostInputObservation{Fact: fact, Ready: ready}, err
}

func (l *ExecutionLifecycle) PreparationBudget() time.Duration {
	timeout, err := time.ParseDuration(l.orca.WaitTimeout)
	if err != nil || timeout <= 0 {
		return 2 * time.Minute
	}
	return timeout
}

func (l *ExecutionLifecycle) CurrentTurn(snapshot executionbackend.LifecycleSnapshot) (*executionbackend.HostTurnFacts, error) {
	// Before resources have been created there is no native turn to inspect.
	// A missing binding for an already bound process remains an error.
	if snapshot.Ref.External.Terminal == "" {
		bound := false
		for _, envelope := range snapshot.Implementation {
			bound = bound || envelope.ImplementationKey == ExecutionStateCodecKey
		}
		if !bound {
			return nil, nil
		}
	}
	state, _, err := l.readHostState(snapshot)
	if err != nil || len(state.Turns) == 0 {
		return nil, err
	}
	turn := state.Turns[len(state.Turns)-1]
	waitTimeout := l.PreparationBudget()
	return &executionbackend.HostTurnFacts{TurnID: turn.TurnID, TaskID: turn.TaskID, Purpose: turn.Purpose, Outcome: turn.Outcome,
		IntentAt: turn.IntentAt, Deadline: turn.IntentAt.Add(waitTimeout), Settled: turn.Settled,
		HasDispatch: turn.DispatchID != "", HasDoneMessage: turn.DoneMessageID != ""}, nil
}

func (l *ExecutionLifecycle) ResetTurnOutput(snapshot executionbackend.LifecycleSnapshot, turnID string) (executionbackend.ImplementationDataEnvelope, error) {
	state, _, err := l.readHostState(snapshot)
	if err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	if len(state.Turns) == 0 || state.Turns[len(state.Turns)-1].TurnID != turnID {
		return executionbackend.ImplementationDataEnvelope{}, errors.New("original host turn is no longer current")
	}
	state.Turns[len(state.Turns)-1].OutputCursor = ""
	return NewExecutionStateEnvelope(l.hostStateIdentity(snapshot), state)
}

func (l *ExecutionLifecycle) PrepareProbe(identity executionbackend.ProbeIdentity) (executionbackend.ProbePlan, error) {
	return BuildProbe(identity)
}

func (l *ExecutionLifecycle) ProbePrompt(plan executionbackend.ProbePlan, nonce, digest string) string {
	return ProbePrompt(plan.Command, nonce, digest)
}

func (l *ExecutionLifecycle) ReadProbe(identity executionbackend.ProbeIdentity, digest string) (*executionbackend.ProbeReceipt, error) {
	return ReadProbe(identity, digest)
}

func (l *ExecutionLifecycle) ReadWorkerOutput(ctx context.Context, snapshot executionbackend.LifecycleSnapshot, input executionbackend.HostReadInput) (executionbackend.HostOutput, executionbackend.ImplementationDataEnvelope, error) {
	state, _, err := l.readHostState(snapshot)
	if err != nil {
		return executionbackend.HostOutput{}, executionbackend.ImplementationDataEnvelope{}, err
	}
	if len(state.Turns) == 0 {
		return executionbackend.HostOutput{}, executionbackend.ImplementationDataEnvelope{}, errors.New("host output has no original turn binding")
	}
	turn := &state.Turns[len(state.Turns)-1]
	if turn.DispatchID == "" {
		return executionbackend.HostOutput{}, executionbackend.ImplementationDataEnvelope{}, errors.New("host output has no original dispatch identity")
	}
	result, resetCursor, err := ReadWorkerOutput(ctx, l.client.MetaCall, turn.DispatchID, turn.OutputCursor, state.Coordinator.RuntimeID)
	if err != nil {
		return executionbackend.HostOutput{}, executionbackend.ImplementationDataEnvelope{}, err
	}
	cursor := result.NextCursor
	if cursor == "" {
		cursor = result.Cursor
	}
	if turn.Purpose != "readiness" {
		turn.OutputCursor = cursor
	}
	turn.OutputSource, turn.OutputComplete = result.Source, result.ContentComplete
	turn.OutputClipping = append([]string(nil), result.Clipping...)
	turn.OutputFallbackReason = result.FallbackReason
	var probeEvidence *executionbackend.ProbeToolEvidence
	if turn.Purpose == "readiness" && input.Probe != nil {
		preparation := *input.Probe
		if resetCursor {
			preparation.Evidence = executionbackend.ProbeToolEvidence{}
		}
		preparation.Evidence, err = ObserveProbe(result, preparation.Command, preparation.Nonce, preparation.ScriptDigest, preparation.Evidence)
		if err != nil {
			return executionbackend.HostOutput{}, executionbackend.ImplementationDataEnvelope{}, err
		}
		probeEvidence = &preparation.Evidence
		if preparation.Evidence.FailureCode != "" {
			fact := executionbackend.HostFact{Reason: preparation.Evidence.FailureCode, Confidence: "confirmed", Source: "host.transcript.tool-result", Turn: turn.TurnID}
			turn.OutputDigest = digestValue(struct{ Source, Content string }{result.Source, result.DisplayContent()})
			envelope, encodeErr := NewExecutionStateEnvelope(l.hostStateIdentity(snapshot), state)
			return executionbackend.HostOutput{Fact: &fact, ProbeEvidence: probeEvidence}, envelope, encodeErr
		}
	}
	if turn.Purpose == "business" {
		if resetCursor {
			turn.ToolContext = executionbackend.ToolContext{}
		}
		var failed bool
		turn.ToolContext, failed, err = ObserveToolInitialization(result, turn.TaskID, turn.IntentAt.UnixMilli(), turn.ToolContext)
		if err != nil {
			return executionbackend.HostOutput{}, executionbackend.ImplementationDataEnvelope{}, err
		}
		if failed {
			fact := executionbackend.HostFact{Reason: "tool_initialization_failed", Confidence: "confirmed", Source: "host.transcript.tool-result", Turn: turn.TurnID}
			envelope, encodeErr := NewExecutionStateEnvelope(l.hostStateIdentity(snapshot), state)
			return executionbackend.HostOutput{Fact: &fact}, envelope, encodeErr
		}
	}
	content := result.DisplayContent()
	digest := digestValue(struct{ Source, Content string }{result.Source, content})
	changed := digest != turn.OutputDigest
	turn.OutputDigest = digest
	envelope, err := NewExecutionStateEnvelope(l.hostStateIdentity(snapshot), state)
	if err != nil {
		return executionbackend.HostOutput{}, executionbackend.ImplementationDataEnvelope{}, err
	}
	fact := (*executionbackend.HostFact)(nil)
	if result.SourceChanged || len(result.Clipping) > 0 || !result.ContentComplete {
		fact = &executionbackend.HostFact{Reason: "output_bounded", Confidence: "confirmed", Source: "host.output", Detail: fmt.Sprintf("%s; %s", result.Source, result.FallbackReason), Turn: turn.TurnID}
	}
	return executionbackend.HostOutput{Source: result.Source, Cursor: cursor, Content: content, ContentComplete: result.ContentComplete,
		SourceChanged: result.SourceChanged, UpdateAttemptCursor: turn.Purpose != "readiness", Clipping: append([]string(nil), result.Clipping...),
		FallbackReason: result.FallbackReason, Fact: fact, ContentChanged: changed, ProbeEvidence: probeEvidence}, envelope, nil
}

func (l *ExecutionLifecycle) ExecuteHostAction(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	input := request.Input.Host
	if input == nil || input.Validate() != nil {
		return nativeReceipt(request, executionbackend.OperationRejected), errors.New("host action is invalid")
	}
	snapshot := request.Snapshot
	switch input.Kind {
	case executionbackend.HostPrepareProbeFiles:
		if err := l.validateProbeTarget(snapshot, *input.Probe); err != nil {
			return nativeReceipt(request, executionbackend.OperationRejected), err
		}
		plan, err := BuildProbe(*input.Probe)
		if err != nil {
			return nativeReceipt(request, executionbackend.OperationRejected), err
		}
		if err = EnsureReadinessIgnored(snapshot.Session.ProjectID); err == nil {
			err = WriteProbe(*input.Probe, plan)
		}
		if err != nil {
			return nativeReceipt(request, executionbackend.OperationUnknown), err
		}
	case executionbackend.HostCleanupProbeFiles:
		if err := l.validateProbeTarget(snapshot, *input.Probe); err != nil {
			return nativeReceipt(request, executionbackend.OperationRejected), err
		}
		if err := CleanupProbe(*input.Probe); err != nil {
			return nativeReceipt(request, executionbackend.OperationUnknown), err
		}
	case executionbackend.HostOpenOriginal:
		authorization := input.Authorization
		original := input.Process != nil && (snapshot.Ref.External == *input.Process || snapshot.OriginalHost != nil && *snapshot.OriginalHost == *input.Process)
		if !original || authorization == nil || snapshot.HostAuthorization == nil || *authorization != *snapshot.HostAuthorization || authorization.ExecutionID != snapshot.Session.ExecutionID || authorization.AttemptID != snapshot.Session.AttemptID {
			return nativeReceipt(request, executionbackend.OperationRejected), errors.New("host action differs from the original process")
		}
		fact, err := ObserveHost(ctx, l.client.MetaCall, workerTerminalIdentity(*input.Process))
		if err != nil || fact.Reason != "" && fact.Reason != "host_wait" {
			return nativeReceipt(request, executionbackend.OperationUnknown), errors.Join(err, errors.New("original host identity is not verified"))
		}
		if _, err = l.client.MetaCall(ctx, "terminal", "switch", "--terminal", input.Process.Terminal, "--json"); err != nil {
			return nativeReceipt(request, executionbackend.OperationUnknown), err
		}
	case executionbackend.HostRemoveFeedback:
		if input.Worktree != snapshot.Ref.WorkspacePath || input.WorkflowRun != snapshot.Session.WorkflowRunID {
			return nativeReceipt(request, executionbackend.OperationRejected), errors.New("feedback cleanup differs from its original execution")
		}
		state, _, err := l.readHostState(snapshot)
		if err != nil || len(state.Turns) == 0 || state.Turns[len(state.Turns)-1].TurnID != input.TurnID {
			return nativeReceipt(request, executionbackend.OperationRejected), errors.Join(err, errors.New("feedback cleanup differs from the original turn"))
		}
		if err := RemoveRelayRequest(input.Worktree, input.WorkflowRun, input.TurnID); err != nil {
			return nativeReceipt(request, executionbackend.OperationUnknown), err
		}
	default:
		return nativeReceipt(request, executionbackend.OperationRejected), executionbackend.UnsupportedAdapterOperation(string(input.Kind), "unsupported host action")
	}
	return nativeReceipt(request, executionbackend.OperationConfirmed), nil
}

func (l *ExecutionLifecycle) ReconcileHostAction(_ context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	input := request.Input.Host
	if input == nil || input.Validate() != nil {
		return nativeReceipt(request, executionbackend.OperationRejected), errors.New("host action is invalid")
	}
	switch input.Kind {
	case executionbackend.HostPrepareProbeFiles:
		if err := l.validateProbeTarget(request.Snapshot, *input.Probe); err != nil {
			return nativeReceipt(request, executionbackend.OperationRejected), err
		}
		plan, err := BuildProbe(*input.Probe)
		if err != nil {
			return nativeReceipt(request, executionbackend.OperationRejected), err
		}
		present, err := CheckProbeFiles(*input.Probe, plan)
		ignored, ignoreErr := ReadinessIgnored(request.Snapshot.Session.ProjectID)
		if err != nil || ignoreErr != nil {
			return nativeReceipt(request, executionbackend.OperationUnknown), errors.Join(err, ignoreErr)
		}
		if present && ignored {
			return nativeReceipt(request, executionbackend.OperationConfirmed), nil
		}
	case executionbackend.HostCleanupProbeFiles:
		if err := l.validateProbeTarget(request.Snapshot, *input.Probe); err != nil {
			return nativeReceipt(request, executionbackend.OperationRejected), err
		}
		absent, err := ProbeFilesAbsent(*input.Probe)
		if err != nil {
			return nativeReceipt(request, executionbackend.OperationUnknown), err
		}
		if absent {
			return nativeReceipt(request, executionbackend.OperationConfirmed), nil
		}
	case executionbackend.HostRemoveFeedback:
		if input.Worktree != request.Snapshot.Ref.WorkspacePath || input.WorkflowRun != request.Snapshot.Session.WorkflowRunID {
			return nativeReceipt(request, executionbackend.OperationRejected), errors.New("feedback cleanup differs from its original execution")
		}
		state, _, stateErr := l.readHostState(request.Snapshot)
		if stateErr != nil || len(state.Turns) == 0 || state.Turns[len(state.Turns)-1].TurnID != input.TurnID {
			return nativeReceipt(request, executionbackend.OperationRejected), errors.Join(stateErr, errors.New("feedback cleanup differs from the original turn"))
		}
		path, err := RelayPath(input.Worktree, input.WorkflowRun, input.TurnID)
		if err != nil {
			return nativeReceipt(request, executionbackend.OperationRejected), err
		}
		if _, err = os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return nativeReceipt(request, executionbackend.OperationConfirmed), nil
		} else if err != nil {
			return nativeReceipt(request, executionbackend.OperationUnknown), err
		}
	}
	return nativeReceipt(request, executionbackend.OperationUnknown), nil
}

func (l *ExecutionLifecycle) hostStateIdentity(snapshot executionbackend.LifecycleSnapshot) ExecutionStateIdentity {
	return ExecutionStateIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID,
		Generation: snapshot.ResourceGeneration, ConfigDigest: snapshot.ConfigurationDigest, Protocol: snapshot.Ref.Protocol, Process: snapshot.Ref.External,
		CoordinatorTerminal: snapshot.Ref.Target.CoordinatorTerminal}
}

func (l *ExecutionLifecycle) readHostState(snapshot executionbackend.LifecycleSnapshot) (NativeExecutionState, executionbackend.ImplementationDataEnvelope, error) {
	var found *executionbackend.ImplementationDataEnvelope
	for i := range snapshot.Implementation {
		if snapshot.Implementation[i].ImplementationKey != ExecutionStateCodecKey {
			continue
		}
		if found != nil {
			return NativeExecutionState{}, executionbackend.ImplementationDataEnvelope{}, errors.New("duplicate host execution state")
		}
		copy := snapshot.Implementation[i].Clone()
		found = &copy
	}
	if found == nil {
		return NativeExecutionState{}, executionbackend.ImplementationDataEnvelope{}, errors.New("host execution state is unavailable")
	}
	state, err := DecodeExecutionStateEnvelope(*found, l.hostStateIdentity(snapshot))
	return state, *found, err
}

func (l *ExecutionLifecycle) validateProbeTarget(snapshot executionbackend.LifecycleSnapshot, probe executionbackend.ProbeIdentity) error {
	if probe.Execution != snapshot.Session.ExecutionID || probe.Worktree != snapshot.Ref.WorkspacePath || probe.Runtime != snapshot.Ref.External.RuntimeID ||
		probe.Terminal != snapshot.Ref.External.Terminal || probe.Incarnation != snapshot.Ref.External.Incarnation {
		return errors.New("probe action differs from its original execution host")
	}
	if snapshot.PreparationProbe == nil || *snapshot.PreparationProbe != probe {
		return errors.New("probe action differs from the committed preparation identity")
	}
	return nil
}

var _ executionbackend.HostEnvironmentPort = (*ExecutionLifecycle)(nil)
