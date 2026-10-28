package query

import api "github.com/erranliu/SteerLine/internal/product/api"

func runnerWorkflow(r *CandidateView, p *api.CandidateSummary) {
	p.RunnerRevision, p.InputDigest = r.RunnerRevision, r.InputDigest
	p.RequestedModel, p.RequestedReasoningEffort = r.RequestedModel, r.RequestedReasoningEffort
	if w := r.RunnerWorkflow; w != nil {
		p.RunnerWorkflow = &api.RunnerWorkflowSummary{ID: w.ID, Version: w.Version}
	}
	for _, m := range r.Materials {
		p.Materials = append(p.Materials, api.RunnerMaterialSummary{Kind: m.Kind, Path: m.Path, Commit: m.Commit, Digest: m.Digest})
	}
	if a := r.Audit; a != nil {
		p.Audit = &api.RunnerAuditSummary{Model: a.Model, ReasoningEffort: a.ReasoningEffort, Results: []api.RunnerAuditResult{}}
		for _, result := range a.Results {
			s := result.ImplementationSource
			v := api.RunnerAuditResult{Step: result.Step, SourceResultID: result.SourceResultID, ImplementationSource: api.RunnerAuditSource{SessionID: s.SessionID, CommandID: s.CommandID, TurnID: s.TurnID, AttemptID: s.AttemptID, InputVersion: s.InputVersion, FinalDigest: s.FinalDigest}, ResultID: result.ResultID, CommandID: result.CommandID, TurnID: result.TurnID, PreHead: result.PreHead, PostHead: result.PostHead, Failure: result.Failure}
			if result.FixedCount != nil {
				count := *result.FixedCount
				v.FixedCount = &count
			}
			if result.FixedIssues != nil {
				v.FixedIssues = make([]api.RunnerFixedIssue, 0, len(result.FixedIssues))
			}
			for _, f := range result.FixedIssues {
				v.FixedIssues = append(v.FixedIssues, api.RunnerFixedIssue{Severity: f.Severity, Issue: f.Issue, Repair: f.Repair, Evidence: f.Evidence})
			}
			if result.RemainingIssues != nil {
				v.RemainingIssues = make([]api.RunnerRemainingIssue, 0, len(result.RemainingIssues))
			}
			for _, f := range result.RemainingIssues {
				v.RemainingIssues = append(v.RemainingIssues, api.RunnerRemainingIssue{Severity: f.Severity, Issue: f.Issue, Reason: f.Reason})
			}
			p.Audit.Results = append(p.Audit.Results, v)
		}
	}
}
