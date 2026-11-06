package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

// Check all unfinished tasks in the reservation transaction, so another task's
// invalidation cannot race a new candidate into the project. Exact replays have
// already returned before this check and retain their original frozen input.
func (tx *sqliteTx) verifyProjectMaterials(project, commit string, tasks []taskmodel.TaskRecord) error {
	materials := []pm.Material{}
	for _, task := range tasks {
		if task.Status != "open" && task.Status != "in_progress" {
			continue
		}
		material := pm.Material{TaskID: task.TaskID, Status: "unprepared"}
		var raw []byte
		err := tx.tx.QueryRowContext(tx.ctx, `SELECT payload FROM project_materials WHERE project_identity=? AND task_id=?`, project, task.TaskID).Scan(&raw)
		if err == nil {
			if err = json.Unmarshal(raw, &material); err != nil {
				return err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if material.Status == "ready" {
			err = tx.tx.QueryRowContext(tx.ctx, `SELECT payload FROM test_generations WHERE project_identity=? AND generation_id=?`, project, material.GenerationID).Scan(&raw)
			var generation testmodel.Generation
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if err != nil || json.Unmarshal(raw, &generation) != nil || generation.State != "generated" || generation.Cancelled || generation.Inputs != material.Inputs {
				material.Status = "unknown"
			}
		}
		materials = append(materials, material)
	}
	readiness := pm.MaterialsReadiness(commit, materials)
	if readiness.Status != "ready" {
		return nativeErr("project_not_ready", "项目未就绪："+strings.Join(readiness.Blockers, "；"))
	}
	return nil
}
