package steerline

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/taskview"
)

// QueryTaskGraph reads all graph facts from one center SQLite snapshot.
func (q QueryService) QueryTaskGraph(ctx context.Context, filter TaskGraphFilter) (TaskGraphPage, error) {
	if q.Store == nil || q.Project == "" {
		return TaskGraphPage{}, errors.New("query service store and project identity are required")
	}
	filter, err := taskview.NormalizeFilter(q.Project, filter)
	if err != nil {
		return TaskGraphPage{}, err
	}
	snapshot, err := taskGraphFromStore(ctx, q.Store, q.Project)
	if err != nil {
		return TaskGraphPage{}, err
	}
	return taskview.Project(snapshot, filter)
}
