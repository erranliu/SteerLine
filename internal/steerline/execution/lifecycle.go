package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// LifecycleAction names one generic, durable side effect. Implementations may
// choose native steps, while the center persists each candidate before calling
// Execute and commits its receipt before asking for another candidate.
type LifecycleAction string

const (
	LifecyclePrepareWorkspace LifecycleAction = "prepare_workspace"
	LifecycleCreateResource   LifecycleAction = "create_resource"
	LifecyclePrepareSession   LifecycleAction = "prepare_session"
	LifecycleStartTurn        LifecycleAction = "start_turn"
	LifecycleSubmitInput      LifecycleAction = "submit_input"
	LifecycleReportCompletion LifecycleAction = "report_completion"
	LifecycleDeliverFeedback  LifecycleAction = "deliver_feedback"
	LifecycleAcknowledge      LifecycleAction = "acknowledge_delivery"
	LifecycleStopObservation  LifecycleAction = "stop_observation"
	LifecycleStopExecution    LifecycleAction = "stop_execution"
	LifecycleCleanup          LifecycleAction = "cleanup_resources"
)

// LifecycleInput is a closed value union. Exactly one member must match the
// action. Native configuration and bindings remain in implementation-owned
// envelopes and are decoded only by the selected implementation.
type LifecycleInput struct {
	Workspace  *WorkspaceIntent  `json:"workspace,omitempty"`
	Resource   *ResourceIntent   `json:"resource,omitempty"`
	Turn       *TurnInput        `json:"turn,omitempty"`
	Completion *CompletionInput  `json:"completion,omitempty"`
	Feedback   *FeedbackInput    `json:"feedback,omitempty"`
	Ack        *AcknowledgeInput `json:"ack,omitempty"`
	Stop       *StopInput        `json:"stop,omitempty"`
	Cleanup    *CleanupInput     `json:"cleanup,omitempty"`
}

type FeedbackInput struct {
	DecisionID string  `json:"decision_id,omitempty"`
	QuestionID string  `json:"question_id,omitempty"`
	Message    string  `json:"message,omitempty"`
	Answer     *Answer `json:"answer,omitempty"`
}

// ResourceIntent describes one generic resource creation. Purpose is a closed
// implementation-owned value interpreted only by the selected lifecycle port.
type ResourceIntent struct {
	Purpose     string `json:"purpose"`
	Name        string `json:"name,omitempty"`
	Objective   string `json:"objective,omitempty"`
	Base        string `json:"base,omitempty"`
	TurnID      string `json:"turn_id,omitempty"`
	TurnPurpose string `json:"turn_purpose,omitempty"`
}

