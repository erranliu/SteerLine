package clientcontracts

import (
	"context"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/task"
	"github.com/erranliu/SteerLine/internal/steerline/taskview"
)

// ProjectRef binds a view to both its config and state location. It contains
// only public identity, never a configuration object or a writable runtime.
type ProjectRef struct {
	Key, Name, ConfigPath, Identity, RunDir          string
	ApplicationHome, ApplicationBin, Error, Conflict string
	Presentation                                     ProjectPresentation
}

type ProjectSnapshot struct {
	CandidateFreshnessLimit time.Duration
	DecisionReasons         map[string]string
	SelectedTaskID          string
	FullReadAt              time.Time
	Connection              ServiceConnection
	Observation             ObservationSnapshot
	Tasks                   *contracts.TaskSnapshot
	TaskError               string
	NativeTasks             *contracts.NativeTaskPage
	SelectedNative          *contracts.NativeTaskDetail
	NativeTaskError         string
	Domain                  *contracts.DomainSnapshot
	DomainError             string
}

type LogBatch struct {
	Cursor      ActivityCursor
	Reset       bool
	Activity    []contracts.ActivityEvent
	Diagnostics []string
}

type LogNormalizer interface {
	Name() string
	Detect([]byte) bool
	Normalize([]byte) []contracts.ActivityEvent
}

type ErrorKind string

var ErrRecordNotFound = errors.New("product record not found")

const (
	Unavailable        ErrorKind = "unavailable"
	MissingDatabase    ErrorKind = "missing_database"
	IncompatibleFormat ErrorKind = "incompatible_format"
	IdentityConflict   ErrorKind = "identity_conflict"
	UnknownRecord      ErrorKind = "unknown_record"
	ReadFailed         ErrorKind = "read_failed"
	Closed             ErrorKind = "closed"
)

type Error struct {
	Kind  ErrorKind
	Cause error
}

func (e *Error) Error() string { return e.Cause.Error() }
func (e *Error) Unwrap() error { return e.Cause }

type ControlAction string

const (
	OpenTerminal       ControlAction = "open_terminal"
	RecheckEnvironment ControlAction = "recheck_environment"
	StartTask          ControlAction = "start_task"
	StartCandidate     ControlAction = "start_candidate"
	AbandonCandidate   ControlAction = "abandon_candidate"
	SetDispatchMode    ControlAction = "set_dispatch_mode"
	PauseDispatch      ControlAction = "pause_dispatch"
	ResumeDispatch     ControlAction = "resume_dispatch"
	StopService        ControlAction = "stop_service"
)

type ControlInput struct {
	Action                             ControlAction
	Payload                            ControlPayload
	RequestID, RequestedBy, InstanceID string
	ExpectedRevision                   *uint64
}

// Receipt status remains the underlying accepted/applied/rejected/failed/
// indeterminate state. Acceptance is never inferred to mean completion.
type ControlReceipt = contracts.ControlReceipt
type DecisionInput struct {
	Answer                 contracts.DecisionAnswer
	RequestedBy, CommandID string
}
type AnswerInput struct {
	RunID, DecisionID, Option, Note, RequestedBy, CommandID string
	ExpectedVersion                                         uint64
}
type CreateInput struct {
	Payload                  []byte
	RequestID, RequestedBy   string
	ExpectedAuthorityVersion uint64
}
type DiscardInput struct {
	TaskID, RequestID, Reason, RequestedBy string
	ExpectedVersion                        uint64
}
type CapabilityFilter struct {
	Backend                                                  execution.BackendKind
	Protocol                                                 execution.ExecutionProtocol
	TargetID, ExecutionID, AttemptID, Source, SourceIdentity string
}

// Reader exposes the existing product projections; its implementation uses
// query services, with no new paging or projection rules in the client.
type Reader interface {
	Close() error
	ReadProjectView(context.Context, string, int) (contracts.ProjectView, error)
	ActivityEvents(context.Context, string, string, int64) ([]contracts.DomainEvent, error)
	Domain(context.Context) (contracts.DomainSnapshot, error)
	DatabaseVersion(context.Context) (string, uint64, error)
	ShowNativeTask(context.Context, string) (contracts.NativeTaskPage, error)
	ListNativeTasks(context.Context, string, int) (contracts.NativeTaskPage, error)
	TaskGraph(context.Context, string, int) (taskview.Page, error)
	QueryTaskGraph(context.Context, taskview.Filter) (taskview.Page, error)
	ReadConversation(context.Context, contracts.ConversationFilter, string, int) (contracts.ConversationPage, error)
	ReadMessageBody(context.Context, contracts.ConversationBodyRef, int64, int) (contracts.ConversationBodyPage, error)
	ReadRaw(context.Context, contracts.ConversationRawRef, int64, int) (contracts.ConversationRawPage, error)
	ReadCapabilities(context.Context, execution.BackendKind, execution.ExecutionProtocol, string, int) ([]contracts.ConversationCapabilityReport, error)
	Command(context.Context, string) (contracts.CommandRecord, error)
}
type TaskCommands interface {
	Close() error
	Validate(context.Context, []byte) (task.TaskBatchValidation, error)
	Create(context.Context, CreateInput) (task.TaskBatchReceipt, error)
	Discard(context.Context, DiscardInput) (task.TaskDiscardReceipt, error)
	AddPredecessors(context.Context, task.TaskPredecessorAddInput) (task.TaskPredecessorAddReceipt, error)
}
type ControlPort interface {
	Submit(context.Context, ControlInput) (ControlRequest, error)
	Receipt(context.Context, string) (*ControlReceipt, error)
	Request(context.Context, string) (*ControlRequest, error)
	Stop(context.Context, string, time.Duration) (ServiceStopResult, error)
}
type ServiceStopResult struct {
	Status        string          `json:"status"`
	RequestID     string          `json:"request_id"`
	InstanceID    string          `json:"instance_id"`
	Receipt       *ControlReceipt `json:"receipt,omitempty"`
	ProcessExited bool            `json:"process_exited"`
	LocksReleased bool            `json:"locks_released"`
	Diagnostic    string          `json:"diagnostic,omitempty"`
}
type DecisionPort interface {
	Eligibility(context.Context, *contracts.Run) error
	Submit(context.Context, DecisionInput) (contracts.CommandRecord, bool, error)
	Answer(context.Context, AnswerInput) (contracts.CommandRecord, bool, error)
}

