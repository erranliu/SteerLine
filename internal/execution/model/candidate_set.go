package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	backend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// The set includes historical PRs. Sampling/turn diagnostics do
// not alter this binding comparison; new candidates, PRs or exits do.
func CandidateBindings(workflows []WorkflowRunRecord, executions []ExecutionRecord, taskID string) string {
	type binding struct {
		ID, Selected, PR, Proposed, Profile string
		Execution                           string
		FlowRef, StepID, Status             string
		InputVersion                        uint64
		History                             []string
	}
	values := []binding{}
	for _, w := range workflows {
		if w.TaskID == taskID {
			identity := ""
			for _, e := range executions {
				if e.WorkflowRunID == w.WorkflowRunID && e.ExecutionID == w.SelectedResult {
					identity = bindingDigest(struct {
						Project, ID, Workflow, Generation  string
						Repository, Worktree, Branch, Base string
						Target                             backend.ExecutionTarget
						Data                               []backend.ImplementationDataEnvelope
						Policy                             *PolicyResources
					}{e.ProjectIdentity, e.ExecutionID, e.WorkflowRunID, e.Generation, e.Workspace.Repository, e.Workspace.Worktree, e.Workspace.Branch, e.Workspace.Base, e.Target, e.ImplementationData, e.PolicyResources})
					break
				}
			}
			values = append(values, binding{FlowRef: w.FlowRef, StepID: w.StepID, Status: w.Status, InputVersion: w.InputVersion, ID: w.WorkflowRunID, Selected: w.SelectedResult, PR: w.PRURL, Proposed: w.PRCandidateURL, Profile: bindingDigest(w.ExecutionProfile), Execution: identity, History: w.PRHistory})
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return bindingDigest(values)
}

// Start safety rechecks known PR identities, not participation count or the
// membership of PR-less siblings. Admission checks participation separately
// inside the reservation transaction.
func CandidatePRBindings(workflows []WorkflowRunRecord, taskID string) string {
	known := []WorkflowRunRecord{}
	for _, w := range workflows {
		if w.TaskID == taskID && (w.PRURL != "" || w.PRCandidateURL != "" || len(w.PRHistory) != 0) {
			known = append(known, w)
		}
	}
	return CandidateBindings(known, nil, taskID)
}

func bindingDigest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
