package sqlite

import (
	"context"
	"errors"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (s *SQLiteStore) ReadProjectView(ctx context.Context, project, selectedTaskID string, limit int) (storageports.ProjectView, error) {
	if s == nil {
		return storageports.ProjectView{}, errors.New("project view reader is required")
	}
	return s.Reader.ReadProjectView(ctx, project, selectedTaskID, limit)
}
