package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"time"

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
		if err := validateUsageCounters(*v.Tokens); err != nil {
			return v, err
		}
		for _, n := range []*int64{v.Tokens.InputTotal, v.Tokens.InputUncached, v.Tokens.CacheRead, v.Tokens.CacheWrite, v.Tokens.Output, v.Tokens.Reasoning, v.Tokens.Total} {
			if n != nil && *n < 0 {
				return v, errors.New("Runner usage has negative tokens")
			}
		}
	}
	return v, nil
}

// RunUsageEvents reads a bounded complete immutable event set, never latest.
func (c Client) RunUsageEvents(ctx context.Context, usage runnercontract.UsageSnapshot) ([]runnercontract.UsageEvent, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var events []runnercontract.UsageEvent
	cursor := ""
	seenCursor := map[string]bool{}
	seenID := map[string]bool{}
	counted := int64(0)
	bytes := 0
	for {
		args := []string{"--run-id", usage.RunID, "--snapshot-revision", strconv.FormatUint(usage.Revision, 10), "--limit", "200"}
		if cursor != "" {
			args = append(args, "--after", cursor)
		}
		response, err := c.Call(ctx, "run-usage-events", args, nil)
		if err != nil {
			return nil, err
		}
		bytes += len(response.Data)
		if bytes > 16<<20 {
			return nil, errors.New("usage event byte budget exceeded")
		}
		var page runnercontract.UsagePage
		if json.Unmarshal(response.Data, &page) != nil || page.RunID != usage.RunID || page.Revision != usage.Revision || len(page.Events) > 200 {
			return nil, errors.New("usage page differs from fixed snapshot")
		}
		for _, e := range page.Events {
			if err := validateUsageCounters(e.Tokens); err != nil {
				return nil, err
			}
			if e.RunID != usage.RunID || e.ID == "" || seenID[e.ID] {
				return nil, errors.New("invalid or duplicate usage event")
			}
			seenID[e.ID] = true
			for _, n := range []*int64{e.Tokens.InputTotal, e.Tokens.InputUncached, e.Tokens.CacheRead, e.Tokens.CacheWrite, e.Tokens.Output, e.Tokens.Reasoning, e.Tokens.Total} {
				if n != nil && *n < 0 {
					return nil, errors.New("negative event tokens")
				}
			}
			if e.Decision == "counted" && (e.Scope == "" || e.Scope == "main_session") {
				counted++
			}
			events = append(events, e)
		}
		if len(events) > 50000 {
			return nil, errors.New("usage record budget exceeded")
		}
		if page.NextCursor == "" {
			break
		}
		if seenCursor[page.NextCursor] || len(page.Events) == 0 {
			return nil, errors.New("nonterminating usage cursor")
		}
		seenCursor[page.NextCursor] = true
		cursor = page.NextCursor
	}
	if counted != usage.Coverage.CountedResponses {
		return nil, errors.New("usage summary and counted events disagree")
	}
	if err := validateInternalUsage(usage.ModelCallCoverage, events); err != nil {
		return nil, err
	}
	if usage.Tokens != nil {
		summary := counterFields(*usage.Tokens)
		for i, want := range summary {
			sum := new(big.Int)
			known := true
			for _, e := range events {
				if e.Decision != "counted" || e.Scope != "" && e.Scope != "main_session" {
					continue
				}
				n := counterFields(e.Tokens)[i]
				if n == nil {
					known = false
				} else {
					sum.Add(sum, big.NewInt(*n))
				}
			}
			if known {
				if !sum.IsInt64() || want == nil || *want != sum.Int64() {
					return nil, errors.New("usage summary differs from event counters")
				}
			} else if want != nil {
				return nil, errors.New("unknown event counter became a known summary")
			}
		}
	}
	return events, nil
}

func validateInternalUsage(coverage *runnercontract.ModelCallCoverage, events []runnercontract.UsageEvent) error {
	var recorded, covered, uncovered, pending int64
	seen := map[string]bool{}
	for _, event := range events {
		if event.Scope == "" || event.Scope == "main_session" {
			continue
		}
		if event.Scope != "runner_internal" || event.CallID == "" || seen[event.CallID] || event.CallSource != "llm_parser" || event.CommandID == "" || event.SourceResultID == "" {
			return errors.New("unknown or unbound internal usage source")
		}
		seen[event.CallID] = true
		recorded++
		switch event.CoverageStatus {
		case "covered":
			if event.Decision != "counted" || event.ConfiguredModel == "" {
				return errors.New("covered internal call lacks counted model attribution")
			}
			covered++
		case "pending", "uncovered":
			if event.Decision == "counted" {
				return errors.New("uncovered internal call was counted")
			}
			for _, value := range counterFields(event.Tokens) {
				if value != nil {
					return errors.New("uncovered internal call has tokens")
				}
			}
			if event.CoverageStatus == "pending" {
				pending++
			} else {
				uncovered++
			}
		default:
			return errors.New("unknown internal coverage status")
		}
	}
	if coverage == nil {
		if recorded != 0 {
			return errors.New("internal model coverage is missing")
		}
		return nil
	}
	if coverage.InternalRecorded != recorded || coverage.InternalCovered != covered || coverage.InternalUncovered != uncovered || coverage.InternalPending != pending {
		return errors.New("internal model coverage differs from fixed events")
	}
	return nil
}

func counterFields(t runnercontract.UsageTokens) []*int64 {
	return []*int64{t.InputTotal, t.InputUncached, t.CacheRead, t.CacheWrite, t.Output, t.Reasoning, t.Total}
}
func validateUsageCounters(t runnercontract.UsageTokens) error {
	for _, n := range counterFields(t) {
		if n != nil && *n < 0 {
			return errors.New("negative usage counter")
		}
	}
	check := func(total *int64, parts ...*int64) error {
		if total == nil {
			return nil
		}
		sum := new(big.Int)
		for _, n := range parts {
			if n == nil {
				return nil
			}
			sum.Add(sum, big.NewInt(*n))
		}
		if !sum.IsInt64() || sum.Int64() != *total {
			return errors.New("inconsistent usage token categories")
		}
		return nil
	}
	if err := check(t.InputTotal, t.InputUncached, t.CacheRead, t.CacheWrite); err != nil {
		return err
	}
	if err := check(t.Total, t.InputTotal, t.Output); err != nil {
		return err
	}
	if t.Reasoning != nil && t.Output != nil && *t.Reasoning > *t.Output {
		return errors.New("reasoning exceeds output")
	}
	return nil
}
