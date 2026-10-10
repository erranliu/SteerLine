package orcaworker

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

const ExecutionStateCodecKey = "orca.execution-state"

const ExecutionStateCodecVersion = 12

// NativeExecutionState contains provider identities and observation positions.
// WorkflowRun, Decision and Operation remain the authorities for business state.
type NativeExecutionState struct {
	PartialWorker       *partialWorker                   `json:"partial_worker,omitempty"`
	FailedStartReleased bool                             `json:"failed_start_released,omitempty"`
	LocalRuntimeID      string                           `json:"local_runtime_id,omitempty"`
	RemoteRuntimeID     string                           `json:"remote_runtime_id,omitempty"`
	RepositoryID        string                           `json:"repository_id,omitempty"`
	Coordinator         executionbackend.ProcessIdentity `json:"coordinator"`
	Turns               []NativeTurnState                `json:"turns,omitempty"`
	DeliveryID          string                           `json:"delivery_id,omitempty"`
	SeenMessages        []string                         `json:"seen_messages,omitempty"`
	CoordinatorClosed   bool                             `json:"coordinator_closed,omitempty"`
	AgentClosed         bool                             `json:"agent_closed,omitempty"`
	RelayIgnored        bool                             `json:"relay_ignored,omitempty"`
	AgentBound          bool                             `json:"agent_bound,omitempty"`
	AgentLaunchCommand  string                           `json:"agent_launch_command,omitempty"`
}

type NativeTurnState struct {
	InputVersion           uint64                            `json:"input_version,omitempty"`
	ResultProcess          *executionbackend.ProcessIdentity `json:"result_process,omitempty"`
	ResultMessageDigest    string                            `json:"result_message_digest,omitempty"`
	FinalReadError         string                            `json:"final_read_error,omitempty"`
	FinalText              string                            `json:"final_text,omitempty"`
	FinalSource            string                            `json:"final_source,omitempty"`
	FinalItemID            string                            `json:"final_item_id,omitempty"`
	FinalNativeTurnID      string                            `json:"final_native_turn_id,omitempty"`
	FinalVerified          bool                              `json:"final_verified,omitempty"`
	HostWait               *executionbackend.HostFact        `json:"host_wait,omitempty"`
	HostWaitSequence       uint64                            `json:"host_wait_sequence,omitempty"`
	Started                bool                              `json:"started,omitempty"`
	RelayReported          bool                              `json:"relay_reported,omitempty"`
	ToolContext            executionbackend.ToolContext      `json:"tool_context,omitempty"`
	InjectionAttempts      int                               `json:"injection_attempts,omitempty"`
	Purpose                string                            `json:"purpose,omitempty"`
	IntentAt               time.Time                         `json:"intent_at"`
	Settled                bool                              `json:"settled,omitempty"`
	Released               bool                              `json:"released,omitempty"`
	InputSubmissionPending bool                              `json:"input_submission_pending,omitempty"`
	TurnID                 string                            `json:"turn_id"`
	TaskID                 string                            `json:"task_id"`
	DispatchID             string                            `json:"dispatch_id"`
	TaskRequestID          string                            `json:"task_request_id,omitempty"`
	DispatchRequestID      string                            `json:"dispatch_request_id,omitempty"`
	DoneMessageID          string                            `json:"done_message_id,omitempty"`
	Outcome                string                            `json:"outcome,omitempty"`
	LastMessage            string                            `json:"last_message,omitempty"`
	PRCandidates           []string                          `json:"pr_candidates,omitempty"`
	OutputSource           string                            `json:"output_source,omitempty"`
	OutputCursor           string                            `json:"output_cursor,omitempty"`
	OutputComplete         bool                              `json:"output_complete,omitempty"`
	OutputClipping         []string                          `json:"output_clipping,omitempty"`
	OutputFallbackReason   string                            `json:"output_fallback_reason,omitempty"`
	OutputDigest           string                            `json:"output_digest,omitempty"`
}

type ExecutionStateIdentity struct {
	ProjectID           string
	ExecutionID         string
	AttemptID           string
	Generation          string
	ConfigDigest        string
	Protocol            executionbackend.ExecutionProtocol
	Process             executionbackend.ProcessIdentity
	CoordinatorTerminal string
}

func NewExecutionStateEnvelope(identity ExecutionStateIdentity, state NativeExecutionState) (executionbackend.ImplementationDataEnvelope, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	return executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{
		CodecVersion: ExecutionStateCodecVersion, Purpose: executionbackend.ImplementationResourcePurpose,
		ImplementationKey: ExecutionStateCodecKey, ProjectID: identity.ProjectID,
		ExecutionID: identity.ExecutionID, AttemptID: identity.AttemptID,
		Generation: identity.Generation, ConfigDigest: identity.ConfigDigest,
	}, payload)
}

func DecodeExecutionStateEnvelope(envelope executionbackend.ImplementationDataEnvelope, identity ExecutionStateIdentity) (NativeExecutionState, error) {
	if envelope.ImplementationKey != ExecutionStateCodecKey || envelope.CodecVersion != ExecutionStateCodecVersion {
		return NativeExecutionState{}, errors.New("unsupported native execution state codec")
	}
	if err := envelope.ValidateResourceIdentity(identity.ProjectID, identity.ExecutionID, identity.AttemptID, identity.Generation, identity.ConfigDigest); err != nil {
		return NativeExecutionState{}, err
	}
	var state NativeExecutionState
	if payload := bytes.TrimSpace(envelope.Payload); len(payload) == 0 || payload[0] != '{' {
		return NativeExecutionState{}, errors.New("native execution state payload must be an object")
	}
	if err := executionbackend.DecodeStrictJSON(envelope.Payload, &state, "native execution state payload has trailing JSON"); err != nil {
		return NativeExecutionState{}, err
	}
	if identity.CoordinatorTerminal != "" && state.Coordinator.Terminal != identity.CoordinatorTerminal {
		return NativeExecutionState{}, errors.New("native coordinator differs from the original execution target")
	}
	if identity.Protocol == executionbackend.ProtocolOrcaTerminalV2 && identity.Process.RuntimeID != "" && state.Coordinator.Terminal != "" && state.Coordinator.RuntimeID != identity.Process.RuntimeID {
		return NativeExecutionState{}, errors.New("native coordinator belongs to a different local runtime")
	}
	if identity.Protocol == executionbackend.ProtocolOrcaSupervised && identity.Process.RuntimeID != "" && state.RemoteRuntimeID != identity.Process.RuntimeID {
		return NativeExecutionState{}, errors.New("native state belongs to a different remote runtime")
	}
	return state, nil
}
