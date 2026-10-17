package ports

import "context"

// Product query readers are read-only capabilities. Implementations own SQL
// and transactions; product/query owns cursor and projection rules.
// ActivityReader pins binding and Runner events in one read transaction. The
// consumer closes the stream after its projection budget or row limit is met.
type ActivityReader interface {
	OpenActivityView(context.Context, string, string, string, string, int64, int, bool) (ActivityBinding, ActivityRecords, error)
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
	EventReader
	TaskReader
	ProjectReader
	ActivityReader
}
