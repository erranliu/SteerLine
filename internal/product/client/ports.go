// Package client owns the single Go product client. Application composition
// injects each read, command and connection capability separately.
package client

import (
	"context"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

// Reader returns only serializable pages. Its application adapter binds one
// database and delegates transaction and cursor semantics to product/query.
type Reader interface {
	Close() error
	ReadProject(context.Context, string, api.ProjectSnapshot) api.ProjectSnapshot
	RecentActivity(context.Context, string, string, string) (api.LogBatch, error)
	Activity(context.Context, string, string, string, api.ActivityCursor) (api.LogBatch, error)
	ShowTask(context.Context, string) (api.TaskPage, error)
	ListTasks(context.Context, string, int) (api.TaskPage, error)
	TaskGraph(context.Context, string, int) (api.GraphPage, error)
	Conversation(context.Context, api.ConversationFilter, string, int) (api.ConversationPage, error)
	Body(context.Context, api.ConversationBodyRef, int64, int) (api.ConversationBodyPage, error)
	Command(context.Context, string) (api.CommandReceipt, error)
}

type TaskCommands interface {
	Close() error
	Validate(context.Context, []byte) (api.TaskBatchValidation, error)
	Create(context.Context, api.CreateInput) (api.TaskBatchReceipt, error)
	AddPredecessors(context.Context, api.TaskPredecessorAddInput) (api.TaskPredecessorAddReceipt, error)
}

type ControlPort interface {
	Submit(context.Context, api.ControlInput) (api.ControlRequest, error)
	Receipt(context.Context, string) (*api.ControlReceipt, error)
	Request(context.Context, string) (*api.ControlRequest, error)
	Stop(context.Context, string, time.Duration) (api.ServiceStopResult, error)
}

// ControlInboxPort publishes an immutable request bound to the client's
// displayed instance/revision. The running service rechecks qualification.
type ControlInboxPort interface {
	SubmitInbox(context.Context, api.ControlInput) (api.ControlRequest, error)
}

// TaskDiscardPort lets the configured service consume a discard through its
// existing command owner while it holds the repository lock.
type TaskDiscardPort interface {
	Discard(context.Context, api.DiscardInput) (api.TaskDiscardReceipt, error)
}

type DecisionPort interface {
	Normalize(context.Context, api.DecisionInput) (api.DecisionInput, error)
	Eligibility(context.Context, *api.CandidateSummary) error
	Submit(context.Context, api.DecisionInput) (api.CommandReceipt, bool, error)
	Answer(context.Context, api.AnswerInput) (api.CommandReceipt, bool, error)
}

// Resources is private Go composition, distinct from the serializable API.
// Close releases client-owned resources; it must not stop a worker or service.
type Resources struct {
	Tests                               api.TestManagementPort
	EnvironmentChecks                   api.EnvironmentCheckPort
	Project                             api.ProjectRef
	DefaultRequestedBy, DefaultAnswerer string
	OpenReader                          func(context.Context) (Reader, error)
	ReadFailure                         func(context.Context, string, error) api.ProjectSnapshot
	OpenCommands                        func(context.Context) (TaskCommands, error)
	Discard                             TaskDiscardPort
	Control                             ControlPort
	Decision                            DecisionPort
	TaskSnapshot                        func(context.Context) (*api.TaskCandidates, error)
	ResolveStartTask                    func(context.Context, string, string) (api.ControlPayload, error)
	InspectRecovery                     func(context.Context, string) (api.RecoveryInspection, error)
	OpenNativeSession                   func(context.Context, api.NativeSessionTarget) error
	Close                               func() error
}

type Factory func(context.Context, api.ProjectRef) (Resources, error)
