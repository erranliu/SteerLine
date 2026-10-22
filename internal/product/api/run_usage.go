package api

import "time"

// Read-only telemetry; it grants no execution or recovery authority.
type RunUsageView struct {
	RunID       string        `json:"run_id"`
	ExecutionID string        `json:"execution_id"`
	Usage       *UsageSummary `json:"usage"`
	Cost        *CostSummary  `json:"cost"`
	Reason      string        `json:"reason,omitempty"`
	CostReason  string        `json:"cost_reason,omitempty"`
}
type UsageTokens struct {
	InputTotal    *int64 `json:"input_total"`
	InputUncached *int64 `json:"input_uncached"`
	CacheRead     *int64 `json:"cache_read"`
	CacheWrite    *int64 `json:"cache_write"`
	Output        *int64 `json:"output"`
	Reasoning     *int64 `json:"reasoning"`
	Total         *int64 `json:"total"`
}
type UsageSummary struct {
	Revision            uint64       `json:"snapshot_revision"`
	Status              string       `json:"status"`
	Scope               string       `json:"scope"`
	Tokens              *UsageTokens `json:"tokens"`
	Finalized           bool         `json:"finalized"`
	CountedResponses    int64        `json:"counted_responses"`
	Issues              []string     `json:"issues"`
	AccessCleanupStatus string       `json:"access_cleanup_status"`
	ObservedAt          time.Time    `json:"observed_at"`
}
type CostSummary struct {
	Status        string   `json:"status"`
	UsageRevision uint64   `json:"usage_revision"`
	PolicyID      string   `json:"pricing_policy"`
	PolicyVersion string   `json:"pricing_policy_version"`
	PolicyDigest  string   `json:"pricing_policy_digest"`
	Currency      string   `json:"currency"`
	Covered       *string  `json:"covered_estimated_amount"`
	Total         *string  `json:"total_estimated_amount"`
	Priced        int64    `json:"priced_responses"`
	Unpriced      int64    `json:"unpriced_responses"`
	Assumptions   []string `json:"assumptions"`
	Issues        []string `json:"issues"`
}
