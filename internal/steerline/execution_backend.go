package steerline

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type CapabilityState string

const (
	CapabilityVerified    CapabilityState = "verified"
	CapabilityUnsupported CapabilityState = "unsupported"
	CapabilityNotTested   CapabilityState = "not_tested"
)

type CapabilityEvidence struct {
	Name      string          `json:"name"`
	State     CapabilityState `json:"state"`
	Version   string          `json:"version,omitempty"`
	Target    string          `json:"target,omitempty"`
	Evidence  string          `json:"evidence,omitempty"`
	CheckedAt time.Time       `json:"checked_at"`
}

type ProbeResult struct {
	Backend      string               `json:"backend"`
	Protocol     string               `json:"protocol"`
	Agent        string               `json:"agent"`
	Target       ExecutionTarget      `json:"target"`
	Capabilities []CapabilityEvidence `json:"capabilities"`
	Version      string               `json:"version,omitempty"`
	Diagnostic   string               `json:"diagnostic,omitempty"`
}

type AttemptSpec struct {
	ProjectIdentity string          `json:"project_identity"`
	TaskID          string          `json:"task_id"`
	WorkflowRunID   string          `json:"workflow_run_id"`
	ExecutionID     string          `json:"execution_id"`
	AttemptID       string          `json:"attempt_id"`
	OperationID     string          `json:"operation_id"`
	WorkflowVersion string          `json:"workflow_version"`
	StepID          string          `json:"step_id"`
	ConfigDigest    string          `json:"config_digest"`
	ContractDigest  string          `json:"contract_digest"`
	Repository      string          `json:"repository"`
	BaseRevision    string          `json:"base_revision"`
	Worktree        string          `json:"worktree"`
	Branch          string          `json:"branch"`
	Agent           string          `json:"agent"`
	Backend         string          `json:"backend"`
	Protocol        string          `json:"protocol"`
	Target          ExecutionTarget `json:"target"`
	Input           []byte          `json:"input"`
	Timeout         time.Duration   `json:"timeout"`
}

type Operation struct {
	ID                string `json:"id"`
	IdempotencyKey    string `json:"idempotency_key"`
	Kind              string `json:"kind"`
	PayloadDigest     string `json:"payload_digest"`
	Payload           []byte `json:"payload,omitempty"`
	PlatformRequestID string `json:"platform_request_id,omitempty"`
}

type ExecutionRef struct {
	ProjectIdentity string           `json:"project_identity"`
	WorkflowRunID   string           `json:"workflow_run_id"`
	ExecutionID     string           `json:"execution_id"`
	AttemptID       string           `json:"attempt_id"`
	Backend         string           `json:"backend"`
	Protocol        string           `json:"protocol"`
	Agent           string           `json:"agent"`
	Target          ExecutionTarget  `json:"target"`
	Process         *ProcessIdentity `json:"process,omitempty"`
	BoundAt         time.Time        `json:"bound_at"`
}

type ActionDisposition string

const (
	ActionAccepted  ActionDisposition = "accepted"
	ActionRejected  ActionDisposition = "rejected"
	ActionUnknown   ActionDisposition = "outcome_unknown"
	ActionConfirmed ActionDisposition = "confirmed"
)

type ActionReceipt struct {
	Disposition ActionDisposition `json:"disposition"`
	Execution   *ExecutionRef     `json:"execution,omitempty"`
	RequestID   string            `json:"request_id,omitempty"`
	Diagnostic  string            `json:"diagnostic,omitempty"`
	Resources   []string          `json:"resources,omitempty"`
	ObservedAt  time.Time         `json:"observed_at"`
}

type Observation struct {
	ExecutionRef ExecutionRef `json:"execution_ref"`
	Liveness     string       `json:"liveness"`
	Connection   string       `json:"connection"`
	WaitReason   string       `json:"wait_reason,omitempty"`
	ObservedAt   time.Time    `json:"observed_at"`
	Source       string       `json:"source"`
	RawStatus    string       `json:"raw_status,omitempty"`
}

type WorkspaceInspection struct {
	ExecutionRef ExecutionRef      `json:"execution_ref"`
	Evidence     WorkspaceEvidence `json:"evidence"`
	ObservedAt   time.Time         `json:"observed_at"`
	Source       string            `json:"source"`
}

type OutputBatch struct {
	ExecutionRef ExecutionRef `json:"execution_ref"`
	Items        [][]byte     `json:"items"`
	NextCursor   string       `json:"next_cursor,omitempty"`
	Gap          string       `json:"gap,omitempty"`
	Source       string       `json:"source"`
	ObservedAt   time.Time    `json:"observed_at"`
}

type ConversationDelivery struct {
	Source       string       `json:"source"`
	ExternalID   string       `json:"external_id"`
	ExecutionRef ExecutionRef `json:"execution_ref"`
	AttemptID    string       `json:"attempt_id"`
	DecisionID   string       `json:"decision_id,omitempty"`
	Payload      []byte       `json:"payload"`
	ReceivedAt   time.Time    `json:"received_at"`
}

