package testenv

import (
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestCandidatePublicationFailureRequiresCompletedOriginalFeedback(t *testing.T) {
	w := executionmodel.WorkflowRunRecord{Status: "waiting_input"}
	e := executionmodel.ExecutionRecord{Status: "idle", CurrentCommandID: "feedback", CurrentTurnID: "turn", LastStoppedTurnID: "turn"}
	if !candidatePublicationEndedWithoutPR(w, e, "feedback") {
		t.Fatal("completed original feedback without PR was not diagnosed")
	}
	for _, test := range []struct {
		name string
		edit func(*executionmodel.WorkflowRunRecord, *executionmodel.ExecutionRecord)
	}{
		{"previous command", func(_ *executionmodel.WorkflowRunRecord, e *executionmodel.ExecutionRecord) {
			e.CurrentCommandID = "initial"
		}},
		{"running", func(_ *executionmodel.WorkflowRunRecord, e *executionmodel.ExecutionRecord) { e.Status = "active" }},
		{"pending command", func(_ *executionmodel.WorkflowRunRecord, e *executionmodel.ExecutionRecord) {
			e.PendingCommandID = "feedback"
		}},
		{"pending turn", func(_ *executionmodel.WorkflowRunRecord, e *executionmodel.ExecutionRecord) { e.PendingTurnID = "next" }},
		{"unsettled turn", func(_ *executionmodel.WorkflowRunRecord, e *executionmodel.ExecutionRecord) {
			e.LastStoppedTurnID = "previous"
		}},
		{"unstarted turn", func(_ *executionmodel.WorkflowRunRecord, e *executionmodel.ExecutionRecord) {
			e.CurrentTurnID, e.LastStoppedTurnID = "", ""
		}},
		{"verified PR", func(w *executionmodel.WorkflowRunRecord, _ *executionmodel.ExecutionRecord) {
			w.Status = "waiting_pr_merge"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			currentW, currentE := w, e
			test.edit(&currentW, &currentE)
			if candidatePublicationEndedWithoutPR(currentW, currentE, "feedback") {
				t.Fatal("unrelated or unfinished feedback was treated as publication failure")
			}
		})
	}
	if candidatePublicationEndedWithoutPR(w, e, "") {
		t.Fatal("unbound feedback was treated as publication failure")
	}
}
