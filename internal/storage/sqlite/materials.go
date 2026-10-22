package sqlite

import (
	"context"
	"encoding/json"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
)

func (s *SQLiteStore) Material(ctx context.Context, project, task string) (pm.Material, error) {
	var m pm.Material
	var raw []byte
	e := s.db.QueryRowContext(ctx, `SELECT payload FROM project_materials WHERE project_identity=? AND task_id=?`, project, task).Scan(&raw)
	if e != nil {
		return m, persistenceError(e)
	}
	e = json.Unmarshal(raw, &m)
	return m, e
}
func (s *SQLiteStore) SaveMaterial(ctx context.Context, project string, m pm.Material) error {
	raw, e := json.Marshal(m)
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(ctx, `INSERT INTO project_materials VALUES(?,?,?) ON CONFLICT(project_identity,task_id) DO UPDATE SET payload=excluded.payload`, project, m.TaskID, raw)
	return e
}
func (s *SQLiteStore) MaterialProfile(ctx context.Context, project string) string {
	var profile string
	_ = s.db.QueryRowContext(ctx, `SELECT profile FROM maintenance_settings WHERE project_identity=?`, project).Scan(&profile)
	return profile
}
func (s *SQLiteStore) SetMaterialProfile(ctx context.Context, project, profile string) error {
	_, e := s.db.ExecContext(ctx, `INSERT INTO maintenance_settings(project_identity,profile) VALUES(?,?) ON CONFLICT(project_identity) DO UPDATE SET profile=excluded.profile`, project, profile)
	return e
}
func (s *SQLiteStore) SaveMaintenanceOperation(ctx context.Context, project, id string, raw []byte) error {
	_, e := s.db.ExecContext(ctx, `INSERT INTO maintenance_requests VALUES(?,?,?,?) ON CONFLICT(project_identity,request_id) DO UPDATE SET result=excluded.result`, project, id, id, raw)
	return e
}

func (s *SQLiteStore) MaterialObservation(ctx context.Context, project string) (pm.Observation, error) {
	var o pm.Observation
	var raw []byte
	e := s.db.QueryRowContext(ctx, `SELECT observation FROM maintenance_settings WHERE project_identity=?`, project).Scan(&raw)
	if e != nil {
		return o, persistenceError(e)
	}
	if len(raw) == 0 {
		return o, nil
	}
	e = json.Unmarshal(raw, &o)
	return o, e
}
func (s *SQLiteStore) SaveMaterialObservation(ctx context.Context, project string, o pm.Observation) error {
	raw, e := json.Marshal(o)
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(ctx, `INSERT INTO maintenance_settings(project_identity,profile,observation) VALUES(?,'',?) ON CONFLICT(project_identity) DO UPDATE SET observation=excluded.observation`, project, raw)
	return e
}

func (s *SQLiteStore) GenerationBatch(ctx context.Context, project string) (pm.GenerationBatch, error) {
	var batch pm.GenerationBatch
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT generation_batch FROM maintenance_settings WHERE project_identity=?`, project).Scan(&raw)
	if err != nil {
		return batch, persistenceError(err)
	}
	if len(raw) == 0 {
		return batch, nil
	}
	err = json.Unmarshal(raw, &batch)
	return batch, err
}

func (s *SQLiteStore) SaveGenerationBatch(ctx context.Context, project string, batch pm.GenerationBatch) error {
	raw, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO maintenance_settings(project_identity,profile,generation_batch) VALUES(?,'',?) ON CONFLICT(project_identity) DO UPDATE SET generation_batch=excluded.generation_batch`, project, raw)
	return err
}
