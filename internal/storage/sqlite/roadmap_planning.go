package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
)

// The explicit discard reader can inspect databases created before planning existed.
func (s *SQLiteStore) PlanningTasksForDiscard(ctx context.Context, project string) ([]roadmap.PlanningTask, error) {
	version, err := readDatabaseSchemaVersion(ctx, s.db)
	if err != nil || version < 48 {
		return nil, err
	}
	return s.PlanningTasks(ctx, project)
}

func (s *SQLiteStore) PlanningTasks(ctx context.Context, project string) ([]roadmap.PlanningTask, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT planning_id,revision,payload FROM roadmap_planning WHERE project_identity=? ORDER BY planning_id", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []roadmap.PlanningTask{}
	for rows.Next() {
		var raw []byte
		var id string
		var revision uint64
		var task roadmap.PlanningTask
		if err := rows.Scan(&id, &revision, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &task); err != nil {
			return nil, err
		}
		if task.Project != project || task.ID != id || task.Revision != revision {
			return nil, errors.New("planning identity/revision mismatch")
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *SQLiteStore) PlanningTask(ctx context.Context, project, id string) (roadmap.PlanningTask, error) {
	var raw []byte
	var revision uint64
	var task roadmap.PlanningTask
	err := s.db.QueryRowContext(ctx, "SELECT revision,payload FROM roadmap_planning WHERE project_identity=? AND planning_id=?", project, id).Scan(&revision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return task, roadmap.ErrNotFound
	}
	if err != nil {
		return task, err
	}
	err = json.Unmarshal(raw, &task)
	if err == nil && (task.Project != project || task.ID != id || task.Revision != revision) {
		err = errors.New("planning identity/revision mismatch")
	}
	return task, err
}

func (s *SQLiteStore) PlanningReceipt(ctx context.Context, project, id string) ([]byte, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, "SELECT receipt FROM roadmap_planning_requests WHERE project_identity=? AND request_id=?", project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return raw, err
}

// SavePlanning atomically accepts a command and all its task changes, or records
// one subsequent transition without changing the original HTTP receipt.
func (s *SQLiteStore) SavePlanning(ctx context.Context, project, request string, receipt []byte, tasks ...roadmap.PlanningTask) error {
	if s.readOnly || project == "" {
		return errors.New("invalid planning store write")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, task := range tasks {
		validPath := task.Kind == "" && roadmap.SourcePath(task.Path) || task.Kind == roadmap.SingleTaskPlanning && task.Path == roadmap.TaskCardPath(task.ID)
		if task.Project != project || !roadmap.IDPattern.MatchString(task.ID) || !validPath || task.Revision == 0 {
			return errors.New("invalid planning task identity")
		}
		raw, err := json.Marshal(task)
		if err != nil {
			return err
		}
		var occupied any = strings.ToLower(task.Path)
		if task.Run == nil && (task.Status == "failed" || task.Status == "closed") || task.Run != nil && task.Run.StopStatus == "stopped" && (task.SourceID != "" || task.Status == "closed") {
			occupied = nil
		}
		if task.Revision == 1 {
			_, err = tx.ExecContext(ctx, "INSERT INTO roadmap_planning(project_identity,planning_id,active_path,revision,payload) VALUES(?,?,?,?,?)", project, task.ID, occupied, task.Revision, raw)
		} else {
			var result sql.Result
			result, err = tx.ExecContext(ctx, "UPDATE roadmap_planning SET active_path=?,revision=?,payload=? WHERE project_identity=? AND planning_id=? AND revision=?", occupied, task.Revision, raw, project, task.ID, task.Revision-1)
			if err == nil {
				n, countErr := result.RowsAffected()
				if countErr != nil {
					return countErr
				}
				if n != 1 {
					return roadmap.ErrPlanningChanged
				}
			}
		}
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed: roadmap_planning.project_identity, roadmap_planning.active_path") {
				return roadmap.ErrPlanningPathOccupied
			}
			return err
		}
	}
	if request != "" {
		if _, err = tx.ExecContext(ctx, "INSERT INTO roadmap_planning_requests(project_identity,request_id,receipt) VALUES(?,?,?)", project, request, receipt); err != nil {
			return err
		}
	}
	return tx.Commit()
}
