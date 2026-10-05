package execution

import (
	"context"

	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func newTestCommandService(store CommandStore, config Config) CommandService {
	tasks := taskapp.Service{Reader: taskCommandReader{CommandStore: store}, Input: taskInputForTest(config), Documents: taskDocumentsForTest(config)}
	return NewCommandService(store, config.Identity(), tasks, tasks)
}

type taskCommandReader struct{ CommandStore }

func (r taskCommandReader) TaskSnapshot(ctx context.Context, project string) (taskmodel.Snapshot, error) {
	graph, err := r.TaskGraphSnapshot(ctx, project)
	if err != nil {
		return taskmodel.Snapshot{}, err
	}
	return graph.TaskGraph(), nil
}
