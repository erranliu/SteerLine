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
		p.Audit = &api.RunnerAuditSummary{Model: a.Model, ReasoningEffort: a.ReasoningEffort, RepairCount: a.RepairCount, Results: []api.RunnerAuditResult{}}
		for _, result := range a.Results {
			v := api.RunnerAuditResult{Step: result.Step, SourceReportCommandID: result.SourceReportCommandID, CommandID: result.CommandID, TurnID: result.TurnID, Verdict: result.Verdict, Failure: result.Failure}
			for _, f := range result.Findings {
				v.Findings = append(v.Findings, api.RunnerFinding{Severity: f.Severity, Message: f.Message, Path: f.Path, Line: f.Line})
			}
			p.Audit.Results = append(p.Audit.Results, v)
		}
	}
}
