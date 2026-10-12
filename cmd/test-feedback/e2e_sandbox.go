package main

import (
	"errors"
	"regexp"
)

type sandboxScenarioProof struct {
	Passed             bool `json:"passed"`
	Cleanup            bool `json:"cleanup_complete"`
	UsageVerified      bool `json:"usage_verified"`
	ManualConfirmation bool `json:"manual_confirmation"`
	Deliveries         int  `json:"deliveries"`
	Candidates         []struct {
		TaskID   string `json:"task_id"`
		RunID    string `json:"run_id"`
		Session  string `json:"session_id"`
		Worktree string `json:"worktree"`
		SHA      string `json:"candidate_sha"`
		Stages   int    `json:"native_stages"`
		MergedPR struct {
			URL   string `json:"url"`
			State string `json:"state"`
			Head  string `json:"headRefOid"`
			Base  string `json:"baseRefName"`
		} `json:"merged_pr"`
		Usage []struct {
			Session string `json:"session_id"`
			Turn    string `json:"turn_id"`
			Model   string `json:"model"`
			Effort  string `json:"reasoning_effort"`
			Tokens  struct {
				Total int64 `json:"total_tokens"`
			} `json:"tokens"`
		} `json:"usage"`
	} `json:"candidates"`
}

func validateSandboxScenarioProof(p *sandboxScenarioProof) error {
	if p == nil || !p.Passed || !p.Cleanup || !p.UsageVerified || p.ManualConfirmation || p.Deliveries != 2 || len(p.Candidates) != 2 {
		return errors.New("requires two unattended delivered candidates, verified usage and cleanup")
	}
	seenRuns, seenSessions, seenWorktrees, seenPRs := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i, c := range p.Candidates {
		if c.TaskID != []string{"1", "2"}[i] || c.RunID == "" || c.Session == "" || c.Worktree == "" || c.Stages != 3 || len(c.Usage) != 3 || seenRuns[c.RunID] || seenSessions[c.Session] || seenWorktrees[c.Worktree] {
			return errors.New("sandbox candidates lack separate original task/run/session/worktree evidence")
		}
		if c.MergedPR.State != "MERGED" || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(c.SHA) || c.MergedPR.Head != c.SHA || !regexp.MustCompile(`^https://github.com/erranliu/SteerlineTest/pull/[1-9][0-9]*$`).MatchString(c.MergedPR.URL) || seenPRs[c.MergedPR.URL] {
			return errors.New("sandbox candidate lacks its exact merged SteerlineTest PR")
		}
		if c.MergedPR.Base == "" || c.MergedPR.Base != p.Candidates[0].MergedPR.Base {
			return errors.New("sandbox candidates differ from their dedicated common base")
		}
		seenTurns := map[string]bool{}
		for _, u := range c.Usage {
			if u.Session != c.Session || u.Turn == "" || seenTurns[u.Turn] || u.Model != "gpt-6-luna" || u.Effort != "medium" || u.Tokens.Total <= 0 {
				return errors.New("sandbox usage is missing, duplicated or belongs to another native identity/model")
			}
			seenTurns[u.Turn] = true
		}
		seenRuns[c.RunID], seenSessions[c.Session], seenWorktrees[c.Worktree], seenPRs[c.MergedPR.URL] = true, true, true, true
	}
	return nil
}
