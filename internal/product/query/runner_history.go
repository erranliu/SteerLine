package query

import (
	"encoding/json"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

// CandidateSummary describes caller participation and the last public Runner
// observation. It is never an executable workflow record.
func CandidateSummary(c runnerstate.Candidate) api.WorkflowSummary {
	var input runnercontract.RunInput
	_ = json.Unmarshal(c.Submit.Input, &input)
	v := api.WorkflowSummary{WorkflowRunID: c.ID, TaskID: c.TaskID, CandidateNumber: c.Number, CreatedAt: c.CreatedAt, FlowRef: input.Workflow.ID, Participation: c.Participation, SubmissionStatus: c.SubmitState, StopRequested: c.StopRequested, WorkflowStatus: runnerstate.ObservedStatus(c),
		ExecutionProfile: &api.ExecutionProfileSummary{ID: c.ProfileID, Host: input.Execution.Target.Kind, Agent: input.Execution.Adapter}}
	if c.Projection != nil {
		v.WorkflowStage = c.Projection.Step
	}
	return v
}
func RunnerHistory(candidates []runnerstate.Candidate) api.HistorySummary {
	v := api.HistorySummary{}
	for _, c := range candidates {
		v.WorkflowRuns = append(v.WorkflowRuns, CandidateSummary(c))
		if r := c.Projection; r != nil {
			if r.Execution.AttemptID != "" {
				v.Attempts = append(v.Attempts, api.AttemptSummary{ExecutionID: c.RunID, AttemptID: r.Execution.AttemptID, CreatedAt: c.CreatedAt})
			}
			if q := r.Question; q != nil {
				v.Decisions = append(v.Decisions, api.DecisionHistory{WorkflowRunID: c.ID, DecisionID: q.ID, ExecutionID: c.RunID, AttemptID: r.Execution.AttemptID, Status: r.Status, Version: r.Revision})
			}

		}
	}
	return v
}
