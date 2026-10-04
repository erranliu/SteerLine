package query

import (
	"encoding/json"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

// ConversationEvent projects display content and stable read targets. Native
// source cursors, private envelopes and complete original input live elsewhere.
func ConversationEvent(e contracts.ConversationEvent) (api.ConversationEvent, error) {
	p := api.ConversationEvent{Version: e.Version, EventID: e.EventID, ProjectID: e.ProjectID, TaskID: e.TaskID, WorkflowID: e.WorkflowID, ExecutionID: e.ExecutionID, AttemptID: e.AttemptID, Backend: string(e.Backend), Protocol: string(e.Protocol), Agent: e.Agent, TargetID: e.TargetID, Source: e.Source, SourceRole: api.ConversationSourceRole(e.SourceRole), SourceSegment: e.SourceSegment, SourceIdentity: e.SourceIdentity, SessionID: e.SessionID, SessionQuality: api.IdentityQuality(e.SessionQuality), TurnID: e.TurnID, TurnQuality: api.IdentityQuality(e.TurnQuality), ItemID: e.ItemID, ItemQuality: api.IdentityQuality(e.ItemQuality), ItemKey: e.ItemKey, SourceEventID: e.SourceEventID, SourceEventQuality: api.IdentityQuality(e.SourceEventQuality), SourceSeq: e.SourceSeq, SourceRevision: e.SourceRevision, Type: api.ConversationEventType(e.Type), Phase: api.MessagePhase(e.Phase), FinalAvailability: api.FinalAvailability(e.FinalAvailability), Status: e.Status, OccurredAt: copyValue(e.OccurredAt), ReceivedAt: e.ReceivedAt, Diagnostic: e.Diagnostic}
	if b := e.Body; b != nil {
		p.Body = &api.ConversationBodyRef{ExecutionID: b.ExecutionID, AttemptID: b.AttemptID, ItemKey: b.ItemKey, Revision: b.Revision, SHA256: b.SHA256, Bytes: b.Bytes, Source: b.Source, Completeness: api.BodyCompleteness(b.Completeness)}
	}
	if r := e.Raw; r != nil {
		p.Raw = &api.ConversationRawRef{ExecutionID: r.ExecutionID, AttemptID: r.AttemptID, ID: r.ID, SHA256: r.SHA256, Bytes: r.Bytes}
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

func ConversationPage(p contracts.ConversationPage) (api.ConversationPage, error) {
	view := api.ConversationPage{Events: []api.ConversationEvent{}, NextCursor: p.NextCursor, HasMore: p.HasMore, PayloadBytes: p.PayloadBytes}
	for _, e := range p.Events {
		v, err := ConversationEvent(e)
		if err != nil {
			return api.ConversationPage{}, err
		}
		view.Events = append(view.Events, v)
	}
	return view, nil
}

func CapabilityReport(r contracts.ConversationCapabilityReport) api.ConversationCapabilityReport {
	p := api.ConversationCapabilityReport{Version: r.Version, Backend: string(r.Backend), Protocol: string(r.Protocol), Agent: r.Agent, AgentVersion: r.AgentVersion, PlatformVersion: r.PlatformVersion, Target: api.TargetSummary{ID: r.Target.ID}, ExecutionID: r.ExecutionID, AttemptID: r.AttemptID, Source: r.Source, SourceIdentity: r.SourceIdentity, CheckedAt: r.CheckedAt}
	for _, c := range r.Capabilities {
		p.Capabilities = append(p.Capabilities, api.ConversationCapability{Name: c.Name, Status: string(c.Status), Evidence: c.Evidence})
	}
	return p
}
