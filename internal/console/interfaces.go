package console

import (
	"context"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"time"
)

// Consumer interfaces expose only the capabilities each product view uses.
type Lifecycle interface {
	Project() api.ProjectRef
	Close() error
}

type TaskClient interface {
	ShowTask(context.Context, string) (api.TaskPage, error)
	ListTasks(context.Context, string, int) (api.TaskPage, error)
	TaskGraph(context.Context, string, int) (api.GraphPage, error)
	QueryTaskGraph(context.Context, api.GraphFilter) (api.GraphPage, error)
	ValidateTaskBatch(context.Context, []byte) (api.TaskBatchValidation, error)
	CreateTaskBatch(context.Context, api.CreateInput) (api.TaskBatchReceipt, error)
	DiscardTask(context.Context, api.DiscardInput) (api.TaskDiscardReceipt, error)
	AddPredecessors(context.Context, api.TaskPredecessorAddInput) (api.TaskPredecessorAddReceipt, error)
}

type ProjectQueries interface {
	ReadProject(context.Context, string, api.ProjectSnapshot) api.ProjectSnapshot
	TaskSnapshot(context.Context) (*api.TaskCandidates, error)
	Activity(context.Context, string, string, string, api.ActivityCursor) (api.LogBatch, error)
}

type ConversationQueries interface {
	Conversation(context.Context, api.ConversationFilter, string) (api.ConversationPage, error)
	Body(context.Context, api.ConversationBodyRef, int64, int) (api.ConversationBodyPage, error)
	Raw(context.Context, api.ConversationRawRef, int64, int) (api.ConversationRawPage, error)
	CursorAfter(api.ConversationFilter, int64) (string, error)
	Capabilities(context.Context, api.CapabilityFilter) ([]api.ConversationCapabilityReport, error)
}

type Controls interface {
	SubmitControl(context.Context, api.ControlInput) (api.ControlRequest, error)
	Receipt(context.Context, string) (*api.ControlReceipt, error)
	Request(context.Context, string) (*api.ControlRequest, error)
	WaitControl(context.Context, string, time.Duration) (*api.ControlReceipt, error)
	StopService(context.Context, string, time.Duration) (api.ServiceStopResult, error)
	StartTaskPayload(context.Context, string, string) (api.ControlPayload, error)
}

type Decisions interface {
	DecisionEligibility(context.Context, *api.CandidateSummary) error
	SubmitDecision(context.Context, api.DecisionInput) (api.CommandReceipt, bool, error)
	Answer(context.Context, api.AnswerInput) (api.CommandReceipt, bool, error)
	Command(context.Context, string) (api.CommandReceipt, error)
	WaitCommand(context.Context, string, time.Duration) (api.CommandReceipt, error)
}

type RecoveryQueries interface {
	InspectRecovery(context.Context, string) (api.RecoveryInspection, error)
}

// InputChecks reports product input availability; center submission remains authoritative.
type InputChecks interface {
	CandidateAvailability(api.ProjectSnapshot, time.Time) (bool, string)
	StartTaskAvailability(api.ProjectSnapshot, time.Time) string
	AutoDispatchAvailability(api.ProjectSnapshot) string
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
