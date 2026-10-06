package evidence

import (
	"errors"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

// Count complete canonical messages by stable item identity. Native readiness
// and business final messages can share a turn ID; replaying an item adds none.
func CandidateFinalCount(events []productapi.ConversationEvent, session string) (int, error) {
	if session == "" {
		return 0, errors.New("candidate native session is missing")
	}
	items := map[string]bool{}
	for _, event := range events {
		if event.Type != productapi.ConversationMessageComplete || event.Phase != productapi.MessagePhaseFinalAnswer || event.SourceRole != productapi.ConversationSourceCanonical {
			continue
		}
		if event.SessionID != session {
			return 0, errors.New("canonical final belongs to another native session")
		}
		if event.TurnID == "" || event.ItemKey == "" || event.Body == nil {
			return 0, errors.New("canonical final lacks its native turn, item or body identity")
		}
		items[event.ItemKey] = true
	}
	return len(items), nil
}

// Candidate resolves by both immutable IDs; list order/current profile are never
// authority. Keep the full snapshot elsewhere for decisions/operations/delivery.
func Candidate(d reconstruction.Snapshot, runID, executionID string) (reconstruction.Snapshot, error) {
	var selected reconstruction.Snapshot
	selected.ProjectIdentity = d.ProjectIdentity
	for _, w := range d.WorkflowRuns {
		if w.WorkflowRunID == runID {
			selected.WorkflowRuns = append(selected.WorkflowRuns, w)
		}
	}
	for _, e := range d.Executions {
		if e.ExecutionID == executionID {
			selected.Executions = append(selected.Executions, e)
		}
	}
	if len(selected.WorkflowRuns) != 1 || len(selected.Executions) != 1 || selected.Executions[0].WorkflowRunID != runID || selected.WorkflowRuns[0].ProjectIdentity != selected.Executions[0].ProjectIdentity || selected.WorkflowRuns[0].ExecutionProfile == nil {
		return selected, errors.New("candidate evidence identity missing/duplicate/foreign")
	}
	return selected, nil
}
