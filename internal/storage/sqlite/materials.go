package sqlite

import (
	"context"
	"database/sql"
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
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `INSERT INTO project_materials VALUES(?,?,?) ON CONFLICT(project_identity,task_id) DO UPDATE SET payload=excluded.payload`, project, m.TaskID, raw); e != nil {
		return e
	}
	if e = bindPreparationGenerations(ctx, tx, project); e != nil {
		return e
	}
	return tx.Commit()
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
	err := s.db.QueryRowContext(ctx, `SELECT b.payload FROM maintenance_settings s LEFT JOIN preparation_batches b ON b.project_identity=s.project_identity AND b.batch_id=s.generation_batch WHERE s.project_identity=?`, project).Scan(&raw)
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
	if batch.ID == "" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = savePreparationBatch(ctx, tx, project, batch); err != nil {
		return err
	}
	// Only a newly created batch selects the current pointer. Updating history
	// cannot steal the running worker's current batch.
	_, err = tx.ExecContext(ctx, `INSERT INTO maintenance_settings(project_identity,profile,generation_batch) VALUES(?,'',?) ON CONFLICT(project_identity) DO UPDATE SET generation_batch=CASE WHEN maintenance_settings.generation_batch IS NULL THEN excluded.generation_batch ELSE maintenance_settings.generation_batch END`, project, batch.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func savePreparationBatch(ctx context.Context, tx *sql.Tx, project string, batch pm.GenerationBatch) error {
	var previous []byte
	err := tx.QueryRowContext(ctx, `SELECT payload FROM preparation_batches WHERE project_identity=? AND batch_id=?`, project, batch.ID).Scan(&previous)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	var old pm.GenerationBatch
	if len(previous) > 0 {
		if err = json.Unmarshal(previous, &old); err != nil {
			return err
		}
	}
	if batch.CreatedAt.IsZero() {
		batch.CreatedAt = old.CreatedAt
		if batch.CreatedAt.IsZero() {
			batch.CreatedAt = now()
		}
	}
	if batch.Status == "completed" || batch.Status == "failed" {
		if batch.BusinessFinishedAt == nil {
			batch.BusinessFinishedAt = old.BusinessFinishedAt
			if batch.BusinessFinishedAt == nil {
				at := now()
				batch.BusinessFinishedAt = &at
			}
		}
		if batch.ResultSnapshot == nil {
			batch.ResultSnapshot = old.ResultSnapshot
		}
	}
	raw, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO preparation_batches VALUES(?,?,?) ON CONFLICT(project_identity,batch_id) DO UPDATE SET payload=excluded.payload`, project, batch.ID, raw)
	return err
}

// Bind only existing durable generation intents, before they can be submitted.
// SaveMaterial also calls this for reuse of an already registered generation.
func bindPreparationGenerations(ctx context.Context, tx *sql.Tx, project string) error {
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO preparation_associations
	SELECT s.project_identity,s.generation_batch,m.task_id,json_extract(CAST(m.payload AS TEXT),'$.inputs'),g.generation_id
	FROM maintenance_settings s JOIN preparation_batches b ON b.project_identity=s.project_identity AND b.batch_id=s.generation_batch
	JOIN json_each(CAST(b.payload AS TEXT),'$.items') i
	JOIN project_materials m ON m.project_identity=s.project_identity AND m.task_id=json_extract(i.value,'$.task_id')
	JOIN test_generations g ON g.project_identity=m.project_identity AND g.generation_id=json_extract(CAST(m.payload AS TEXT),'$.generation_id')
	WHERE s.project_identity=? AND json_extract(CAST(b.payload AS TEXT),'$.status') IN ('queued','preparing','generating')
	AND json_extract(CAST(m.payload AS TEXT),'$.inputs')=json_extract(CAST(g.payload AS TEXT),'$.inputs')`, project)
	return err
}

// The original request and its runnable batch become durable together, so a
// crash cannot acknowledge an intent that the preparation worker cannot find.
func (s *SQLiteStore) RegisterPreparation(ctx context.Context, project, id string, batch pm.GenerationBatch) error {
	raw, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error {
		if _, err := tx.tx.ExecContext(ctx, `INSERT INTO maintenance_requests(project_identity,request_id,digest,result) VALUES(?,?,?,?)`, project, id, id, raw); err != nil {
			return err
		}
		if err := savePreparationBatch(ctx, tx.tx, project, batch); err != nil {
			return err
		}
		_, err := tx.tx.ExecContext(ctx, `INSERT INTO maintenance_settings(project_identity,profile,generation_batch) VALUES(?,'',?) ON CONFLICT(project_identity) DO UPDATE SET generation_batch=excluded.generation_batch`, project, batch.ID)
		if err != nil {
			return err
		}
		return bindPreparationGenerations(ctx, tx.tx, project)
	})
}
