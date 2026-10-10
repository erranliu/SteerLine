package execution

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/testkit/fixtures"
	"time"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type taskCommandStorage = fixtures.TaskStorage
type taskCommandStore = fixtures.TaskStore

// NewTaskCommands extracts only fixed task input and injects narrow I/O ports.

func NewTaskCommands(store taskCommandStorage, config Config) taskapp.Commands {
	return taskapp.Commands{Store: taskCommandStore{Store: store}, Documents: taskDocumentsForTest(config), Input: taskInputForTest(config)}
}

func taskDocumentsForTest(config Config) taskapp.Documents {
	dependencies, _ := neutralFixtureRuntime(config)
	timeout, _ := time.ParseDuration(config.CommandTimeout)
	return taskDocumentsForPorts(func(ctx context.Context) (string, error) {
		return configuredRepositoryTarget(ctx, config, dependencies.ProcessHost)
	}, config.Repository, config.Git, timeout, dependencies.ProcessHost)
}

func taskInputForTest(config Config) taskapp.Input {
	return taskapp.Input{Project: config.Identity(), Authority: "native", Remote: config.Remote, Branch: config.Branch, ExecutableTypes: []string{"task", "bug", "feature", "chore"}, DefaultRequestedBy: "operator"}
}

func taskCommandStorageForTest(store CommandStore) taskCommandStorage {
	value, ok := store.(taskCommandStorage)
	if !ok {
		panic("task command fixture requires the explicit task storage ports")
	}
	return value
}

func (c CommandService) ValidateTaskBatch(ctx context.Context, config Config, data []byte) (taskmodel.TaskBatchValidation, error) {
	if config.Repository == "" || config.Remote == "" || config.Branch == "" {
		return taskmodel.TaskBatchValidation{}, errors.New("repository, remote and branch are required")
	}
	commands := NewTaskCommands(taskCommandStorageForTest(c.Store), config)
	return commands.ValidateTaskBatch(ctx, data)
}
func (c CommandService) CreateTaskBatch(ctx context.Context, config Config, data []byte, request string, version uint64, by string) (taskmodel.TaskBatchReceipt, error) {
	if config.Repository == "" || config.Remote == "" || config.Branch == "" {
		return taskmodel.TaskBatchReceipt{}, errors.New("repository, remote and branch are required")
	}
	commands := NewTaskCommands(taskCommandStorageForTest(c.Store), config)
	return commands.CreateTaskBatch(ctx, data, request, version, by)
}
func (c CommandService) AddNativeTaskPredecessors(ctx context.Context, config Config, in productapi.TaskPredecessorAddInput) (productapi.TaskPredecessorAddReceipt, error) {
	if config.Repository == "" || config.Remote == "" || config.Branch == "" {
		return productapi.TaskPredecessorAddReceipt{}, errors.New("repository, remote and branch are required")
	}
	commands := NewTaskCommands(taskCommandStorageForTest(c.Store), config)
	receipt, err := commands.AddNativeTaskPredecessors(ctx, taskmodel.PredecessorAddInput(in))
	return productapi.TaskPredecessorAddReceipt(receipt), err
}
func validateTaskBatchAgainstGraph(ctx context.Context, config Config, batch taskmodel.TaskBatchInput, graph storageports.TaskGraphSnapshot, verify bool) ([]taskmodel.TaskBatchIssue, []string) {
	return taskmodel.ValidateBatchAgainstGraph(batch, graph.TaskGraph(), taskInputForTest(config).ExecutableTypes, taskmodel.ValidateContract)
}
func validateNativePredecessorAddition(graph storageports.TaskGraphSnapshot, id string, predecessors []string) error {
	return taskmodel.ValidatePredecessorAddition(graph.TaskGraph(), id, predecessors)
}
