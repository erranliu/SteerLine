package model

import (
	"sort"
)

type DiscardFacts struct {
	PRURL             string
	PRURLs            []string
	CandidateBindings string
	UncertainPR       bool
	StopPending       []string
}

func CandidateDiscardFacts(workflows []WorkflowRunRecord, executions []ExecutionRecord, taskID string) DiscardFacts {
	runs := map[string]bool{}
	for _, run := range workflows {
		if run.TaskID == taskID {
			runs[run.WorkflowRunID] = true
		}
	}
	ownedExecutions := map[string]ExecutionRecord{}
	for _, execution := range executions {
		if runs[execution.WorkflowRunID] {
			ownedExecutions[execution.ExecutionID] = execution
		}
	}
	plan := DiscardFacts{StopPending: []string{}}
	prs := map[string]bool{}
	for _, run := range workflows {
		if run.TaskID != taskID {
			continue
		}
		for _, url := range append(append(append([]string(nil), run.PRHistory...), run.PRURL), run.PRCandidateURL) {
			if url != "" {
				prs[url] = true
			}
		}
		if run.PRURL != "" && (run.PRState == "" || run.PRState == "unknown") {
			plan.UncertainPR = true
		}
		if run.PRURL == "" && run.PRCandidateURL == "" && run.Status != "queued" {
			// A started worker may have published a PR before the center saw a
			// link. Discard cannot assume that no external PR exists.
			plan.UncertainPR = true
		}
	}
	for _, execution := range ownedExecutions {

		if execution.Status == "active" || execution.Status == "waiting_decision" || execution.Status == "blocked" || execution.Status == "pending" || execution.Process != nil && execution.Status != "stopped" {
			plan.StopPending = append(plan.StopPending, "execution_stop_manual:"+execution.ExecutionID)
		}
	}
	for url := range prs {
		plan.PRURLs = append(plan.PRURLs, url)
	}
	plan.PRURLs = sortedDiscardValues(plan.PRURLs)
	if len(plan.PRURLs) != 0 {
		plan.PRURL = plan.PRURLs[0]
	}
	plan.CandidateBindings = CandidateBindings(workflows, executions, taskID)
	plan.StopPending = sortedDiscardValues(plan.StopPending)
	return plan
}

func sortedDiscardValues(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
