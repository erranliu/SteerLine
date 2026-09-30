package query

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func (q Service) ReadProjectView(ctx context.Context, selectedTaskID string, limit int) (contracts.ProjectView, error) {
	if q.Projects == nil || q.Project == "" {
		return contracts.ProjectView{}, errors.New("query service store and project identity are required")
	}
	return q.Projects.ReadProjectView(ctx, q.Project, selectedTaskID, limit)
}
