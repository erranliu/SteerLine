package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	backend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// The set includes historical and abandoned PRs. Sampling/turn diagnostics do
// not alter this binding comparison; new candidates, PRs or exits do.
func CandidateBindings(workflows []WorkflowRunRecord, executions []ExecutionRecord, taskID string) string {
	type binding struct {
		ID, Selected, PR, Proposed, Profile string
		Execution                           string
		History                             []string
		Exit                                *CandidateAbandonment
	}
	values := []binding{}
	for _, w := range workflows {
		if w.TaskID == taskID {
			identity := ""
			for _, e := range executions {
				if e.WorkflowRunID == w.WorkflowRunID && e.ExecutionID == w.SelectedResult {
					identity = bindingDigest(struct {
						Project, ID, Workflow, Generation string
						Workspace                         WorkspaceEvidence
						Target                            backend.ExecutionTarget
						Data                              []backend.ImplementationDataEnvelope
						Policy                            *PolicyResources
					}{e.ProjectIdentity, e.ExecutionID, e.WorkflowRunID, e.Generation, e.Workspace, e.Target, e.ImplementationData, e.PolicyResources})
					break
				}
			}
			values = append(values, binding{ID: w.WorkflowRunID, Selected: w.SelectedResult, PR: w.PRURL, Proposed: w.PRCandidateURL, Profile: bindingDigest(w.ExecutionProfile), Execution: identity, History: w.PRHistory, Exit: w.Abandonment})
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return bindingDigest(values)
}

// Start safety rechecks known PR identities, not participation count or the
// membership of PR-less siblings. Two starts sampled below five may both commit.
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
