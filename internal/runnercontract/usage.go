package runnercontract

import "time"

// Usage and cost are observations, independent of Run business revisions.
type UsageTokens struct {
	InputTotal    *int64 `json:"input_total"`
	InputUncached *int64 `json:"input_uncached"`
	CacheRead     *int64 `json:"cache_read"`
	CacheWrite    *int64 `json:"cache_write"`
	Output        *int64 `json:"output"`
	Reasoning     *int64 `json:"reasoning"`
	Total         *int64 `json:"total"`
}
type UsageIssue struct {
	Category  string `json:"category"`
	Code      string `json:"code"`
	Retryable bool   `json:"retryable"`
}
type UsageSnapshot struct {
	Schema   string       `json:"schema"`
	RunID    string       `json:"run_id"`
	Revision uint64       `json:"snapshot_revision"`
	Status   string       `json:"status"`
	Scope    string       `json:"scope"`
	Tokens   *UsageTokens `json:"tokens"`
	Coverage struct {
		ExpectedSources  *int64 `json:"expected_sources"`
		VerifiedSources  int64  `json:"verified_sources"`
		CountedResponses int64  `json:"counted_responses"`
		Finalized        bool   `json:"finalized"`
	} `json:"coverage"`
	Issues              []UsageIssue `json:"issues"`
	AccessCleanupStatus string       `json:"access_cleanup_status"`
	ObservedAt          time.Time    `json:"observed_at"`
}
type CostEstimate struct {
	Schema        string   `json:"schema"`
	RunID         string   `json:"run_id"`
	Status        string   `json:"status"`
	UsageRevision uint64   `json:"usage_revision"`
	Scope         string   `json:"scope"`
	UsageStatus   string   `json:"usage_status"`
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
	Basis         string   `json:"basis"`
}
