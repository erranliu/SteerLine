package execution

import (
	"context"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// These ports belong to the execution consumer. Task applications satisfy them
// structurally; execution never needs to import the task implementation.
type TaskWorkPort interface {
	VerifyContractFiles(context.Context, taskmodel.TaskContract) error
	Ready(context.Context, string) ([]taskmodel.TaskRecord, error)
	CanExecuteType(string) bool
	DefaultRequestedBy() string
	Fixed(context.Context, string, string, uint64, bool) (taskmodel.FixedWork, error)
	Claim(taskmodel.FixedWork, string, time.Time) (taskmodel.TaskRecord, error)
}

type TaskResultPort interface {
	Declare(taskmodel.TaskRecord, taskmodel.TaskContract, taskmodel.Result, time.Time) (taskmodel.ResultDeclaration, error)
	Intent(taskmodel.DiscardIntent) (taskmodel.DiscardIntent, error)
	Discard(taskmodel.TaskRecord, taskmodel.DiscardIntent, string, []string, time.Time) (taskmodel.TaskRecord, error)
}

// CommandStore is the execution command consumer's bounded storage surface.
// It exposes execution/task declaration commits, never a connection or factory.
type CommandStore interface {
	storageports.Commands
	storageports.Claims
	storageports.Results
	storageports.Discards
	storageports.TaskCandidateReader
	TaskGraphSnapshot(context.Context, string) (storageports.TaskGraphSnapshot, error)
	NativeAuthority(context.Context, string) (executionmodel.ProjectTaskAuthority, error)
	TaskContract(context.Context, string, taskmodel.TaskRecord) (taskmodel.TaskContract, error)
	Operation(context.Context, string, string) (executionmodel.OperationRecord, error)
	WorkflowOwned(context.Context, string, string, string) (bool, error)
	UpdateDiscardReceipt(context.Context, executionmodel.OperationRecord) error
	TransactCandidate(context.Context, func(storageports.CandidateTx) error) error
	TransactDispatch(context.Context, func(storageports.DispatchTx) error) error
	TransactDiscardIntent(context.Context, func(storageports.DiscardIntentTx) error) error
	TransactDiscardPending(context.Context, func(storageports.DiscardPendingTx) error) error
	TransactDeliveryAudit(context.Context, func(storageports.DeliveryAuditTx) error) error
	TransactCleanupProgress(context.Context, func(storageports.CleanupProgressTx) error) error
	TransactCleanup(context.Context, func(storageports.CleanupTx) error) error
}

type CommandService struct {
	Store       CommandStore
	Project     string
	Work        TaskWorkPort
	taskResults TaskResultPort
	runtime     RuntimeDependencies
}

// The app supplies the task work and result capabilities only to execution.
func NewCommandService(store CommandStore, project string, work TaskWorkPort, results TaskResultPort) CommandService {
	return CommandService{Store: store, Project: project, Work: work, taskResults: results}
}

// NewCommandServiceWithRuntime composes an app-owned profile verifier and
// implementation adapters with the bounded command ports.
func NewCommandServiceWithRuntime(store CommandStore, project string, work TaskWorkPort, results TaskResultPort, runtime RuntimeDependencies) CommandService {
	service := NewCommandService(store, project, work, results)
	service.runtime = runtime
	return service
}

func (s *Service) commandService(project string) CommandService {
	command := NewCommandService(s.commandStore, project, s.taskWork, s.taskResults)
	command.runtime = s.runtime
	return command
}

func (s CommandService) defaultRequestedBy() string {
	if s.Work == nil {
		return "unknown"
	}
	return s.Work.DefaultRequestedBy()
}
