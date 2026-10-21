package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func readApproval(ctx context.Context, db schemaQuery, project, plan string) (pm.Approval, error) {
	var a pm.Approval
	var raw []byte
	e := db.QueryRowContext(ctx, `SELECT payload FROM maintenance_approvals WHERE project_identity=? AND plan=?`, project, plan).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return a, nil
	}
	if e != nil {
		return a, e
	}
	e = json.Unmarshal(raw, &a)
	return a, e
}
func (s *SQLiteStore) MaterialApproval(ctx context.Context, project, plan string) (pm.Approval, error) {
	return readApproval(ctx, s.db, project, plan)
}
func (s *SQLiteStore) SaveMaterialApproval(ctx context.Context, a pm.Approval, expected uint64) (pm.Approval, error) {
	var out pm.Approval
	if a.Project == "" || a.Plan == "" || a.RequestID == "" || a.By == "" || len(a.Inputs) != 64 || len(a.Output) != 64 {
		return out, errors.New("invalid material approval")
	}
	digest := pm.Hash(struct {
		Plan, Inputs, Output, Commit, By string
		Expected                         uint64
	}{a.Plan, a.Inputs, a.Output, a.Commit, a.By, expected})
	e := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		var d string
		var raw []byte
		e := tx.tx.QueryRowContext(ctx, `SELECT digest,result FROM maintenance_requests WHERE project_identity=? AND request_id=?`, a.Project, a.RequestID).Scan(&d, &raw)
		if e == nil {
			if d != digest {
				return errors.New("request_conflict: approval differs")
			}
			return json.Unmarshal(raw, &out)
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		old, e := readApproval(ctx, tx.tx, a.Project, a.Plan)
		if e != nil {
			return e
		}
		if old.Revision != expected {
			return errors.New("stale_revision: material approval changed")
		}
		a.Revision = expected + 1
		a.At = now()
		out = a
		raw, _ = json.Marshal(a)
		if _, e = tx.tx.ExecContext(ctx, `INSERT INTO maintenance_approvals VALUES(?,?,?,?) ON CONFLICT(project_identity,plan) DO UPDATE SET revision=excluded.revision,payload=excluded.payload`, a.Project, a.Plan, a.Revision, raw); e != nil {
			return e
		}
		_, e = tx.tx.ExecContext(ctx, `INSERT INTO maintenance_requests VALUES(?,?,?,?)`, a.Project, a.RequestID, digest, raw)
		return e
	})
	return out, e
}
func readExecution(ctx context.Context, db schemaQuery, project, id string) (taskmodel.ExecutionSnapshot, error) {
	var x taskmodel.ExecutionSnapshot
	var raw []byte
	e := db.QueryRowContext(ctx, `SELECT payload FROM task_execution_snapshots WHERE project_identity=? AND task_id=?`, project, id).Scan(&raw)
	if e != nil {
		return x, persistenceError(e)
	}
	e = json.Unmarshal(raw, &x)
	return x, e
}
func (s *SQLiteStore) TaskExecution(ctx context.Context, project, id string) (taskmodel.ExecutionSnapshot, error) {
	return readExecution(ctx, s.db, project, id)
}
func (s *SQLiteStore) ConfirmTaskMaterials(ctx context.Context, project, id string, version uint64, contract taskmodel.TaskContract, requestID string) (taskmodel.TaskRecord, error) {
	var result taskmodel.TaskRecord
	if project == "" || id == "" || requestID == "" || version == 0 {
		return result, errors.New("fixed task confirmation request required")
	}
	if e := taskmodel.ValidateContract(contract); e != nil {
		return result, e
	}
	digest := pm.Hash(struct {
		ID       string
		Version  uint64
		Contract taskmodel.TaskContract
	}{id, version, contract})
	e := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		var oldDigest string
		var saved []byte
		e := tx.tx.QueryRowContext(ctx, `SELECT digest,result FROM maintenance_requests WHERE project_identity=? AND request_id=?`, project, requestID).Scan(&oldDigest, &saved)
		if e == nil {
			if digest != oldDigest {
				return errors.New("request_conflict: task confirmation differs")
			}
			return json.Unmarshal(saved, &result)
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		graph, e := tx.TaskGraphSnapshot(ctx, project)
		if e != nil {
			return e
		}
		r, ok := graph.ByID[id]
		if !ok || r.Version != version || r.Status != "open" {
			return errors.New("stale_version: task changed")
		}
		var count int
		if e = tx.tx.QueryRowContext(ctx, `SELECT count(*) FROM runner_candidates WHERE project_identity=? AND task_id=?`, project, id).Scan(&count); e != nil {
			return e
		}
		if count != 0 {
			return errors.New("immutable_task: task already reserved")
		}
		old, e := tx.TaskContract(ctx, project, r)
		if e != nil {
			return e
		}
		if contract.ContractID != old.ContractID || contract.Repository != old.Repository || contract.Remote != old.Remote || contract.Branch != old.Branch {
			return errors.New("immutable_task: repository target cannot change")
		}
		raw, _ := json.Marshal(contract)
		if _, e = tx.tx.ExecContext(ctx, `UPDATE task_contracts SET digest=?,payload=? WHERE project_identity=? AND task_id=?`, contract.Digest, raw, project, id); e != nil {
			return e
		}
		r.Version++
		r.UpdatedAt = now()
		raw, _ = json.Marshal(r)
		res, e := tx.tx.ExecContext(ctx, `UPDATE tasks SET version=?,payload=? WHERE project_identity=? AND task_id=? AND version=?`, r.Version, raw, project, id, version)
		if e = requireIdentityWrite(res, e, "stale_version: task confirmation changed"); e != nil {
			return e
		}
		result = r
		_, e = tx.tx.ExecContext(ctx, `INSERT INTO maintenance_requests VALUES(?,?,?,?)`, project, requestID, digest, raw)
		return e
	})
	return result, e
}
func verifyMaterial(ctx context.Context, tx *sql.Tx, project string, w taskmodel.FixedWork) error {
	if w.MaterialPlan == "" {
		return nil
	}
	a, e := readApproval(ctx, tx, project, w.MaterialPlan)
	if e != nil {
		return e
	}
	if a.Revision != w.MaterialRevision || a.Inputs != w.MaterialInputs || a.Output != w.MaterialOutput {
		return fmt.Errorf("materials_changed: reviewed Plan changed before reservation")
	}
	return nil
}

func (s *SQLiteStore) MaintenanceReceipt(ctx context.Context, project, id string) (json.RawMessage, error) {
	var raw []byte
	e := s.db.QueryRowContext(ctx, `SELECT result FROM maintenance_requests WHERE project_identity=? AND request_id=?`, project, id).Scan(&raw)
	return raw, persistenceError(e)
}
