package steerline

import (
	"encoding/json"
	"fmt"
	"os"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// note queues a canonical event for the next domain transaction.
func (s *Service) note(r *Run, source, kind, status, title, detail string, meta map[string]any) {
	ev := ActivityEvent{Source: source, Kind: kind, Status: status, Title: title, Detail: detail, Meta: meta}
	s.recordActivity(r, ev)
	s.emitConsole(ev)
}

func (s *Service) recordActivity(r *Run, ev ActivityEvent) {
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

func (s *Service) emitConsole(ev ActivityEvent) {
	if s.Console == "" || s.Console == "off" {
		return
	}
	out := s.consoleOut
	if out == nil {
		out = os.Stdout
	}
	switch s.Console {
	case "json":
		// The console combines service events with several per-run journals.
		// Give that output stream its own sequence without changing the journals.
		s.consoleSequence++
		ev.V, ev.Seq = ActivityVersion, s.consoleSequence
		if ev.TS.IsZero() {
			ev.TS = now()
		}
		b, e := json.Marshal(ev)
		if e == nil {
			fmt.Fprintln(out, string(b))
		}
	case "summary":
		if ev.Source == "steer" {
			fmt.Fprintln(out, RenderActivity(ev, RenderOptions{Color: false}))
		}
	default:
		fmt.Fprintln(out, RenderActivity(ev, RenderOptions{Color: stdoutColor()}))
	}
}
