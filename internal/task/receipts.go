package task

import (
	"context"
	"encoding/json"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// ReadReceipt projects the original durable task command. It never replays a
// mutation, validates Git files or changes request identity after a restart.
func (c Commands) ReadReceipt(ctx context.Context, id, kind, target string) (json.RawMessage, error) {
	record, found, err := c.Store.LookupCommand(ctx, c.Input.Project, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, &taskmodel.Error{Code: "command_not_found", Message: "original task command is absent"}
	}
	if record.ProjectIdentity != c.Input.Project || record.CommandID != id || record.Kind != kind || record.TargetID != target {
		return nil, taskError("identity_conflict", "original command kind or target differs")
	}
	if record.Status != "accepted" || !json.Valid(record.Result) {
		return nil, taskError("outcome_unknown", "original task command has no confirmed receipt")
	}
	return append(json.RawMessage(nil), record.Result...), nil
}
