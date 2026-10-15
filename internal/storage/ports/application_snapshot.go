package ports

import "context"

// ApplicationProjectSnapshot keeps the product page and execution observation
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
