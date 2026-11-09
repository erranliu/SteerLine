package executionconfig

import (
	"encoding/json"
	"errors"
)

// UsagePolicy controls collection for both local and verified remote sessions.
// The Runner derives the log access scope from the original Run binding.
type UsagePolicy struct {
	Enabled bool `json:"enabled"`
}

type UsageCollection struct {
	Status         string      `json:"status"`
	InstanceID     string      `json:"instance_id"`
	Policy         UsagePolicy `json:"usage_policy"`
	PolicyDigest   string      `json:"usage_policy_digest"`
	PolicyRevision string      `json:"usage_policy_revision"`
}

func DecodeUsagePolicy(raw json.RawMessage) (UsagePolicy, error) {
	p := UsagePolicy{Enabled: true}
	if len(raw) == 0 {
		return p, nil
	}
	var required map[string]json.RawMessage
	if json.Unmarshal(raw, &required) != nil || required["enabled"] == nil || string(required["enabled"]) == "null" {
		return p, errors.New("usage_policy requires enabled")
	}
	if err := Decode(raw, &p); err != nil {
		return p, err
	}
	return p, nil
}
