package query

import (
	"context"
	"errors"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (q Service) ReadProjectView(ctx context.Context, selectedTaskID string, limit int) (storageports.ProjectView, error) {
	if q.Projects == nil || q.Project == "" {
		return storageports.ProjectView{}, errors.New("query service store and project identity are required")
	}
	view, err := q.Projects.ReadProjectView(ctx, q.Project, selectedTaskID, limit)
	if err == nil && view.Tasks != nil {
		view.Tasks.Protocol = NativeTaskProtocol
		SetTaskPageCursor(view.Tasks, 0)
	}
	return view, err
}
