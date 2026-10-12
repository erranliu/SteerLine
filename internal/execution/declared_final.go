package execution

import (
	"context"
	"errors"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
	m "github.com/erranliu/SteerLine/internal/execution/model"
	r "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

// Adapter-settled replies can supply content without a provider history reader.
// Keep their declared/rendered provenance; do not invent native turn identities.
func (s *Service) recordDeclaredFinal(ctx context.Context, run *r.RunView, evidence b.TurnEvidence) error {
	if (evidence.FinalOrigin != "worker_declared" && evidence.FinalOrigin != "terminal_observed") || !evidence.FinalVerified {
		return nil
	}
	_, execution := s.workflowRecords(run)
	if execution == nil || evidence.CommandID != execution.CurrentCommandID || evidence.TurnID != execution.CurrentTurnID || evidence.SessionID != execution.SessionID {
		return m.ErrTurnStale
	}
	if !evidence.Started || !evidence.Completed || evidence.FinalItemID == "" || evidence.FinalSource == "" || evidence.LastMessage == "" || evidence.FinalNativeTurnID != "" || evidence.FinalNativeSessionID != "" {
		return errors.New("declared final lacks its complete settled provenance")
	}
	event := m.ConversationEvent{Version: m.ConversationVersion, ProjectID: execution.ProjectIdentity,
		TaskID: run.DomainTaskID, WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID,
		Backend: b.BackendKind(run.Backend), Protocol: b.ExecutionProtocol(run.Protocol), Agent: run.Agent, TargetID: run.Target.ID,
		Source: evidence.FinalSource, SourceRole: m.ConversationSourceCanonical,
		SourceIdentity: evidence.SessionID, SourceSegment: evidence.TurnID,
		SessionID: evidence.SessionID, SessionQuality: m.IdentityDerived, TurnID: evidence.TurnID, TurnQuality: m.IdentityDerived,
		ItemID: evidence.FinalItemID, ItemQuality: m.IdentityDerived, SourceEventID: evidence.FinalItemID, SourceEventQuality: m.IdentityDerived,
		Type: m.ConversationMessageComplete, Phase: m.MessagePhaseFinalAnswer, FinalAvailability: m.FinalAvailable, ReceivedAt: now(),
		Evidence: map[string]string{"phase_origin": evidence.FinalOrigin, "settlement": "bound_worker_reply", "command_id": evidence.CommandID}}
	event.ItemKey = event.ItemIdentity()
	event.Body = &m.ConversationBodyRef{ExecutionID: event.ExecutionID, AttemptID: event.AttemptID, ItemKey: event.ItemKey, Revision: 1, Source: event.Source, Completeness: m.BodyComplete}
	complete := event
	complete.Body, complete.ItemID, complete.ItemKey = nil, "", ""
	complete.ItemQuality = m.IdentityUnknown
	complete.SourceEventID = "settled:" + evidence.FinalItemID
	complete.Type, complete.Phase = m.ConversationTurnCompleted, m.MessagePhaseUnknown
	complete.Status = "succeeded"
	if !evidence.Succeeded {
		complete.Type, complete.Status = m.ConversationTurnFailed, "failed"
	}
	_, err := s.conversationStore.CommitConversation(ctx, []m.ConversationWrite{{Event: event, Body: []byte(evidence.LastMessage), BodyMode: "complete", BodySource: event.Source}, {Event: complete}}, m.ConversationCheckpoint{})
	return err
}
