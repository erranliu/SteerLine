package steerline

import (
	"context"
	"database/sql"
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
)

type ProjectView = contracts.ProjectView

func (s *SQLiteStore) ReadProjectView(ctx context.Context, project, selectedTaskID string, limit int) (ProjectView, error) {
	if s == nil {
		return ProjectView{}, errors.New("project view reader is required")
	}
	records, err := s.reader().ReadProjectRecords(ctx, project, selectedTaskID, limit)
	if err != nil {
		return ProjectView{}, err
	}
	return projectReadRecords(records), nil
}
func (s *SQLiteStore) readProjectViewTx(ctx context.Context, tx *sql.Tx, project, selectedTaskID string, limit int) ProjectView {
	return projectReadRecords(s.reader().ReadProjectRecordsTx(ctx, tx, project, selectedTaskID, limit))
}
func projectReadRecords(records sqlite.ProjectRecords) ProjectView {
	view := records.View
	if metadata := records.Metadata; metadata != nil {
		hydrateRuntime(metadata, *view.Domain)
		if err := projectRuntime(metadata); err != nil {
			view.RuntimeError = err
		} else {
			view.Runtime = metadata
		}
	}
	return view
}
