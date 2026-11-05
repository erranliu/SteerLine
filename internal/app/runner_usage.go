package app

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/billing"
	"github.com/erranliu/SteerLine/internal/billingconfig"
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
	return readRunnerUsage(ctx, cli, candidateID, execution, HomeDir())
}

func readRunnerUsage(ctx context.Context, cli runnerclient.Client, candidate, execution, home string) (api.RunUsageView, error) {
	view := api.RunUsageView{RunID: candidate, ExecutionID: execution}
	if !cli.RunUsageSupported {
		view.Reason = "USAGE_UNSUPPORTED"
		return view, nil
	}
	settings, priceErr := (billingconfig.Store{Home: home}).Read()
	policy, activeErr := settings.ActivePolicy()
	u, err := cli.RunUsage(ctx, execution)
	if err != nil {
		return view, err
	}
	view.Usage = &api.UsageSummary{Revision: u.Revision, Status: u.Status, Scope: u.Scope, Tokens: (*api.UsageTokens)(u.Tokens), Finalized: u.Coverage.Finalized, CountedResponses: u.Coverage.CountedResponses, Issues: []api.UsageIssue{}, AccessCleanupStatus: u.AccessCleanupStatus, ObservedAt: u.ObservedAt}
	for _, issue := range u.Issues {
		view.Usage.Issues = append(view.Usage.Issues, api.UsageIssue{Category: issue.Category, Code: issue.Code, Retryable: issue.Retryable, SourceRef: issue.SourceRef})
	}
	if priceErr != nil {
		view.CostReason = "PRICE_POLICY_INVALID"
		return view, nil
	}
	if activeErr != nil {
		view.CostReason = "PRICE_POLICY_NOT_CONFIGURED"
		return view, nil
	}
	if !cli.RunUsageEventsSupported {
		view.CostReason = "USAGE_DETAILS_UNSUPPORTED"
		return view, nil
	}
	events, err := cli.RunUsageEvents(ctx, u)
	if err != nil {
		view.CostReason = "COST_QUERY_FAILED"
		return view, nil
	}
	usage := billing.Usage{RunID: execution, Scope: u.Scope, Status: u.Status, Revision: u.Revision, Finalized: u.Coverage.Finalized, CoverageComplete: u.Coverage.ExpectedSources != nil && *u.Coverage.ExpectedSources == u.Coverage.VerifiedSources}
	for _, e := range events {
		if e.Decision != "counted" {
			continue
		}
		conflict := false
		for _, i := range e.MetadataIssues {
			if i.Category == "metadata" && i.Code == "SOURCE_METADATA_CONFLICT" {
				conflict = true
			}
		}
		usage.Records = append(usage.Records, billing.Record{ID: e.ID, Provider: e.Provider, Model: e.Model, MetadataBasis: e.MetadataBasis, TimeBasis: e.TimeBasis, CallTime: e.CallTime, Tokens: billing.Tokens(e.Tokens), MetadataConflict: conflict})
	}
	usage.ZeroKnown = u.Tokens != nil && u.Tokens.Total != nil && *u.Tokens.Total == 0
	if len(usage.Records) == 0 && (u.Tokens == nil || u.Tokens.Total == nil || *u.Tokens.Total != 0) {
		usage.CoverageComplete = false
	}
	estimate, err := billing.Calculate(usage, policy)
	if err != nil {
		view.CostReason = "COST_CALCULATION_FAILED"
		return view, nil
	}
	estimate.Generation = settings.Generation
	view.Cost = &estimate
	return view, nil
}
