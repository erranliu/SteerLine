package execution

import (
	"context"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// These ports belong to the execution consumer. Task applications satisfy them
// structurally; execution never needs to import the task implementation.
type TaskWorkPort interface {
	VerifyContractFiles(context.Context, taskmodel.TaskContract) error
	Ready(context.Context, string) ([]taskmodel.TaskRecord, error)
	Fixed(context.Context, string, string, uint64, bool) (taskmodel.FixedWork, error)
	Claim(taskmodel.FixedWork, string, time.Time) (taskmodel.TaskRecord, error)
}

type TaskResultPort interface {
	Declare(taskmodel.TaskRecord, taskmodel.TaskContract, taskmodel.Result, time.Time) (taskmodel.ResultDeclaration, error)
	Intent(taskmodel.DiscardIntent) (taskmodel.DiscardIntent, error)
	Discard(taskmodel.TaskRecord, taskmodel.DiscardIntent, string, []string, time.Time) (taskmodel.TaskRecord, error)
}

type CommandService struct {
	Store       persistence.CommandStore
	Project     string
	Work        TaskWorkPort
	taskResults TaskResultPort
}

// The app supplies the task work and result capabilities only to execution.
func NewCommandService(store persistence.CommandStore, project string, work TaskWorkPort, results TaskResultPort) CommandService {
	return CommandService{Store: store, Project: project, Work: work, taskResults: results}
}

func (s *Service) commandService(project string) CommandService {
	return NewCommandService(s.commandStore, project, s.taskWork, s.taskResults)
}
