package sqlite

import (
	"context"
	"errors"
)

func (s *SQLiteStore) ReadProjectView(ctx context.Context, project, selectedTaskID string, limit int) (ProjectView, error) {
	if s == nil {
		return ProjectView{}, errors.New("project view reader is required")
	}
	return s.Reader.ReadProjectView(ctx, project, selectedTaskID, limit)
}
