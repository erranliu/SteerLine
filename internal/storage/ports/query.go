package ports

import "context"

// Product query readers are read-only capabilities. Implementations own SQL
// and transactions; product/query owns cursor and projection rules.
type WorkflowReader interface {
	ReadWorkflow(context.Context, string, string) (WorkflowSnapshot, error)
}

// ActivityReader samples identity records and bounded events in one read transaction.
type ActivityReader interface {
	ReadActivityView(context.Context, string, string, string, string, int64, int, int) (ActivityBinding, []DomainEvent, error)
	ReadRecentActivityView(context.Context, string, string, string, string) (ActivityBinding, []DomainEvent, error)
}

// TaskCandidateReader returns the whole task graph and every candidate fact
// for one task from a single snapshot. It does not expose unrelated execution
// records such as decisions and private operation requests.
type TaskCandidateReader interface {
	ReadTaskCandidateSnapshot(context.Context, string, string) (TaskCandidateSnapshot, error)
}

type EventReader interface {
	Events(context.Context, string, string, string, int64, int, int) ([]DomainEvent, error)
	EventPayload(context.Context, string, int64, int64, int) ([]byte, int64, string, error)
}

type TaskReader interface {
	NativeTaskPage(context.Context, string, string, string, uint64, int, int) (NativeTaskPage, error)
	TaskGraphSnapshot(context.Context, string) (TaskGraphSnapshot, error)
}

type ProjectReader interface {
	ReadProjectView(context.Context, string, string, int) (ProjectView, error)
}

type ProductReader interface {
	WorkflowReader
	EventReader
	TaskReader
	ProjectReader
	ActivityReader
}
