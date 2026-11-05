package executionconfig

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

type UsageGrant struct {
	Environment  string   `json:"environment"`
	HostID       string   `json:"host_id"`
	SessionRoots []string `json:"session_roots"`
}
type UsagePolicy struct {
	Enabled         bool `json:"enabled"`
	RemoteLogAccess struct {
		Mode   string       `json:"mode"`
		Grants []UsageGrant `json:"grants"`
	} `json:"remote_log_access"`
}
type UsageRequirement struct {
	RunID       string    `json:"run_id"`
	SourceRef   string    `json:"source_ref"`
	Environment string    `json:"environment"`
	HostID      string    `json:"host_id"`
	SessionRoot string    `json:"session_root"`
	ObservedAt  time.Time `json:"observed_at"`
}
type UsageCollection struct {
	Status         string             `json:"status"`
	InstanceID     string             `json:"instance_id"`
	Policy         UsagePolicy        `json:"usage_policy"`
	PolicyDigest   string             `json:"usage_policy_digest"`
	PolicyRevision string             `json:"usage_policy_revision"`
	Requirements   []UsageRequirement `json:"requirements"`
	Diagnostic     string             `json:"diagnostic,omitempty"`
}

var usageHost = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var usageLeaf = regexp.MustCompile(`(?i)^[a-z]:[/\\].*[/\\]sessions[/\\][0-9]{4}[/\\][0-9]{2}[/\\][0-9]{2}$`)

func SafeUsageRoot(root string) bool {
	return len(root) <= 2048 && usageLeaf.MatchString(root) && !strings.ContainsAny(root, "*?\x00\r\n") && !strings.Contains(strings.ReplaceAll(root, "\\", "/"), "/../") && !strings.Contains(strings.ReplaceAll(root, "\\", "/"), "/./")
}
func DecodeUsagePolicy(raw json.RawMessage) (UsagePolicy, error) {
	var p UsagePolicy
	if len(raw) == 0 {
		p.Enabled = true
		p.RemoteLogAccess.Mode = "disabled"
		p.RemoteLogAccess.Grants = []UsageGrant{}
		return p, nil
	}
	var required map[string]json.RawMessage
	if json.Unmarshal(raw, &required) != nil || required["enabled"] == nil || string(required["enabled"]) == "null" || required["remote_log_access"] == nil {
		return p, errors.New("usage_policy requires enabled and remote_log_access")
	}
	if err := Decode(raw, &p); err != nil {
		return p, err
	}
	if p.RemoteLogAccess.Mode != "disabled" && p.RemoteLogAccess.Mode != "temporary" || p.RemoteLogAccess.Grants == nil || p.RemoteLogAccess.Mode == "disabled" && len(p.RemoteLogAccess.Grants) > 0 {
		return p, errors.New("invalid remote usage mode or grants")
	}
	seen := map[string]bool{}
	for _, g := range p.RemoteLogAccess.Grants {
		if g.Environment == "" || !usageHost.MatchString(g.HostID) || len(g.SessionRoots) == 0 {
			return p, errors.New("verified environment, host fingerprint and session roots required")
		}
		for _, r := range g.SessionRoots {
			key := g.Environment + g.HostID + strings.ToLower(strings.ReplaceAll(r, "\\", "/"))
			if !SafeUsageRoot(r) || seen[key] {
				return p, errors.New("unsafe or duplicate usage session leaf")
			}
			seen[key] = true
		}
	}
	return p, nil
}
