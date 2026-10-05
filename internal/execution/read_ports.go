package execution

import (
	"context"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// StateReader is the execution actor's atomic startup/recovery read port.
type StateReader interface {
	storageports.ExecutionStateReader
	NativeAuthority(context.Context, string) (executionmodel.ProjectTaskAuthority, error)
	DatabaseVersion(context.Context) (string, uint64, error)
}

// ExecutionRecordReader provides only immutable bindings and operation
// receipts required to verify original execution identity.
type ExecutionRecordReader interface {
	ExecutionBinding(context.Context, string, string) (executionbackend.ExecutionRef, error)
	ExecutionOperation(context.Context, string, string, string) (executionbackend.Operation, error)
}

type NativeCommandReader interface {
	CommandByIdempotency(context.Context, string, string) (storageports.CommandRecord, error)
}
