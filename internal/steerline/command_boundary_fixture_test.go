package steerline

import (
	"context"

	"github.com/erranliu/SteerLine/internal/execution"
	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type taskTestCommands struct{ execution.CommandService }

func newTestCommandService(store execution.CommandStore, project string) taskTestCommands {
	tasks := taskapp.Service{Reader: testTaskReader{CommandStore: store}}
	return taskTestCommands{execution.NewCommandService(store, project, tasks, tasks)}
}

type testTaskReader struct{ execution.CommandStore }

func (r testTaskReader) TaskSnapshot(ctx context.Context, project string) (taskmodel.Snapshot, error) {
	graph, err := r.TaskGraphSnapshot(ctx, project)
	if err != nil {
		return taskmodel.Snapshot{}, err
	}
	return graph.TaskGraph(), nil
}