type CompletionInput struct {
	TurnID  string `json:"turn_id"`
	Outcome string `json:"outcome"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type AcknowledgeInput struct {
	DeliveryID string `json:"delivery_id"`
}

type StopInput struct {
	TurnID string `json:"turn_id,omitempty"`
}

type CleanupInput struct {
	Resources []ProcessIdentity `json:"resources,omitempty"`
	Workspace *WorkspaceIntent  `json:"workspace,omitempty"`
}

func (input LifecycleInput) Validate(action LifecycleAction) error {
	count := 0
	if input.Workspace != nil {
		count++
	}
	if input.Resource != nil {
		count++
	}
	if input.Turn != nil {
		count++
	}
	if input.Completion != nil {
		count++
	}
	if input.Feedback != nil {
		count++
	}
	if input.Ack != nil {
		count++
	}
	if input.Stop != nil {
		count++
	}
	if input.Cleanup != nil {
		count++
	}
	valid := false
	switch action {
	case LifecyclePrepareWorkspace:
		valid = input.Workspace != nil
	case LifecycleCreateResource:
		valid = input.Resource != nil && input.Resource.Purpose != ""
	case LifecyclePrepareSession, LifecycleStartTurn, LifecycleSubmitInput:
		valid = input.Turn != nil
	case LifecycleReportCompletion:
		valid = input.Completion != nil && input.Completion.TurnID != "" && (input.Completion.Outcome == "succeeded" || input.Completion.Outcome == "failed")
	case LifecycleDeliverFeedback:
		valid = input.Feedback != nil
	case LifecycleAcknowledge:
		valid = input.Ack != nil
	case LifecycleStopObservation, LifecycleStopExecution:
		valid = input.Stop != nil
	case LifecycleCleanup:
		valid = input.Cleanup != nil
	}
	if count != 1 || !valid {
		return errors.New("lifecycle input does not match exactly one action")
	}
	return nil
}

func (input LifecycleInput) Clone() LifecycleInput {
	if input.Workspace != nil {
		value := *input.Workspace
		input.Workspace = &value
	}
	if input.Resource != nil {
		value := *input.Resource
		input.Resource = &value
	}
	if input.Turn != nil {
		value := *input.Turn
		value.LinkedWorktreeArgs = append([]string(nil), value.LinkedWorktreeArgs...)
		input.Turn = &value
	}
	if input.Completion != nil {
		value := *input.Completion
		input.Completion = &value
	}
	if input.Feedback != nil {
		value := *input.Feedback
		if value.Answer != nil {
			answer := cloneAnswer(*value.Answer)
			value.Answer = &answer
		}
		input.Feedback = &value
	}
	if input.Ack != nil {
		value := *input.Ack
		input.Ack = &value
	}
	if input.Stop != nil {
		value := *input.Stop
		input.Stop = &value
	}
	if input.Cleanup != nil {
		value := *input.Cleanup
		value.Resources = append([]ProcessIdentity(nil), value.Resources...)
		if value.Workspace != nil {
			workspace := *value.Workspace
			value.Workspace = &workspace
		}
		input.Cleanup = &value
	}
	return input
}

// LifecycleSnapshot is a read-only view supplied to a planner. The slices are
// values, not storage handles; implementations cannot commit business state.
type LifecycleSnapshot struct {
	Ref                 ExecutionRef
	Session             SessionContext
	TaskTitle           string
	WorkflowPhase       string
	TaskDelivered       bool
	ConfigurationDigest string
	ResourceGeneration  string
	Implementation      []ImplementationDataEnvelope
	PolicyResources     *ImplementationDataEnvelope
	Operations          []Operation
	Workspace           WorkspaceIntent
	WorkspacePlan       WorkspaceIntent
	PendingTurn         *TurnInput
	QuestionRoute       *QuestionRoute
	OutputCursor        string
	PendingDeliveryID   string
	KnownQuestionIDs    []string
	PendingQuestions    []Question
}

func (snapshot LifecycleSnapshot) Clone() LifecycleSnapshot {
	snapshot.Implementation = cloneImplementationData(snapshot.Implementation)
	if snapshot.PolicyResources != nil {
		resource := snapshot.PolicyResources.Clone()
		snapshot.PolicyResources = &resource
	}
	operations := make([]Operation, len(snapshot.Operations))
	for i, operation := range snapshot.Operations {
		operations[i] = operation
		operations[i].ResidualResources = append([]string(nil), operation.ResidualResources...)
		operations[i].Request = append([]byte(nil), operation.Request...)
		operations[i].Receipt = append([]byte(nil), operation.Receipt...)
		if operation.Ref != nil {
			ref := *operation.Ref
			operations[i].Ref = &ref
		}
	}
	snapshot.Operations = operations
	snapshot.PendingQuestions = cloneQuestions(snapshot.PendingQuestions)
	snapshot.KnownQuestionIDs = append([]string(nil), snapshot.KnownQuestionIDs...)
	if snapshot.PendingTurn != nil {
		turn := *snapshot.PendingTurn
		turn.LinkedWorktreeArgs = append([]string(nil), turn.LinkedWorktreeArgs...)
		snapshot.PendingTurn = &turn
	}
	if snapshot.QuestionRoute != nil {
		route := snapshot.QuestionRoute.Clone()
		snapshot.QuestionRoute = &route
	}
	return snapshot
}

// LifecycleCandidate contains one proposed operation. Its identity and digest
// are filled and persisted by the center after qualification checks pass.
type LifecycleCandidate struct {
	Action LifecycleAction `json:"action"`
	Input  LifecycleInput  `json:"input"`
	Done   bool            `json:"done,omitempty"`
}

func (candidate LifecycleCandidate) Validate() error {
	if candidate.Done {
		if candidate.Action != "" || candidate.Input != (LifecycleInput{}) {
			return errors.New("completed lifecycle plan must not carry an action or input")
		}
		return nil
	}
	switch candidate.Action {
	case LifecyclePrepareWorkspace, LifecycleCreateResource, LifecyclePrepareSession, LifecycleStartTurn, LifecycleSubmitInput, LifecycleReportCompletion,
		LifecycleDeliverFeedback, LifecycleAcknowledge, LifecycleStopObservation,
		LifecycleStopExecution, LifecycleCleanup:
		return candidate.Input.Validate(candidate.Action)
	default:
		return errors.New("unsupported lifecycle action")
	}
}

type LifecycleRequest struct {
	Snapshot  LifecycleSnapshot
	Operation Operation
	Input     LifecycleInput
}

func (request LifecycleRequest) Validate() error {
	if request.Operation.ID == "" || request.Operation.PayloadDigest == "" || request.Operation.Action == "" {
		return errors.New("lifecycle request has no durable operation identity")
	}
	candidate := LifecycleCandidate{Action: LifecycleAction(request.Operation.Action), Input: request.Input}
	return candidate.Validate()
}

func (request LifecycleRequest) Clone() LifecycleRequest {
	request.Snapshot = request.Snapshot.Clone()
	request.Operation.ResidualResources = append([]string(nil), request.Operation.ResidualResources...)
	request.Operation.Request = append([]byte(nil), request.Operation.Request...)
	request.Operation.Receipt = append([]byte(nil), request.Operation.Receipt...)
	if request.Operation.Ref != nil {
		ref := *request.Operation.Ref
		request.Operation.Ref = &ref
	}
	request.Input = request.Input.Clone()
	return request
}

type LifecycleReceipt struct {
	Operation      Operation                    `json:"operation"`
	Disposition    OperationStatus              `json:"disposition"`
	RequestID      string                       `json:"request_id,omitempty"`
	Diagnostic     string                       `json:"diagnostic,omitempty"`
	Process        *ProcessIdentity             `json:"process,omitempty"`
	Target         *ExecutionTarget             `json:"target,omitempty"`
	Workspace      *WorkspaceInspection         `json:"workspace,omitempty"`
	Implementation []ImplementationDataEnvelope `json:"implementation,omitempty"`
	Facts          []EventFact                  `json:"facts,omitempty"`
	DeliveryID     string                       `json:"delivery_id,omitempty"`
	EventPath      string                       `json:"event_path,omitempty"`
	State          LifecycleStateFacts          `json:"state,omitempty"`
	TurnEvidence   *TurnEvidence                `json:"turn_evidence,omitempty"`
	Handle         StopHandle                   `json:"-"`
}

// LifecycleStateFacts contains only general execution facts. It deliberately
// omits native outcome fields, which each implementation resolves to these
// values before returning a receipt.
type LifecycleStateFacts struct {
	SessionID         string `json:"session_id,omitempty"`
	SessionGeneration string `json:"session_generation,omitempty"`
	TurnID            string `json:"turn_id,omitempty"`
	InputAccepted     bool   `json:"input_accepted,omitempty"`
	TurnStarted       bool   `json:"turn_started,omitempty"`
	TurnCompleted     bool   `json:"turn_completed,omitempty"`
	WorkerAbsent      bool   `json:"worker_absent,omitempty"`
	WritersAbsent     bool   `json:"writers_absent,omitempty"`
	QuestionPending   bool   `json:"question_pending,omitempty"`
	Stopped           bool   `json:"stopped,omitempty"`
	ResourceID        string `json:"resource_id,omitempty"`
	RuntimeID         string `json:"runtime_id,omitempty"`
	WorktreeID        string `json:"worktree_id,omitempty"`
}

func (receipt LifecycleReceipt) Clone() LifecycleReceipt {
	receipt.Operation.ResidualResources = append([]string(nil), receipt.Operation.ResidualResources...)
	receipt.Operation.Request = append([]byte(nil), receipt.Operation.Request...)
	receipt.Operation.Receipt = append([]byte(nil), receipt.Operation.Receipt...)
	if receipt.Operation.Ref != nil {
		ref := *receipt.Operation.Ref
		receipt.Operation.Ref = &ref
	}
	if receipt.Process != nil {
		process := *receipt.Process
		receipt.Process = &process
	}
	if receipt.Target != nil {
		target := *receipt.Target
		receipt.Target = &target
	}
	if receipt.Workspace != nil {
		workspace := *receipt.Workspace
		receipt.Workspace = &workspace
	}
	receipt.Implementation = cloneImplementationData(receipt.Implementation)
	receipt.Facts = cloneFacts(receipt.Facts)
	if receipt.TurnEvidence != nil {
		facts := *receipt.TurnEvidence
		facts.PRCandidates = append([]string(nil), facts.PRCandidates...)
		receipt.TurnEvidence = &facts
	}
	return receipt
}

func (receipt LifecycleReceipt) ValidateFor(request LifecycleRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	want, got := request.Operation, receipt.Operation
	if got.ID != want.ID || got.RunID != want.RunID || got.AttemptID != want.AttemptID || got.Backend != want.Backend || got.IdempotencyKey != want.IdempotencyKey ||
		got.Protocol != want.Protocol || got.Target != want.Target || got.Action != want.Action || got.PayloadDigest != want.PayloadDigest || !bytes.Equal(got.Request, want.Request) ||
		!sameLifecycleRef(got.Ref, want.Ref) {
		return fmt.Errorf("lifecycle receipt changed its original operation identity or input binding (id=%t run=%t attempt=%t backend=%t protocol=%t target=%t action=%t digest=%t ref=%t)", got.ID == want.ID, got.RunID == want.RunID, got.AttemptID == want.AttemptID, got.Backend == want.Backend, got.Protocol == want.Protocol, got.Target == want.Target, got.Action == want.Action, got.PayloadDigest == want.PayloadDigest, sameLifecycleRef(got.Ref, want.Ref))
	}
	if receipt.Disposition != OperationAccepted && receipt.Disposition != OperationConfirmed && receipt.Disposition != OperationRejected && receipt.Disposition != OperationUnknown {
		return errors.New("lifecycle receipt has no terminal or accepted disposition")
	}
	return nil
}

// CandidateForOperation reconstructs the original single-step value so a
// planner can select read-only reconciliation after an unknown receipt.
func CandidateForOperation(operation Operation) (LifecycleCandidate, error) {
	action := LifecycleAction(operation.Action)
	var input LifecycleInput
	decode := func(target any) error {
		decoder := json.NewDecoder(bytes.NewReader(operation.Request))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(target); err != nil {
			return err
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return errors.New("saved lifecycle request has trailing JSON")
		}
		return nil
	}
	switch action {
	case LifecyclePrepareWorkspace:
		var value WorkspaceIntent
		if err := decode(&value); err != nil {
			return LifecycleCandidate{}, err
		}
		input.Workspace = &value
	case LifecycleCreateResource:
		var value ResourceIntent
		if err := decode(&value); err != nil {
			return LifecycleCandidate{}, err
		}
		input.Resource = &value
	case LifecyclePrepareSession, LifecycleStartTurn, LifecycleSubmitInput:
		var value TurnInput
		if err := decode(&value); err != nil {
			return LifecycleCandidate{}, err
		}
		input.Turn = &value
	case LifecycleReportCompletion:
		var value CompletionInput
		if err := decode(&value); err != nil {
			return LifecycleCandidate{}, err
		}
		input.Completion = &value
	case LifecycleDeliverFeedback:
		var value FeedbackInput
		if err := decode(&value); err != nil {
			return LifecycleCandidate{}, err
		}
		input.Feedback = &value
	case LifecycleAcknowledge:
		var value AcknowledgeInput
		if err := decode(&value); err != nil {
			return LifecycleCandidate{}, err
		}
		input.Ack = &value
	case LifecycleStopObservation, LifecycleStopExecution:
		var value StopInput
		if err := decode(&value); err != nil {
			return LifecycleCandidate{}, err
		}
		input.Stop = &value
	case LifecycleCleanup:
		var value CleanupInput
		if err := decode(&value); err != nil {
			return LifecycleCandidate{}, err
		}
		input.Cleanup = &value
	default:
		return LifecycleCandidate{}, errors.New("saved Operation is not a lifecycle action")
	}
	candidate := LifecycleCandidate{Action: action, Input: input}
	return candidate, candidate.Validate()
}

func sameLifecycleRef(a, b *ExecutionRef) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func cloneImplementationData(values []ImplementationDataEnvelope) []ImplementationDataEnvelope {
	cloned := make([]ImplementationDataEnvelope, len(values))
	for i, value := range values {
		cloned[i] = value.Clone()
	}
	return cloned
}

func cloneQuestions(values []Question) []Question {
	cloned := make([]Question, len(values))
	for i, value := range values {
		cloned[i] = value
		cloned[i].Options = append([]DecisionOption(nil), value.Options...)
	}
	return cloned
}

func cloneFacts(values []EventFact) []EventFact {
	cloned := make([]EventFact, len(values))
	for i, value := range values {
		cloned[i] = value
		cloned[i].Questions = cloneQuestions(value.Questions)
		cloned[i].QuestionOptions = append([]DecisionOption(nil), value.QuestionOptions...)
		cloned[i].PRCandidates = append([]string(nil), value.PRCandidates...)
		if value.QuestionRoute != nil {
			route := value.QuestionRoute.Clone()
			cloned[i].QuestionRoute = &route
		}
		if value.ToolReply != nil {
			reply := *value.ToolReply
			reply.Answers = make([]map[string][]string, len(value.ToolReply.Answers))
			for j, answers := range value.ToolReply.Answers {
				reply.Answers[j] = cloneAnswers(answers)
			}
			if value.ToolReply.Success != nil {
				success := *value.ToolReply.Success
				reply.Success = &success
			}
			cloned[i].ToolReply = &reply
		}
	}
	return cloned
}

func (candidate LifecycleCandidate) Clone() LifecycleCandidate {
	candidate.Input = candidate.Input.Clone()
	return candidate
}

func cloneAnswer(answer Answer) Answer {
	answer.Answers = cloneAnswers(answer.Answers)
	return answer
}

func cloneAnswers(values map[string][]string) map[string][]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string][]string, len(values))
	for key, value := range values {
		cloned[key] = append([]string(nil), value...)
	}
	return cloned
}

type LifecycleEvent struct {
	Fact           EventFact
	Implementation []ImplementationDataEnvelope
	Candidate      *LifecycleCandidate
	DeliveryID     string
	DeliveryFinal  bool
	Summary        string
	Kind           string
	Status         string
	Title          string
	Persisted      chan error
	Processed      chan error
}

func (event LifecycleEvent) Clone() LifecycleEvent {
	event.Implementation = cloneImplementationData(event.Implementation)
	event.Fact = cloneFacts([]EventFact{event.Fact})[0]
	if event.Candidate != nil {
		candidate := event.Candidate.Clone()
		event.Candidate = &candidate
	}
	return event
}

// LifecycleObserver is an implementation-owned read-only stream. The center
// acknowledges only after it commits the event facts.
type LifecycleObserver interface {
	Events() <-chan LifecycleEvent
	Done() <-chan error
	StopHandle() StopHandle
	Close(context.Context) error
}

type LifecyclePlanPort interface {
	Next(context.Context, LifecycleSnapshot) (LifecycleCandidate, error)
}

type LifecycleProbePort interface {
	PreflightLifecycle(context.Context, LifecycleSetup) (CapabilityReport, error)
}

type LifecycleExecutePort interface {
	Execute(context.Context, LifecycleRequest) (LifecycleReceipt, LifecycleObserver, error)
}

type LifecycleReconcilePort interface {
	Reconcile(context.Context, LifecycleRequest) (LifecycleReceipt, LifecycleObserver, error)
}

type LifecycleObservePort interface {
	ObserveLifecycle(context.Context, LifecycleSnapshot) (LifecycleReceipt, LifecycleObserver, error)
}

type LifecycleStopPort interface {
	StopLifecycle(context.Context, LifecycleRequest, StopHandle) (LifecycleReceipt, error)
}

type LifecycleCleanupPort interface {
	CleanupLifecycle(context.Context, LifecycleRequest) (LifecycleReceipt, error)
}

type LifecycleRecoveryEvidence struct {
	SessionAvailable bool
	ManagedProcess   bool
	ProcessAbsent    bool
}

type LifecycleRecoveryPort interface {
	VerifyManualRecovery(context.Context, LifecycleSnapshot) (LifecycleRecoveryEvidence, error)
	BindRecovery(context.Context, LifecycleSnapshot, LifecycleSnapshot) (ImplementationDataEnvelope, error)
}

type LifecyclePolicyCapabilities struct {
	LinkedWorkspaceWriteRoots bool
	SeparateTurnStart         bool
	ResumeActiveObservation   bool
}

type LifecyclePolicyPort interface {
	PolicyCapabilities() LifecyclePolicyCapabilities
}

// LifecyclePort composes the narrow capabilities needed by a service. Each
// mutation receives one already-persisted operation; reconciliation is a
// separate read-only method so an unknown operation can never be resubmitted.
type LifecyclePort interface {
	LifecycleProbePort
	LifecyclePlanPort
	LifecycleExecutePort
	LifecycleReconcilePort
	LifecycleObservePort
	LifecycleStopPort
	LifecycleCleanupPort
	LifecycleRecoveryPort
	LifecyclePolicyPort
}

type LifecycleResolver interface {
	ResolveLifecycle(LifecycleSetup) (LifecyclePort, error)
}

// LifecycleSetup carries only fixed, non-secret values needed to construct an
// implementation port. It is not a center record or a write capability.
type LifecycleSetup struct {
	Snapshot              LifecycleSnapshot
	ProfileData           []ImplementationDataEnvelope
	ProfileID             string
	AcceptedConfigVersion string
	Policy                ExecutionPolicy
	ProcessHost           ManagedProcessHost
	RepositoryPath        string
	RunDirectory          string
	CommandTimeout        time.Duration
	AttemptNumber         int
	BlockedEnvironment    []string
	GitExecutable         string
	GitArgs               []string
	RemoteName            string
	TargetBranch          string
}

func (setup LifecycleSetup) Clone() LifecycleSetup {
	setup.Snapshot = setup.Snapshot.Clone()
	setup.ProfileData = cloneImplementationData(setup.ProfileData)
	setup.BlockedEnvironment = append([]string(nil), setup.BlockedEnvironment...)
	setup.GitArgs = append([]string(nil), setup.GitArgs...)
	return setup
}
