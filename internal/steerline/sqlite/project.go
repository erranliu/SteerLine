package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	queryrules "github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type ProjectRecords struct {
	View     contracts.ProjectView
	Metadata *contracts.Runtime
}

func (s Reader) ReadProjectRecords(ctx context.Context, project, selectedTaskID string, limit int) (ProjectRecords, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ProjectRecords{}, err
	}
	defer tx.Rollback()
	records := s.ReadProjectRecordsTx(ctx, tx, project, selectedTaskID, limit)
	if err := tx.Commit(); err != nil {
		return ProjectRecords{}, err
	}
	return records, nil
}

func (s Reader) ReadProjectRecordsTx(ctx context.Context, tx *sql.Tx, project, selectedTaskID string, limit int) ProjectRecords {
	view := contracts.ProjectView{SampledAt: time.Now().UTC()}
	page, err := s.NativeTaskPageTx(ctx, tx, project, "", "", 0, 0, limit)
	if err != nil {
		view.TaskError = err
	} else {
		page.SampledAt = view.SampledAt
		queryrules.SetTaskPageCursor(&page, 0)
		view.Tasks = &page
		graph, graphErr := s.TaskGraphSnapshotTx(ctx, tx, project)
		if graphErr != nil {
			view.TaskError = graphErr
		} else {
			graph.SampledAt = view.SampledAt
			ready := queryrules.ReadySnapshot(graph)
			view.Ready = &ready
		}
		if selectedTaskID != "" {
			view.SelectedTaskError = taskmodel.ValidateID(selectedTaskID)
			if view.SelectedTaskError == nil {
				for i := range page.Tasks {
					if page.Tasks[i].TaskID == selectedTaskID {
						view.SelectedTask = &page.Tasks[i]
						break
					}
				}
				if view.SelectedTask == nil {
					selected, selectedErr := s.NativeTaskPageTx(ctx, tx, project, selectedTaskID, "", 0, 0, 1)
					view.SelectedTaskError = selectedErr
					if selectedErr == nil {
						if len(selected.Tasks) == 1 {
							view.SelectedTask = &selected.Tasks[0]
						} else {
							view.SelectedTaskError = nativeErr("task_not_found", "native task was not found")
						}
					}
				}
			}
		}
	}
	domain, err := QuerySnapshot(ctx, tx, project)
	view.DomainError = err
	if err == nil {
		view.Domain = &domain
	}
	runtime, err := ReadRuntimeCheckpointTx(ctx, tx, project, true)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectRecords{View: view} // A project can precede its first service run.
	}
	if err == nil {
		err = view.DomainError
	}

	view.RuntimeError = err
	if err == nil {
		return ProjectRecords{View: view, Metadata: runtime}
	}
	return ProjectRecords{View: view}
}

// ReadProjectView samples all records in one transaction, then applies the shared pure projector.
func (s Reader) ReadProjectView(ctx context.Context, project, selectedTaskID string, limit int) (contracts.ProjectView, error) {
	records, err := s.ReadProjectRecords(ctx, project, selectedTaskID, limit)
	if err != nil {
		return contracts.ProjectView{}, err
	}
	return projectView(records), nil
}

func (s Reader) ReadProjectViewTx(ctx context.Context, tx *sql.Tx, project, selectedTaskID string, limit int) contracts.ProjectView {
	return projectView(s.ReadProjectRecordsTx(ctx, tx, project, selectedTaskID, limit))
}

func projectView(records ProjectRecords) contracts.ProjectView {
	view := records.View
	if metadata := records.Metadata; metadata != nil {
		projected, err := reconstruction.Build(*view.Domain, reconstruction.MetadataOf(*metadata))
		if err != nil {
			view.RuntimeError = err
		} else {
			view.Runtime = &projected
		}
	}
	return view
}
