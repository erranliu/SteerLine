package sqlite

import (
	"encoding/json"
	m "github.com/erranliu/SteerLine/internal/execution/model"
	p "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (tx *sqliteTx) appendWorkerStateActivity(beforeW *m.WorkflowRunRecord, beforeE *m.ExecutionRecord, w m.WorkflowRunRecord, e m.ExecutionRecord) error {
	event, changed := m.WorkerStateActivity(beforeW, beforeE, w, e, now())
	if !changed {
		return nil
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = tx.AppendEvent(p.DomainEvent{ProjectIdentity: w.ProjectIdentity, TaskID: w.TaskID, WorkflowRunID: w.WorkflowRunID, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, ExternalSource: "center:" + w.WorkflowRunID, ExternalID: event.ItemID, Type: "activity.worker_state", ReceivedAt: event.TS, Payload: payload})
	return err
}
