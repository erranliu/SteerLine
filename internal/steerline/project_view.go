package steerline

import (
	"context"
	"database/sql"
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

type ProjectView = contracts.ProjectView

func (s *SQLiteStore) ReadProjectView(ctx context.Context, project, selectedTaskID string, limit int) (ProjectView, error) {
	if s == nil {
		return ProjectView{}, errors.New("project view reader is required")
	}
	return s.reader().ReadProjectView(ctx, project, selectedTaskID, limit)
}
func (s *SQLiteStore) readProjectViewTx(ctx context.Context, tx *sql.Tx, project, selectedTaskID string, limit int) ProjectView {
	return s.reader().ReadProjectViewTx(ctx, tx, project, selectedTaskID, limit)
}
