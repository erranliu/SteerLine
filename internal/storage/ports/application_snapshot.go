package ports

import "context"

// ApplicationProjectSnapshot keeps task/candidate facts and execution observation
// on the same database revision without exposing Runtime or Domain to product
// query/API consumers.
type ApplicationProjectSnapshot struct {
	ProjectView ProjectView
	RunnerState RunnerSnapshot
	RunnerError error
}

type ApplicationProjectSnapshotReader interface {
	ReadApplicationProjectSnapshot(context.Context, string, string, int) (ApplicationProjectSnapshot, error)
}
