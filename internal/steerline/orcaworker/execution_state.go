package orcaworker

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

const ExecutionStateCodecKey = "orca.execution-state"

// NativeExecutionState contains provider identities and observation positions.
// WorkflowRun, Decision and Operation remain the authorities for business state.
type NativeExecutionState struct {
	RemoteRuntimeID string                       `json:"remote_runtime_id,omitempty"`
	RepositoryID    string                       `json:"repository_id,omitempty"`
	Coordinator     executionapi.ProcessIdentity `json:"coordinator"`
	Turns           []NativeTurnState            `json:"turns,omitempty"`
	DeliveryID      string                       `json:"delivery_id,omitempty"`
	SeenMessages    []string                     `json:"seen_messages,omitempty"`
}

type NativeTurnState struct {
	ToolContext          contracts.ToolContext `json:"tool_context,omitempty"`
	InjectionAttempts    int                   `json:"injection_attempts,omitempty"`
	Purpose              string                `json:"purpose,omitempty"`
	IntentAt             time.Time             `json:"intent_at"`
	Settled              bool                  `json:"settled,omitempty"`
	TurnID               string                `json:"turn_id"`
	TaskID               string                `json:"task_id"`
	DispatchID           string                `json:"dispatch_id"`
	TaskRequestID        string                `json:"task_request_id,omitempty"`
	DispatchRequestID    string                `json:"dispatch_request_id,omitempty"`
	DoneMessageID        string                `json:"done_message_id,omitempty"`
	Outcome              string                `json:"outcome,omitempty"`
	OutputSource         string                `json:"output_source,omitempty"`
	OutputCursor         string                `json:"output_cursor,omitempty"`
	OutputComplete       bool                  `json:"output_complete,omitempty"`
	OutputClipping       []string              `json:"output_clipping,omitempty"`
	OutputFallbackReason string                `json:"output_fallback_reason,omitempty"`
	OutputDigest         string                `json:"output_digest,omitempty"`
}

type ExecutionStateIdentity struct {
	ProjectID           string
	ExecutionID         string
	AttemptID           string
	Generation          string
	ConfigDigest        string
	Protocol            executionapi.ExecutionProtocol
	Process             executionapi.ProcessIdentity
	CoordinatorTerminal string
}

func NewExecutionStateEnvelope(identity ExecutionStateIdentity, state NativeExecutionState) (executionapi.ImplementationDataEnvelope, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return executionapi.ImplementationDataEnvelope{}, err
	}
	return executionapi.NewImplementationDataEnvelope(executionapi.ImplementationDataEnvelope{
		CodecVersion: 1, Purpose: executionapi.ImplementationResourcePurpose,
		ImplementationKey: ExecutionStateCodecKey, ProjectID: identity.ProjectID,
		ExecutionID: identity.ExecutionID, AttemptID: identity.AttemptID,
		Generation: identity.Generation, ConfigDigest: identity.ConfigDigest,
	}, payload)
}

func DecodeExecutionStateEnvelope(envelope executionapi.ImplementationDataEnvelope, identity ExecutionStateIdentity) (NativeExecutionState, error) {
	if envelope.ImplementationKey != ExecutionStateCodecKey || envelope.CodecVersion != 1 {
		return NativeExecutionState{}, errors.New("unsupported native execution state codec")
	}
	if err := envelope.ValidateResourceIdentity(identity.ProjectID, identity.ExecutionID, identity.AttemptID, identity.Generation, identity.ConfigDigest); err != nil {
		return NativeExecutionState{}, err
	}
	var state NativeExecutionState
	if payload := bytes.TrimSpace(envelope.Payload); len(payload) == 0 || payload[0] != '{' {
		return NativeExecutionState{}, errors.New("native execution state payload must be an object")
	}
	decoder := json.NewDecoder(bytes.NewReader(envelope.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return NativeExecutionState{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return NativeExecutionState{}, errors.New("native execution state payload has trailing JSON")
	}
	if identity.CoordinatorTerminal != "" && state.Coordinator.Terminal != identity.CoordinatorTerminal {
		return NativeExecutionState{}, errors.New("native coordinator differs from the original execution target")
	}
	if identity.Protocol == executionapi.ProtocolOrcaTerminalV2 && identity.Process.RuntimeID != "" && state.Coordinator.Terminal != "" && state.Coordinator.RuntimeID != identity.Process.RuntimeID {
		return NativeExecutionState{}, errors.New("native coordinator belongs to a different local runtime")
	}
	if identity.Protocol == executionapi.ProtocolOrcaSupervised && identity.Process.RuntimeID != "" && state.RemoteRuntimeID != identity.Process.RuntimeID {
		return NativeExecutionState{}, errors.New("native state belongs to a different remote runtime")
	}
	return state, nil
}
