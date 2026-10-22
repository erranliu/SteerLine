package runnerclient

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// VerifyTakeover may run against the existing service or offline writer lease.
// It never starts a service and does not replace current-instance verification.
func (c Client) VerifyTakeover(ctx context.Context) (runnercontract.TakeoverReport, error) {
	response, err := c.Call(ctx, "service verify-takeover", []string{"--candidate", c.Executable}, nil)
	if err != nil {
		return runnercontract.TakeoverReport{}, err
	}
	var report runnercontract.TakeoverReport
	if json.Unmarshal(response.Data, &report) != nil {
		return report, errors.New("Runner candidate takeover is unverified or incompatible")
	}
	if err := report.Validate(c.StateDir, c.SHA256); err != nil {
		return report, err
	}
	return report, nil
}

// Stops are the caller's already durable envelopes. Runner commits them before
// starting workers; an unknown reply must retain exactly these original inputs.
func (c Client) StartServiceWithStops(ctx context.Context, version Version, stops []runnercontract.PendingStop) (ServiceInfo, error) {
	if len(stops) == 0 {
		return c.StartService(ctx, version)
	}
	if err := (runnercontract.ServiceStartInput{PendingStops: stops}).Validate(); err != nil {
		return ServiceInfo{}, err
	}
	if current, err := c.ServiceStatus(ctx); err == nil && current.Status == "ready" {
		if err := c.validateStartedService(current, version); err != nil {
			return current, err
		}
		if len(c.UsagePolicy) > 0 {
			if _, err := c.StartService(ctx, version); err != nil {
				return current, err
			}
		}
		bound := c
		bound.InstanceID = current.InstanceID
		for _, stop := range stops {
			if _, err := bound.Call(ctx, "stop", []string{"--run-id", stop.RunID}, &stop.Request); err != nil {
				return current, err
			}
		}
		return current, nil
	}
	response, err := c.call(ctx, "service start", nil, nil, &runnercontract.ServiceStartInput{PendingStops: stops, UsagePolicy: c.UsagePolicy})
	var info ServiceInfo
	if err != nil {
		return info, err
	}
	if json.Unmarshal(response.Data, &info) != nil {
		return info, errors.New("Runner service identity is unavailable")
	}
	return info, c.validateStartedService(info, version)
}
