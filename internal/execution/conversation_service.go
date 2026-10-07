package execution

import (
	"context"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

// Raw output stays in the native journal. Only waits and collection gaps enter
// the center; questions and operation receipts have their own durable records.
func (s *Service) recordConversationEvent(run *reconstructiontypes.RunView, source WorkerEvent) error {
	if run == nil || (!source.SourceGap && !source.InputWait) || s.conversationStore == nil {
		return nil
	}
	attemptID := source.AttemptID
	if attemptID == "" {
		attemptID = run.AttemptID
	}
	if !executionbackend.StableExecutionID(run.ExecutionID) || !executionbackend.StableExecutionID(attemptID) {
		return nativeErr("contract_mismatch", "conversation fact has no fixed execution identity")
	}
	event := executionmodel.ConversationEvent{Version: executionmodel.ConversationVersion, ProjectID: s.Config.identity, TaskID: run.DomainTaskID, WorkflowID: run.RunID,
		ExecutionID: run.ExecutionID, AttemptID: attemptID, Backend: executionbackend.BackendKind(run.Backend), Protocol: executionbackend.ExecutionProtocol(run.Protocol),
		Agent: run.Agent, TargetID: run.Target.ID, Source: source.Source, SourceRole: executionmodel.ConversationSourceFallback,
		SourceSegment: source.SourceSegment, SourceIdentity: source.SourceIdentity, SourceEventID: source.SourceEventID,
		SessionID: source.Session, SessionQuality: executionmodel.IdentityUnknown, TurnQuality: executionmodel.IdentityUnknown,
		ItemQuality: executionmodel.IdentityUnknown, SourceEventQuality: executionmodel.IdentityDerived,
		Phase: executionmodel.MessagePhaseUnknown, ReceivedAt: now(), Diagnostic: source.Diagnostic}
	if event.Source == "" {
		event.Source = "execution-observer"
	}
	if event.SourceSegment == "" {
		event.SourceSegment = attemptID
	}
	event.Type, event.Status = executionmodel.ConversationGap, "gap"
	if !source.SourceGap {
		event.Type, event.Status = executionmodel.ConversationInputWait, "waiting"
		event.Attributes = map[string]any{"wait_origin": "host", "business_decision": false}
	}
	if event.SourceEventID == "" {
		event.SourceEventID = recordFingerprint(struct {
			Type       executionmodel.ConversationEventType
			Generation string
			Sequence   uint64
			Diagnostic string
		}{event.Type, processGeneration(run.Process), source.SourceSeq, source.Diagnostic})
	}
	_, err := s.conversationStore.CommitConversation(context.Background(), []executionmodel.ConversationWrite{{Event: event}}, executionmodel.ConversationCheckpoint{})
	return err
}

func processGeneration(process *executionbackend.ProcessIdentity) string {
	if process == nil {
		return ""
	}
	return process.Incarnation
}

func conversationProcessSnapshot(process *executionbackend.ProcessIdentity) *executionbackend.ProcessIdentity {
	if process == nil {
		return nil
	}
	identity := *process
	return &identity
}

func validateConversationSourceWrites(expected executionmodel.ConversationEvent, writes []executionmodel.ConversationWrite) error {
	for _, write := range writes {
		event := write.Event
		if event.ProjectID != expected.ProjectID || event.TaskID != expected.TaskID || event.WorkflowID != expected.WorkflowID || event.ExecutionID != expected.ExecutionID || event.AttemptID != expected.AttemptID || event.Backend != expected.Backend || event.Protocol != expected.Protocol || event.Agent != expected.Agent || event.TargetID != expected.TargetID {
			return errors.New("conversation source changed its fixed project, execution or attempt owner")
		}
		if expected.Source != "" && (event.Source != expected.Source || event.SourceSegment != expected.SourceSegment || event.SourceIdentity != expected.SourceIdentity) {
			return errors.New("conversation source changed its discovered source identity")
		}
	}
	return nil
}
