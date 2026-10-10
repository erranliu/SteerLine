package host

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/testkit/fixtures"
	"time"

	execution "github.com/erranliu/SteerLine/internal/execution"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type taskCommandStorage = fixtures.TaskStorage
type taskCommandStore = fixtures.TaskStore

// NewTaskCommands extracts only fixed task input and injects narrow I/O ports.
func NewTaskCommands(store taskCommandStorage, config execution.Config) taskapp.Commands {
	timeout, _ := time.ParseDuration(config.CommandTimeout)
	dependencies := hostFixtureRuntime(config)
	return taskapp.Commands{Store: taskCommandStore{Store: store}, Documents: taskDocumentsForPorts(func(ctx context.Context) (string, error) {
		return config.TaskRepositoryTarget(ctx, dependencies.ProcessHost)
	}, config.Repository, config.Git, timeout, dependencies.ProcessHost), Input: taskapp.Input{Project: config.Identity(), Authority: "native", Remote: config.Remote, Branch: config.Branch, ExecutableTypes: []string{"task", "bug", "feature", "chore"}, DefaultRequestedBy: DefaultRequestedBy()}}
}

func taskCommandStorageForTest(store execution.CommandStore) taskCommandStorage {
	value, ok := store.(taskCommandStorage)
	if !ok {
		panic("test service command store does not expose the task command fixture ports")
	}
	return value
}

func (c taskTestCommands) ValidateTaskBatch(ctx context.Context, config execution.Config, data []byte) (taskmodel.TaskBatchValidation, error) {
	commands := NewTaskCommands(taskCommandStorageForTest(c.Store), config)
	return commands.ValidateTaskBatch(ctx, data)
}
func (c taskTestCommands) CreateTaskBatch(ctx context.Context, config execution.Config, data []byte, request string, version uint64, by string) (taskmodel.TaskBatchReceipt, error) {
	if config.Repository == "" || config.Remote == "" || config.Branch == "" {
		return taskmodel.TaskBatchReceipt{}, errors.New("repository, remote and branch are required")
	}
	commands := NewTaskCommands(taskCommandStorageForTest(c.Store), config)
	return commands.CreateTaskBatch(ctx, data, request, version, by)
}
func (c taskTestCommands) AddNativeTaskPredecessors(ctx context.Context, config execution.Config, in productapi.TaskPredecessorAddInput) (productapi.TaskPredecessorAddReceipt, error) {
	if config.Repository == "" || config.Remote == "" || config.Branch == "" {
		return productapi.TaskPredecessorAddReceipt{}, errors.New("repository, remote and branch are required")
	}
	commands := NewTaskCommands(taskCommandStorageForTest(c.Store), config)
	receipt, err := commands.AddNativeTaskPredecessors(ctx, taskmodel.PredecessorAddInput(in))
	return productapi.TaskPredecessorAddReceipt(receipt), err
}
func validateTaskBatchAgainstGraph(ctx context.Context, config execution.Config, batch taskmodel.TaskBatchInput, graph storageports.TaskGraphSnapshot, verify bool) ([]taskmodel.TaskBatchIssue, []string) {
	return taskmodel.ValidateBatchAgainstGraph(batch, graph.TaskGraph(), []string{"task", "bug", "feature", "chore"}, taskmodel.ValidateContract)
}
func validateNativePredecessorAddition(graph storageports.TaskGraphSnapshot, id string, predecessors []string) error {
	return taskmodel.ValidatePredecessorAddition(graph.TaskGraph(), id, predecessors)
}
