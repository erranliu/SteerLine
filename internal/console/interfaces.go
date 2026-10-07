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

type ProjectQueries interface {
	ReadProject(context.Context, string, api.ProjectSnapshot) api.ProjectSnapshot
	RecentActivity(context.Context, string, string, string) (api.LogBatch, error)
}

type ConversationQueries interface {
	Conversation(context.Context, api.ConversationFilter, string) (api.ConversationPage, error)
	Body(context.Context, api.ConversationBodyRef, int64, int) (api.ConversationBodyPage, error)
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
