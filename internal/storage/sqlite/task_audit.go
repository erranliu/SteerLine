package sqlite

import (
	"encoding/json"
	"errors"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (t *sqliteTx) RecordPredecessorAudit(v storageports.CommandRecord) error {
	if v.Kind != "add_task_predecessors" || v.Status != "accepted" || v.CommandID == "" || v.ProjectIdentity == "" {
		return errors.New("predecessor audit requires its accepted Task command")
	}
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = t.tx.ExecContext(t.ctx, `INSERT INTO task_audits(project_identity,command_id,payload) VALUES(?,?,?)`, v.ProjectIdentity, v.CommandID, b)
	return e
}
