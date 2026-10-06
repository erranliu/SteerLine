package execution

import (
	"encoding/json"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// note queues a canonical event for the next domain transaction.
func (s *Service) note(r *reconstructiontypes.RunView, source, kind, status, title, detail string, meta map[string]any) {
	ev := executionvalues.ActivityEvent{Source: source, Kind: kind, Status: status, Title: title, Detail: detail, Meta: meta}
	s.recordActivity(r, ev)
	s.emitConsole(ev)
}

func (s *Service) recordActivity(r *reconstructiontypes.RunView, ev executionvalues.ActivityEvent) {
	if r == nil {
		return
	}
	ev.V, ev.RunID, ev.TaskID = ActivityVersion, r.RunID, r.TaskID
	if ev.TS.IsZero() {
		ev.TS = now()
	}
	metadata := map[string]any{}
	for key, value := range ev.Meta {
		metadata[key] = value
	}
	metadata["attempt_id"], metadata["execution_id"] = r.AttemptID, r.ExecutionID
	ev.Meta = metadata
	payload, err := json.Marshal(ev)
	if err != nil {
		s.setError(err)
		return
	}
	domainTask, executionID := r.DomainTaskID, r.ExecutionID
	if domainTask == "" || executionID == "" || !executionbackend.StableExecutionID(r.RunID) || !executionbackend.StableExecutionID(r.AttemptID) {
		s.setError(nativeErr("unsupported_schema", "native activity lacks its fixed task, run, execution or attempt identity"))
		return
	}
	externalID := newID()
	if ev.ItemID != "" {
		externalID = "activity-" + r.AttemptID + "-" + operationDigest([]byte(ev.Source + "\x00" + ev.ItemID))[:24]
	}
	s.pendingEvents = append(s.pendingEvents, storageports.DomainEvent{ProjectIdentity: s.Config.identity, TaskID: domainTask, WorkflowRunID: r.RunID, ExecutionID: executionID, AttemptID: r.AttemptID, ExternalSource: "center:" + r.RunID, ExternalID: externalID, Type: "activity." + ev.Kind, ReceivedAt: ev.TS, Payload: payload})
}
