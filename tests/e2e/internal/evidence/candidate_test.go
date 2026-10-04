package evidence

import (
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"testing"
)

func TestCandidateEvidenceDoesNotFollowListOrderOrSibling(t *testing.T) {
	d := fixture(t)
	d.WorkflowRuns[0].WorkflowRunID = "run-a"
	d.WorkflowRuns[0].ProjectIdentity = "project"
	d.Executions[0].WorkflowRunID = "run-a"
	sibling := executionmodel.ExecutionRecord{ProjectIdentity: "project", ExecutionID: "sibling", WorkflowRunID: "run-b"}
	d.Executions = append([]executionmodel.ExecutionRecord{sibling}, d.Executions...)
	d.WorkflowRuns = append(d.WorkflowRuns, executionmodel.WorkflowRunRecord{WorkflowRunID: "run-b", ProjectIdentity: "project"})
	selected, err := Candidate(d, "run-a", "execution")
	if err != nil {
		t.Fatal(err)
	}
	if err := FullAccess(selected); err != nil {
		t.Fatal(err)
	}
	if _, err := Candidate(d, "run-a", "sibling"); err == nil {
		t.Fatal("sibling evidence accepted")
	}
	d.Executions = append(d.Executions, d.Executions[1])
	if _, err := Candidate(d, "run-a", "execution"); err == nil {
		t.Fatal("duplicate execution accepted")
	}
}
