package httpapi

import (
	"context"
	"sync"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

// ProjectQueryPort provides project and task snapshots used by project routes.
type ProjectQueryPort interface {
	Project() api.ProjectRef
	ReadProject(context.Context, string, api.ProjectSnapshot) api.ProjectSnapshot
	ListTasks(context.Context, string, int) (api.TaskPage, error)
	ShowTask(context.Context, string) (api.TaskPage, error)
	TaskGraph(context.Context, string, int) (api.GraphPage, error)
	TaskSnapshot(context.Context) (*api.TaskCandidates, error)
}

type ActivityQueryPort interface {
	RecentActivity(context.Context, string, string, string) (api.LogBatch, error)
	Activity(context.Context, string, string, string, api.ActivityCursor) (api.LogBatch, error)
}

type ConversationQueryPort interface {
	ConversationPage(context.Context, api.ConversationFilter, string, int) (api.ConversationPage, error)
	Body(context.Context, api.ConversationBodyRef, int64, int) (api.ConversationBodyPage, error)
}

type RecoveryQueryPort interface {
	InspectRecovery(context.Context, string) (api.RecoveryInspection, error)
	DecisionEligibility(context.Context, *api.CandidateSummary) error
}

// QueryPort is the HTTP server's read-only view of the product application.
// Its cohesive subports follow this transport's queries without depending on
// the product client's wider command and lifecycle surface.
type QueryPort interface {
	ProjectQueryPort
	ActivityQueryPort
	ConversationQueryPort
	RecoveryQueryPort
}

// CommandPort contains application operations used to prepare and submit the
// fixed command mapped from an HTTP envelope.
type CommandPort interface {
	DiscardTask(context.Context, api.DiscardInput) (api.TaskDiscardReceipt, error)
	NormalizeDecision(context.Context, api.DecisionInput) (api.DecisionInput, error)
	SubmitDecision(context.Context, api.DecisionInput) (api.CommandReceipt, bool, error)
	StartTaskPayload(context.Context, string, string) (api.ControlPayload, error)
	SubmitControlInbox(context.Context, api.ControlInput) (api.ControlRequest, error)
}

// AuthorityPort reads and submits durable command/control facts for request
// reconciliation. It does not expose arbitrary storage or database access.
type AuthorityPort interface {
	Command(context.Context, string) (api.CommandReceipt, error)
	Request(context.Context, string) (*api.ControlRequest, error)
	Receipt(context.Context, string) (*api.ControlReceipt, error)
	WaitControl(context.Context, string, time.Duration) (*api.ControlReceipt, error)
}

// NativeSessionPort owns the one native viewer side effect behind HTTP.
type NativeSessionPort interface {
	OpenNativeSession(context.Context, api.NativeSessionTarget) error
}

// Ports are the capabilities consumed by the HTTP server. A product client
// can satisfy them directly; no forwarding client is required.
type Ports struct {
	Query        QueryPort
	Commands     CommandPort
	Authority    AuthorityPort
	NativeViewer NativeSessionPort
}

type readinessState struct {
	mu         sync.RWMutex
	ready      bool
	status     string
	diagnostic string
}

func (r *readinessState) Ready() bool {
	ready, _, _ := r.Snapshot()
	return ready
}

func (r *readinessState) Snapshot() (bool, string, string) {
	if r == nil {
		return false, "", ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ready, r.diagnostic, r.status
}

func (r *readinessState) Set(ready bool, diagnostic, status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ready, r.diagnostic, r.status = ready, diagnostic, status
}
