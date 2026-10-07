package query

import (
	"encoding/json"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

// ConversationEvent projects display content and stable read targets. Native
// source cursors, private envelopes and complete original input live elsewhere.
func ConversationEvent(e executionmodel.ConversationEvent) (api.ConversationEvent, error) {
	p := api.ConversationEvent{Version: e.Version, EventID: e.EventID, ProjectID: e.ProjectID, TaskID: e.TaskID, WorkflowID: e.WorkflowID, ExecutionID: e.ExecutionID, AttemptID: e.AttemptID, Backend: string(e.Backend), Protocol: string(e.Protocol), Agent: e.Agent, TargetID: e.TargetID, Source: e.Source, SourceRole: api.ConversationSourceRole(e.SourceRole), SourceSegment: e.SourceSegment, SourceIdentity: e.SourceIdentity, SessionID: e.SessionID, SessionQuality: api.IdentityQuality(e.SessionQuality), TurnID: e.TurnID, TurnQuality: api.IdentityQuality(e.TurnQuality), ItemID: e.ItemID, ItemQuality: api.IdentityQuality(e.ItemQuality), ItemKey: e.ItemKey, SourceEventID: e.SourceEventID, SourceEventQuality: api.IdentityQuality(e.SourceEventQuality), SourceSeq: e.SourceSeq, SourceRevision: e.SourceRevision, Type: api.ConversationEventType(e.Type), Phase: api.MessagePhase(e.Phase), FinalAvailability: api.FinalAvailability(e.FinalAvailability), Status: e.Status, OccurredAt: copyValue(e.OccurredAt), ReceivedAt: e.ReceivedAt, Diagnostic: e.Diagnostic}
	if b := e.Body; b != nil {
		p.Body = &api.ConversationBodyRef{ExecutionID: b.ExecutionID, AttemptID: b.AttemptID, ItemKey: b.ItemKey, Revision: b.Revision, SHA256: b.SHA256, Bytes: b.Bytes, Source: b.Source, Completeness: api.BodyCompleteness(b.Completeness)}
	}
	// Only the display end reason is public. Private native attributes and
	// implementation payloads are never forwarded as an open JSON object.
	if e.Attributes != nil {
		_, err := json.Marshal(e.Attributes)
		if err != nil {
			return api.ConversationEvent{}, err
		}
		p.EndReason, _ = e.Attributes["reason"].(string)
	}
	return p, nil
}
