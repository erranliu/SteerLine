package clientcontracts

import (
	"context"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/taskview"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

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
	Cursor      productapi.ActivityCursor
	Reset       bool
	Activity    []contracts.ActivityEvent
	Diagnostics []string
}

type LogNormalizer interface {
	Name() string
	Detect([]byte) bool
	Normalize([]byte) []contracts.ActivityEvent
}

type DecisionInput struct {
	Answer                 executionmodel.DecisionAnswer
	RequestedBy, CommandID string
}

type CapabilityFilter struct {
	Backend                                                  executionbackend.BackendKind
	Protocol                                                 executionbackend.ExecutionProtocol
	TargetID, ExecutionID, AttemptID, Source, SourceIdentity string
}

// Reader exposes the existing product projections; its implementation uses
// query services, with no new paging or projection rules in the client.
type Reader interface {
	Close() error
	ReadProjectView(context.Context, string, int) (contracts.ProjectView, error)
	ActivityEvents(context.Context, string, string, int64) ([]storageports.DomainEvent, error)
	Domain(context.Context) (contracts.DomainSnapshot, error)
	DatabaseVersion(context.Context) (string, uint64, error)
	ShowNativeTask(context.Context, string) (contracts.NativeTaskPage, error)
	ListNativeTasks(context.Context, string, int) (contracts.NativeTaskPage, error)
	TaskGraph(context.Context, string, int) (taskview.Page, error)
	QueryTaskGraph(context.Context, taskview.Filter) (taskview.Page, error)
	ReadConversation(context.Context, contracts.ConversationFilter, string, int) (contracts.ConversationPage, error)
	ReadMessageBody(context.Context, contracts.ConversationBodyRef, int64, int) (contracts.ConversationBodyPage, error)
	ReadRaw(context.Context, contracts.ConversationRawRef, int64, int) (contracts.ConversationRawPage, error)
	ReadCapabilities(context.Context, executionbackend.BackendKind, executionbackend.ExecutionProtocol, string, int) ([]contracts.ConversationCapabilityReport, error)
	Command(context.Context, string) (storageports.CommandRecord, error)
}

type TaskCommands interface {
	Close() error
	Validate(context.Context, []byte) (taskmodel.TaskBatchValidation, error)
	Create(context.Context, productapi.CreateInput) (taskmodel.TaskBatchReceipt, error)
	Discard(context.Context, productapi.DiscardInput) (taskmodel.TaskDiscardReceipt, error)
	AddPredecessors(context.Context, productapi.TaskPredecessorAddInput) (productapi.TaskPredecessorAddReceipt, error)
}

type ControlPort interface {
	Submit(context.Context, productapi.ControlInput) (productapi.ControlRequest, error)
	Receipt(context.Context, string) (*productapi.ControlReceipt, error)
	Request(context.Context, string) (*productapi.ControlRequest, error)
	Stop(context.Context, string, time.Duration) (productapi.ServiceStopResult, error)
}

type DecisionPort interface {
	Eligibility(context.Context, *contracts.Run) error
	Submit(context.Context, DecisionInput) (storageports.CommandRecord, bool, error)
	Answer(context.Context, productapi.AnswerInput) (storageports.CommandRecord, bool, error)
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
	ProbeProcess            func(*executionbackend.ProcessIdentity) productapi.ProcessProbe
	Control                 ControlPort
	Decision                DecisionPort
	TaskSnapshot            func(context.Context) (*contracts.TaskSnapshot, error)
	ResolveStartTask        func(context.Context, string, string) (productapi.ControlPayload, error)
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
	ValidateTaskBatch(context.Context, []byte) (taskmodel.TaskBatchValidation, error)
	CreateTaskBatch(context.Context, productapi.CreateInput) (taskmodel.TaskBatchReceipt, error)
	DiscardTask(context.Context, productapi.DiscardInput) (taskmodel.TaskDiscardReceipt, error)
	AddPredecessors(context.Context, productapi.TaskPredecessorAddInput) (productapi.TaskPredecessorAddReceipt, error)
}

type ProjectQueries interface {
	ReadProject(context.Context, string, ProjectSnapshot) ProjectSnapshot
	TaskSnapshot(context.Context) (*contracts.TaskSnapshot, error)
	Activity(context.Context, string, string, string, productapi.ActivityCursor) (LogBatch, error)
}

type ConversationQueries interface {
	Conversation(context.Context, contracts.ConversationFilter, string) (contracts.ConversationPage, error)
	Body(context.Context, contracts.ConversationBodyRef, int64, int) (contracts.ConversationBodyPage, error)
	Raw(context.Context, contracts.ConversationRawRef, int64, int) (contracts.ConversationRawPage, error)
	CursorAfter(contracts.ConversationFilter, int64) (string, error)
	Capabilities(context.Context, CapabilityFilter) ([]contracts.ConversationCapabilityReport, error)
}

type Controls interface {
	SubmitControl(context.Context, productapi.ControlInput) (productapi.ControlRequest, error)
	Receipt(context.Context, string) (*productapi.ControlReceipt, error)
	Request(context.Context, string) (*productapi.ControlRequest, error)
	WaitControl(context.Context, string, time.Duration) (*productapi.ControlReceipt, error)
	StopService(context.Context, string, time.Duration) (productapi.ServiceStopResult, error)
	StartTaskPayload(context.Context, string, string) (productapi.ControlPayload, error)
}

type Decisions interface {
	DecisionEligibility(context.Context, *contracts.Run) error
	SubmitDecision(context.Context, DecisionInput) (storageports.CommandRecord, bool, error)
	Answer(context.Context, productapi.AnswerInput) (storageports.CommandRecord, bool, error)
	Command(context.Context, string) (storageports.CommandRecord, error)
	WaitCommand(context.Context, string, time.Duration) (storageports.CommandRecord, error)
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
