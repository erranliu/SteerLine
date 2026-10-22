package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerclient"
)

func (r *clientReader) RunUsage(ctx context.Context, candidateID, execution string) (api.RunUsageView, error) {
	view := api.RunUsageView{RunID: candidateID, ExecutionID: execution}
	config := r.config
	reader, err := OpenNativeTaskReader(ctx, &config)
	if err != nil {
		return view, err
	}
	defer reader.Close()
	candidate, err := reader.Candidate(ctx, r.project, candidateID)
	if err != nil {
		return view, err
	}
	if execution == "" || candidate.RunID != execution {
		return view, errors.New("usage differs from fixed candidate/Run")
	}
	cli, err := r.config.runnerClient(ctx)
	if err != nil {
		return view, err
	}
	if !strings.EqualFold(filepath.Clean(candidate.StateDir), filepath.Clean(cli.StateDir)) {
		return view, errors.New("Runner state directory differs from the fixed candidate")
	}
	return readRunnerUsage(ctx, cli, candidateID, execution, r.config.RunnerExecution.PricingPolicy)
}

func readRunnerUsage(ctx context.Context, cli runnerclient.Client, candidate, execution, policy string) (api.RunUsageView, error) {
	view := api.RunUsageView{RunID: candidate, ExecutionID: execution}
	if !cli.RunUsageSupported {
		view.Reason = "USAGE_UNSUPPORTED"
		return view, nil
	}
	u, err := cli.RunUsage(ctx, execution)
	if err != nil {
		return view, err
	}
	view.Usage = &api.UsageSummary{Revision: u.Revision, Status: u.Status, Scope: u.Scope, Tokens: (*api.UsageTokens)(u.Tokens), Finalized: u.Coverage.Finalized, CountedResponses: u.Coverage.CountedResponses, Issues: []string{}, AccessCleanupStatus: u.AccessCleanupStatus, ObservedAt: u.ObservedAt}
	for _, issue := range u.Issues {
		view.Usage.Issues = append(view.Usage.Issues, issue.Code)
	}
	if policy == "" {
		view.CostReason = "PRICE_POLICY_NOT_CONFIGURED"
		return view, nil
	}
	if !filepath.IsAbs(policy) {
		view.CostReason = "PRICE_POLICY_INVALID"
		return view, nil
	}
	if !cli.RunCostSupported {
		view.CostReason = "COST_UNSUPPORTED"
		return view, nil
	}
	p, err := cli.EstimateRunCost(ctx, u, policy)
	if err != nil {
		// A pricing/query error must never hide the independently obtained tokens.
		view.CostReason = "COST_QUERY_FAILED"
		return view, nil
	}
	view.Cost = &api.CostSummary{Status: p.Status, UsageRevision: p.UsageRevision, PolicyID: p.PolicyID, PolicyVersion: p.PolicyVersion, PolicyDigest: p.PolicyDigest, Currency: p.Currency, Covered: p.Covered, Total: p.Total, Priced: p.Priced, Unpriced: p.Unpriced, Assumptions: p.Assumptions, Issues: p.Issues}
	return view, nil
}
