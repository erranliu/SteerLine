package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

// These values are public Runner CLI contracts, not native lifecycle state.
type PendingStop struct {
	RunID   string  `json:"run_id"`
	Request Request `json:"request"`
}
type ServiceStartInput struct {
	PendingStops []PendingStop `json:"pending_stops"`
}
type TakeoverItem struct {
	RequestID     string `json:"request_id,omitempty"`
	RequestDigest string `json:"request_digest,omitempty"`
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Revision      uint64 `json:"revision"`
	InputDigest   string `json:"input_digest"`
	Status        string `json:"status"`
	Diagnostic    string `json:"diagnostic,omitempty"`
}
type TakeoverReport struct {
	Contract        string         `json:"contract"`
	StateDir        string         `json:"state_dir"`
	InstanceID      string         `json:"instance_id,omitempty"`
	CandidateSHA256 string         `json:"candidate_sha256"`
	Compatible      bool           `json:"compatible"`
	Items           []TakeoverItem `json:"items"`
}

func (s ServiceStartInput) validate() error {
	if s.PendingStops == nil || len(s.PendingStops) > 500 {
		return errors.New("original pending stops array is required")
	}
	seen := map[string]bool{}
	for _, stop := range s.PendingStops {
		var revision struct {
			ExpectedRevision uint64 `json:"expected_revision"`
		}
		if stop.RunID == "" || stop.Request.Protocol != Protocol || stop.Request.RequestID == "" || seen[stop.Request.RequestID] || json.Unmarshal(stop.Request.Input, &revision) != nil || revision.ExpectedRevision == 0 {
			return errors.New("original stop identity/revision is required")
		}
		seen[stop.Request.RequestID] = true
	}
	return nil
}

// VerifyTakeover may run against the existing service or offline writer lease.
// It never starts a service and does not replace current-instance verification.
func (c Client) VerifyTakeover(ctx context.Context) (TakeoverReport, error) {
	response, err := c.Call(ctx, "service verify-takeover", []string{"--candidate", c.Executable}, nil)
	if err != nil {
		return TakeoverReport{}, err
	}
	var report TakeoverReport
	if json.Unmarshal(response.Data, &report) != nil || report.Contract != "steerrunner-takeover/v1" || !strings.EqualFold(filepath.Clean(report.StateDir), filepath.Clean(c.StateDir)) || !strings.EqualFold(report.CandidateSHA256, c.SHA256) || !report.Compatible || report.Items == nil {
		return report, errors.New("Runner candidate takeover is unverified or incompatible")
	}
	seen := map[string]bool{}
	for _, item := range report.Items {
		key := item.Kind + ":" + item.ID
		if item.ID == "" || seen[key] || item.Status != "compatible" && item.Status != "read_only" {
			return report, errors.New("Runner takeover contains unsupported or duplicate state")
		}
		seen[key] = true
	}
	return report, nil
}

// Stops are the caller's already durable envelopes. Runner commits them before
// starting workers; an unknown reply must retain exactly these original inputs.
func (c Client) StartServiceWithStops(ctx context.Context, version Version, stops []PendingStop) (ServiceInfo, error) {
	if len(stops) == 0 {
		return c.StartService(ctx, version)
	}
	if err := (ServiceStartInput{PendingStops: stops}).validate(); err != nil {
		return ServiceInfo{}, err
	}
	if current, err := c.ServiceStatus(ctx); err == nil && current.Status == "ready" {
		if err := c.validateStartedService(current, version); err != nil {
			return current, err
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
	response, err := c.call(ctx, "service start", nil, nil, &ServiceStartInput{PendingStops: stops})
	var info ServiceInfo
	if err != nil {
		return info, err
	}
	if json.Unmarshal(response.Data, &info) != nil {
		return info, errors.New("Runner service identity is unavailable")
	}
	return info, c.validateStartedService(info, version)
}