// Resources is supplied by application composition. Factories own concrete
// configuration/SQL construction; each returned reader or command resource is
// closed by the client in the same operation. Close only releases client-owned
// resources and must never stop a service or worker.
type Resources struct {
	Project                 ProjectRef
	ExecutableTypes         []string
	DefaultRequestedBy      string
	DefaultAnswerer         string
	CandidateFreshnessLimit time.Duration
	OpenReader              func(context.Context) (Reader, error)
	OpenCommands            func(context.Context, bool) (TaskCommands, error)
	Connect                 func(context.Context, *ObservationSnapshot) ServiceConnection
	Observe                 func(*contracts.Runtime, error, time.Time) ObservationSnapshot
	ProbeProcess            func(*execution.ProcessIdentity) ProcessProbe
	Control                 ControlPort
	Decision                DecisionPort
	TaskSnapshot            func(context.Context) (*contracts.TaskSnapshot, error)
	ResolveStartTask        func(context.Context, string, string) (ControlPayload, error)
	InspectRecovery         func(context.Context, string) (RecoveryInspection, error)
	DecisionAvailability    func(context.Context, *contracts.Run) string
	Close                   func() error
}
type Factory func(context.Context, ProjectRef) (Resources, error)

// Consumer interfaces expose only the capabilities each product view uses.
type Lifecycle interface {
	Project() ProjectRef
	Close() error
}
type TaskClient interface {
	ShowTask(context.Context, string) (contracts.NativeTaskPage, error)
	ListTasks(context.Context, string, int) (contracts.NativeTaskPage, error)
	TaskGraph(context.Context, string, int) (taskview.Page, error)
	QueryTaskGraph(context.Context, taskview.Filter) (taskview.Page, error)
	ValidateTaskBatch(context.Context, []byte) (task.TaskBatchValidation, error)
	CreateTaskBatch(context.Context, CreateInput) (task.TaskBatchReceipt, error)
	DiscardTask(context.Context, DiscardInput) (task.TaskDiscardReceipt, error)
	AddPredecessors(context.Context, task.TaskPredecessorAddInput) (task.TaskPredecessorAddReceipt, error)
}
type ProjectQueries interface {
	ReadProject(context.Context, string, ProjectSnapshot) ProjectSnapshot
	TaskSnapshot(context.Context) (*contracts.TaskSnapshot, error)
	Activity(context.Context, string, string, string, ActivityCursor) (LogBatch, error)
}
type ConversationQueries interface {
	Conversation(context.Context, contracts.ConversationFilter, string) (contracts.ConversationPage, error)
	Body(context.Context, contracts.ConversationBodyRef, int64, int) (contracts.ConversationBodyPage, error)
	Raw(context.Context, contracts.ConversationRawRef, int64, int) (contracts.ConversationRawPage, error)
	CursorAfter(contracts.ConversationFilter, int64) (string, error)
	Capabilities(context.Context, CapabilityFilter) ([]contracts.ConversationCapabilityReport, error)
}
type Controls interface {
	SubmitControl(context.Context, ControlInput) (ControlRequest, error)
	Receipt(context.Context, string) (*ControlReceipt, error)
	Request(context.Context, string) (*ControlRequest, error)
	WaitControl(context.Context, string, time.Duration) (*ControlReceipt, error)
	StopService(context.Context, string, time.Duration) (ServiceStopResult, error)
	StartTaskPayload(context.Context, string, string) (ControlPayload, error)
}
type Decisions interface {
	DecisionEligibility(context.Context, *contracts.Run) error
	SubmitDecision(context.Context, DecisionInput) (contracts.CommandRecord, bool, error)
	Answer(context.Context, AnswerInput) (contracts.CommandRecord, bool, error)
	Command(context.Context, string) (contracts.CommandRecord, error)
	WaitCommand(context.Context, string, time.Duration) (contracts.CommandRecord, error)
}
type RecoveryQueries interface {
	InspectRecovery(context.Context, string) (RecoveryInspection, error)
}

// InputChecks reports product input availability; center submission remains authoritative.
type InputChecks interface {
	CandidateAvailability(ProjectSnapshot, time.Time) (bool, string)
	StartTaskAvailability(ProjectSnapshot, time.Time) string
	AutoDispatchAvailability(ProjectSnapshot) string
}

// ConsoleClient groups the small, pure ports used by a single project view.
type ConsoleClient interface {
	InputChecks
	Lifecycle
	ProjectQueries
	ConversationQueries
	Controls
	Decisions
	RecoveryQueries
}

type Client interface {
	InputChecks
	RecoveryQueries
	Lifecycle
	TaskClient
	ProjectQueries
	ConversationQueries
	Controls
	Decisions
}

// ActivityCursor belongs to one canonical database stream.
type ActivityCursor struct {
	RunID, ExecutionID, AttemptID string
	AfterEventID                  int64
	Initialized                   bool
}
