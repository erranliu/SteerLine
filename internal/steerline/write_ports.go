package steerline

import (
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/query"
	"github.com/erranliu/SteerLine/internal/steerline/task"
)

type recordWrite[T any] = persistence.RecordWrite[T]
type workerCommit = persistence.WorkerCommit
type workerCommitResult = persistence.WorkerCommitResult
type centerReader interface {
	persistence.CenterReader
	query.Store
}

type taskGraphSnapshot = task.Snapshot
