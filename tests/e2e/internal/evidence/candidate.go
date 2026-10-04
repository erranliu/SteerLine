package evidence

import (
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

// Candidate resolves by both immutable IDs; list order/current profile are never
// authority. Keep the full snapshot elsewhere for decisions/operations/delivery.
func Candidate(d contracts.DomainSnapshot, runID, executionID string) (contracts.DomainSnapshot, error) {
	var selected contracts.DomainSnapshot
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
