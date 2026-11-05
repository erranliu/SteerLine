package api

import (
	"github.com/erranliu/SteerLine/internal/billing"
	"time"
)

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
	Issues              []UsageIssue `json:"issues"`
	AccessCleanupStatus string       `json:"access_cleanup_status"`
	ObservedAt          time.Time    `json:"observed_at"`
}
type CostSummary = billing.Estimate
type UsageIssue struct {
	Category  string `json:"category"`
	Code      string `json:"code"`
	Retryable bool   `json:"retryable"`
	SourceRef string `json:"source_ref,omitempty"`
}
