package steerline

import (
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type recordWrite[T any] = storageports.RecordWrite[T]

type workerCommit = persistence.WorkerCommit

type workerCommitResult = persistence.WorkerCommitResult

type centerReader interface {
	persistence.CenterReader
	query.Store
}

type taskGraphSnapshot = storageports.TaskGraphSnapshot
