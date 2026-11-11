package runnercontract

import "time"

// Usage facts are observations, independent of Run business revisions.
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
	SourceRef string `json:"source_ref,omitempty"`
}
type UsageSnapshot struct {
	ModelCallCoverage *ModelCallCoverage `json:"model_call_coverage,omitempty"`
	Schema            string             `json:"schema"`
	RunID             string             `json:"run_id"`
	Revision          uint64             `json:"snapshot_revision"`
	Status            string             `json:"status"`
	Scope             string             `json:"scope"`
	Tokens            *UsageTokens       `json:"tokens"`
	Coverage          struct {
		ExpectedSources  *int64 `json:"expected_sources"`
		VerifiedSources  int64  `json:"verified_sources"`
		CountedResponses int64  `json:"counted_responses"`
		Finalized        bool   `json:"finalized"`
	} `json:"coverage"`
	Issues              []UsageIssue `json:"issues"`
	AccessCleanupStatus string       `json:"access_cleanup_status"`
	ObservedAt          time.Time    `json:"observed_at"`
}

type ModelCallCoverage struct {
	InternalRecorded  int64    `json:"internal_recorded"`
	InternalCovered   int64    `json:"internal_covered"`
	InternalUncovered int64    `json:"internal_uncovered"`
	InternalPending   int64    `json:"internal_pending"`
	Untracked         []string `json:"untracked"`
}
type UsageEvent struct {
	Scope                     string       `json:"scope,omitempty"`
	CallID                    string       `json:"call_id,omitempty"`
	CallSource                string       `json:"call_source,omitempty"`
	SourceResultID            string       `json:"source_result_id,omitempty"`
	CoverageStatus            string       `json:"coverage_status,omitempty"`
	CommandID                 string       `json:"command_id,omitempty"`
	AttemptID                 string       `json:"attempt_id,omitempty"`
	StepID                    string       `json:"step_id"`
	ConfiguredModel           string       `json:"configured_model"`
	ConfiguredReasoningEffort string       `json:"configured_reasoning_effort"`
	ID                        string       `json:"event_id"`
	SourceRef                 string       `json:"source_ref"`
	RunID                     string       `json:"run_id"`
	Decision                  string       `json:"decision"`
	Provider                  *string      `json:"provider"`
	Model                     *string      `json:"model"`
	MetadataBasis             string       `json:"metadata_basis"`
	TimeBasis                 string       `json:"time_basis"`
	CallTime                  *time.Time   `json:"call_time"`
	Tokens                    UsageTokens  `json:"tokens"`
	MetadataIssues            []UsageIssue `json:"metadata_issues"`
}
type UsagePage struct {
	RunID      string       `json:"run_id"`
	Revision   uint64       `json:"snapshot_revision"`
	Events     []UsageEvent `json:"events"`
	NextCursor string       `json:"next_cursor"`
}
