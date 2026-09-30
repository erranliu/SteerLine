package steerline

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/taskview"
)

func (q QueryService) TaskGraph(ctx context.Context, cursor string, limit int) (TaskGraphPage, error) {
	if q.Store == nil || q.Project == "" {
		return TaskGraphPage{}, errors.New("query service store and project identity are required")
	}
	request, err := taskview.NormalizePage(q.Project, cursor, limit)
	if err != nil {
		return TaskGraphPage{}, err
	}
	snapshot, err := taskGraphFromStore(ctx, q.Store, q.Project)
	if err != nil {
		return TaskGraphPage{}, err
	}
	return taskview.Paginate(snapshot, request)
}
