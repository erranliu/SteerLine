package sqlite

import (
	"context"
	"database/sql"
	"time"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type ProjectRecords struct {
	View storageports.ProjectView
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
	view := storageports.ProjectView{SampledAt: time.Now().UTC()}
	page, err := s.NativeTaskPageTx(ctx, tx, project, "", "", 0, 0, limit)
	if err != nil {
		view.TaskError = err
	} else {
		page.SampledAt = view.SampledAt
		view.Tasks = &page
		graph, graphErr := s.TaskGraphSnapshotTx(ctx, tx, project)
		if graphErr != nil {
			view.TaskError = graphErr
		} else {
			graph.SampledAt = view.SampledAt
			ready := graph.ReadySnapshot()
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

	candidates, err := readRunnerCandidates(ctx, tx, project, "")
	view.HistoryError = err
	if err == nil {
		view.Candidates = candidates
	}
	return ProjectRecords{View: view}
}

// ReadProjectView samples every product section in one read transaction. It
// returns storage facts and never projects product DTOs or execution Runtime.
func (s Reader) ReadProjectView(ctx context.Context, project, selectedTaskID string, limit int) (storageports.ProjectView, error) {
	records, err := s.ReadProjectRecords(ctx, project, selectedTaskID, limit)
	if err != nil {
		return storageports.ProjectView{}, err
	}
	return records.View, nil
}

func (s Reader) ReadProjectViewTx(ctx context.Context, tx *sql.Tx, project, selectedTaskID string, limit int) storageports.ProjectView {
	return s.ReadProjectRecordsTx(ctx, tx, project, selectedTaskID, limit).View
}
