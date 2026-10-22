package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func (c Client) RunUsage(ctx context.Context, run string) (runnercontract.UsageSnapshot, error) {
	var v runnercontract.UsageSnapshot
	response, err := c.Call(ctx, "run-usage", []string{"--run-id", run}, nil)
	if err != nil {
		return v, err
	}
	if json.Unmarshal(response.Data, &v) != nil || v.Schema != "run-usage/v1" || v.RunID != run || v.Revision == 0 || v.Scope != "main_session" || v.ObservedAt.IsZero() {
		return v, errors.New("Runner usage identity or snapshot is invalid")
	}
	switch v.Status {
	case "pending", "available", "partial", "unavailable":
	default:
		return v, errors.New("Runner usage status is invalid")
	}
	if v.Coverage.VerifiedSources < 0 || v.Coverage.CountedResponses < 0 || v.Coverage.ExpectedSources != nil && *v.Coverage.ExpectedSources < 0 {
		return v, errors.New("Runner usage coverage is invalid")
	}
	if v.Tokens != nil {
		for _, n := range []*int64{v.Tokens.InputTotal, v.Tokens.InputUncached, v.Tokens.CacheRead, v.Tokens.CacheWrite, v.Tokens.Output, v.Tokens.Reasoning, v.Tokens.Total} {
			if n != nil && *n < 0 {
				return v, errors.New("Runner usage has negative tokens")
			}
		}
	}
	return v, nil
}

var costDecimal = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
var costCurrency = regexp.MustCompile(`^[A-Z]{3}$`)

// The estimator must consume exactly the snapshot just read; never recalculate latest.
func (c Client) EstimateRunCost(ctx context.Context, usage runnercontract.UsageSnapshot, policy string) (runnercontract.CostEstimate, error) {
	var v runnercontract.CostEstimate
	if usage.RunID == "" || usage.Revision == 0 || !filepath.IsAbs(policy) {
		return v, errors.New("cost query requires a fixed usage snapshot and absolute price file")
	}
	response, err := c.Call(ctx, "estimate-run-cost", []string{"--run-id", usage.RunID, "--usage-revision", strconv.FormatUint(usage.Revision, 10), "--pricing-policy", policy}, nil)
	if err != nil {
		return v, err
	}
	if json.Unmarshal(response.Data, &v) != nil || v.Schema != "run-cost-estimate/v1" || v.RunID != usage.RunID || v.UsageRevision != usage.Revision || v.Scope != usage.Scope || v.UsageStatus != usage.Status || v.Basis != "api_equivalent" || v.Priced < 0 || v.Unpriced < 0 {
		return v, errors.New("Runner cost differs from the fixed usage snapshot")
	}
	for _, amount := range []*string{v.Total, v.Covered} {
		if amount != nil && (len(*amount) > 128 || !costDecimal.MatchString(*amount) || !costCurrency.MatchString(v.Currency)) {
			return v, errors.New("Runner cost amount or currency is invalid")
		}
	}
	switch v.Status {
	case "estimated":
		if v.Total == nil || v.Covered == nil || usage.Status != "available" || !usage.Coverage.Finalized || usage.Coverage.ExpectedSources == nil || *usage.Coverage.ExpectedSources != usage.Coverage.VerifiedSources || v.Unpriced != 0 || v.Priced != usage.Coverage.CountedResponses {
			return v, errors.New("Runner total cost coverage is incomplete")
		}
	case "partial":
		if v.Total != nil || v.Covered == nil || v.Priced == 0 || v.Priced != usage.Coverage.CountedResponses-v.Unpriced {
			return v, errors.New("Runner partial cost is invalid")
		}
	case "unavailable":
		if v.Total != nil || v.Covered != nil {
			return v, errors.New("Runner unavailable cost has an amount")
		}
	default:
		return v, errors.New("Runner cost status is invalid")
	}
	return v, nil
}
