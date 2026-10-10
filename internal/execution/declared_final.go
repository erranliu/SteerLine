package execution

import (
	"context"
	"errors"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
	m "github.com/erranliu/SteerLine/internal/execution/model"
	r "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

// Explicit worker result protocols can supply canonical content without a
// provider history reader. The same settled evidence drives steps and the UI.
func (s *Service) recordDeclaredFinal(ctx context.Context, run *r.RunView, evidence b.TurnEvidence) error {
	if evidence.FinalOrigin != "worker_declared" || !evidence.FinalVerified {
		return nil
	}
	_, execution := s.workflowRecords(run)
	if execution == nil || evidence.CommandID != execution.CurrentCommandID || evidence.TurnID != execution.CurrentTurnID || evidence.SessionID != execution.SessionID {
		return m.ErrTurnStale
	}
	if !evidence.Completed || evidence.FinalItemID == "" || evidence.FinalSource == "" || evidence.LastMessage == "" || evidence.FinalNativeTurnID == "" {
		return errors.New("declared final lacks its complete settled provenance")
	}
	event := m.ConversationEvent{Version: m.ConversationVersion, ProjectID: execution.ProjectIdentity,
		TaskID: run.DomainTaskID, WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID,
		Backend: b.BackendKind(run.Backend), Protocol: b.ExecutionProtocol(run.Protocol), Agent: run.Agent, TargetID: run.Target.ID,
		Source: evidence.FinalSource, SourceRole: m.ConversationSourceCanonical,
		SourceIdentity: evidence.SessionID, SourceSegment: evidence.TurnID,
		SessionID: evidence.SessionID, SessionQuality: m.IdentityDerived, TurnID: evidence.TurnID, TurnQuality: m.IdentityDerived,
		ItemID: evidence.FinalItemID, ItemQuality: m.IdentityNative, SourceEventID: evidence.FinalItemID, SourceEventQuality: m.IdentityNative,
		Type: m.ConversationMessageComplete, Phase: m.MessagePhaseFinalAnswer, FinalAvailability: m.FinalAvailable, ReceivedAt: now(),
		Evidence: map[string]string{"phase_origin": "worker_declared", "settlement": "original_dispatch", "native_turn_id": evidence.FinalNativeTurnID}}
	event.ItemKey = event.ItemIdentity()
	event.Body = &m.ConversationBodyRef{ExecutionID: event.ExecutionID, AttemptID: event.AttemptID, ItemKey: event.ItemKey, Revision: 1, Source: event.Source, Completeness: m.BodyComplete}
	complete := event
	complete.Body, complete.ItemID, complete.ItemKey = nil, "", ""
	complete.ItemQuality = m.IdentityUnknown
	complete.SourceEventID = "settled:" + evidence.FinalNativeTurnID
	complete.Type, complete.Phase = m.ConversationTurnCompleted, m.MessagePhaseUnknown
	complete.Status = "succeeded"
	if !evidence.Succeeded {
		complete.Type, complete.Status = m.ConversationTurnFailed, "failed"
	}
	_, err := s.conversationStore.CommitConversation(ctx, []m.ConversationWrite{{Event: event, Body: []byte(evidence.LastMessage), BodyMode: "complete", BodySource: event.Source}, {Event: complete}}, m.ConversationCheckpoint{})
	return err
}
