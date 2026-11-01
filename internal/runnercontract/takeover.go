package runnercontract

import (
	"bytes"
	"encoding/json"
	"errors"
)

type PendingStop struct {
	RunID   string  `json:"run_id"`
	Request Request `json:"request"`
}

type ServiceStartInput struct {
	PendingStops []PendingStop   `json:"pending_stops"`
	UsagePolicy  json.RawMessage `json:"usage_policy,omitempty"`
}

func (s ServiceStartInput) Validate() error {
	if len(s.UsagePolicy) > 0 && (!json.Valid(s.UsagePolicy) || !bytes.HasPrefix(bytes.TrimSpace(s.UsagePolicy), []byte("{"))) {
		return errors.New("usage policy must be a public Runner policy object")
	}
	if s.PendingStops == nil || len(s.PendingStops) > 500 {
		return errors.New("original pending stops array is required")
	}
	seen := map[string]bool{}
	for _, stop := range s.PendingStops {
		var revision struct {
			ExpectedRevision uint64 `json:"expected_revision"`
		}
		if stop.RunID == "" || stop.Request.Protocol != "steerrunner-cli/v1" || stop.Request.RequestID == "" || seen[stop.Request.RequestID] || json.Unmarshal(stop.Request.Input, &revision) != nil || revision.ExpectedRevision == 0 {
			return errors.New("original stop identity/revision is required")
		}
		seen[stop.Request.RequestID] = true
	}
	return nil
}