type ResourceDisposition string

const (
	ResourceRelease ResourceDisposition = "release"
	ResourceRetain  ResourceDisposition = "retain"
)

type ExecutionBackend interface {
	BackendIdentity() (backend, protocol string)
	Probe(context.Context, ExecutionTarget, string) (ProbeResult, error)
	Preflight(context.Context, AttemptSpec) error
	Prepare(context.Context, AttemptSpec, Operation) (ExecutionRef, ActionReceipt, error)
	Start(context.Context, AttemptSpec, Operation) (ExecutionRef, ActionReceipt, error)
	Observe(context.Context, ExecutionRef) (Observation, error)
	InspectWorkspace(context.Context, ExecutionRef) (WorkspaceInspection, error)
	ReadOutput(context.Context, ExecutionRef, string, int) (OutputBatch, error)
	Receive(context.Context, string, int) ([]ConversationDelivery, ActionReceipt, error)
	Ack(context.Context, ConversationDelivery) (ActionReceipt, error)
	Reply(context.Context, ExecutionRef, string, []byte, Operation) (ActionReceipt, error)
	StopExecution(context.Context, ExecutionRef, Operation) (ActionReceipt, error)
	DetachObservation(context.Context, ExecutionRef) error
	SettleResources(context.Context, ExecutionRef, ResourceDisposition, Operation) (ActionReceipt, error)
	ReconcileOperation(context.Context, ExecutionRef, Operation) (ActionReceipt, error)
}

type BackendRegistry struct{ backends map[string]ExecutionBackend }

func NewBackendRegistry() *BackendRegistry {
	return &BackendRegistry{backends: map[string]ExecutionBackend{}}
}
func backendKey(backend, protocol string) string { return backend + "\x00" + protocol }
func (r *BackendRegistry) Register(adapter ExecutionBackend) error {
	if r == nil || adapter == nil {
		return errors.New("backend registry and adapter are required")
	}
	backend, protocol := adapter.BackendIdentity()
	if backend == "" || protocol == "" {
		return errors.New("adapter must declare backend and protocol")
	}
	key := backendKey(backend, protocol)
	if _, exists := r.backends[key]; exists {
		return fmt.Errorf("execution adapter %s/%s is already registered", backend, protocol)
	}
	r.backends[key] = adapter
	return nil
}
func (r *BackendRegistry) Resolve(backend, protocol string) (ExecutionBackend, error) {
	if r != nil {
		if adapter := r.backends[backendKey(backend, protocol)]; adapter != nil {
			return adapter, nil
		}
	}
	return nil, &AdapterError{Kind: AdapterUnsupported, Operation: "resolve", Diagnostic: fmt.Sprintf("no adapter registered for backend=%q protocol=%q; fallback is disabled", backend, protocol)}
}

type AdapterErrorKind string

const (
	AdapterUnsupported      AdapterErrorKind = "capability_unsupported"
	AdapterRejected         AdapterErrorKind = "rejected"
	AdapterUnreachable      AdapterErrorKind = "unreachable"
	AdapterIdentityMismatch AdapterErrorKind = "identity_mismatch"
	AdapterProtocolInvalid  AdapterErrorKind = "protocol_invalid"
	AdapterOutcomeUnknown   AdapterErrorKind = "outcome_unknown"
)

type AdapterError struct {
	Kind       AdapterErrorKind
	Operation  string
	Diagnostic string
	Cause      error
}

func (e *AdapterError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s %s: %s: %v", e.Kind, e.Operation, e.Diagnostic, e.Cause)
	}
	return fmt.Sprintf("%s %s: %s", e.Kind, e.Operation, e.Diagnostic)
}
func (e *AdapterError) Unwrap() error { return e.Cause }
func UnsupportedAdapterOperation(operation, diagnostic string) error {
	return &AdapterError{Kind: AdapterUnsupported, Operation: operation, Diagnostic: diagnostic}
}
func IsAdapterError(err error, kind AdapterErrorKind) bool {
	var target *AdapterError
	return errors.As(err, &target) && target.Kind == kind
}

// RemoteStopUnsupported is the current supervised Orca capability result. It
// deliberately carries no accepted receipt: a center disconnect is not a stop.
func RemoteStopUnsupported() (ActionReceipt, error) {
	return ActionReceipt{Disposition: ActionRejected, Diagnostic: "capability_unsupported: current orca_supervised target has no production stop operation", ObservedAt: now()}, UnsupportedAdapterOperation("StopExecution", "current supervised remote target does not support cancellation")
}

func ExecutionBackendKind(host string) (backend, protocol string, err error) {
	switch host {
	case "", "local":
		return "local_process", "local_process", nil
	case "orca":
		return "orca", "orca_terminal_v2", nil
	default:
		return "", "", fmt.Errorf("unsupported execution host %q; fallback is disabled", host)
	}
}
