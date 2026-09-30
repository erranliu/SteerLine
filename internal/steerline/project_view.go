package steerline

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ProjectView samples task details, readiness, execution records and service
// metadata in one read transaction. Section errors keep usable read-only data
// available without presenting an incomplete refresh as a reusable snapshot.
// Process liveness is observed separately; Ready is never a claim permit.
type ProjectView struct {
	SampledAt         time.Time
	Tasks             *NativeTaskPage
	Ready             *NativeReadyTaskSnapshot
	SelectedTask      *NativeTaskDetail
	Domain            *DomainSnapshot
	Runtime           *Runtime
	TaskError         error
	SelectedTaskError error
	DomainError       error
	RuntimeError      error
}

func (q QueryService) ReadProjectView(ctx context.Context, selectedTaskID string, limit int) (ProjectView, error) {
	store, ok := q.Store.(*SQLiteStore)
	if !ok || store == nil || q.Project == "" {
		return ProjectView{}, errors.New("project view requires the center SQLite store and project identity")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ProjectView{}, err
	}
	defer tx.Rollback()
	view := store.readProjectViewTx(ctx, tx, q.Project, selectedTaskID, limit)
	if err := tx.Commit(); err != nil {
		return ProjectView{}, err
	}
	return view, nil
}

func (s *SQLiteStore) readProjectViewTx(ctx context.Context, tx *sql.Tx, project, selectedTaskID string, limit int) ProjectView {
	view := ProjectView{SampledAt: time.Now().UTC()}
	page, err := s.nativeTaskPageTx(ctx, tx, project, "", "", 0, 0, limit)
	if err != nil {
		view.TaskError = err
	} else {
		page.SampledAt = view.SampledAt
		setNativeTaskPageCursor(&page, 0)
		view.Tasks = &page
		graph, graphErr := s.taskGraphSnapshotReadTx(ctx, tx, project)
		if graphErr != nil {
			view.TaskError = graphErr
		} else {
			graph.SampledAt = view.SampledAt
			ready := nativeReadyTaskSnapshot(graph)
			view.Ready = &ready
		}
		if selectedTaskID != "" {
			view.SelectedTaskError = ValidateNativeTaskID(selectedTaskID)
			if view.SelectedTaskError == nil {
				for i := range page.Tasks {
					if page.Tasks[i].TaskID == selectedTaskID {
						view.SelectedTask = &page.Tasks[i]
						break
					}
				}
				if view.SelectedTask == nil {
					selected, selectedErr := s.nativeTaskPageTx(ctx, tx, project, selectedTaskID, "", 0, 0, 1)
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
	domain, err := querySnapshot(ctx, tx, project)
	view.DomainError = err
	if err == nil {
		view.Domain = &domain
	}
	runtime, err := readRuntimeCheckpointTx(ctx, tx, project, true)
	if errors.Is(err, sql.ErrNoRows) {
		return view // Native task projects can exist before their first service run.
	}
	if err == nil {
		err = view.DomainError
	}
	if err == nil {
		hydrateRuntime(runtime, domain)
		err = projectRuntime(runtime)
	}
	view.RuntimeError = err
	if err == nil {
		view.Runtime = runtime
	}
	return view
}
