package api

// These are read-only product values. They contain no native bindings or
// mutable execution settings and cannot authorize the next workflow step.
type RunnerWorkflowSummary struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

type RunnerMaterialSummary struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Commit string `json:"commit,omitempty"`
	Digest string `json:"digest"`
}

type RunnerAuditSummary struct {
	Model           string              `json:"model"`
	ReasoningEffort string              `json:"reasoning_effort"`
	Results         []RunnerAuditResult `json:"results"`
}
type RunnerAuditResult struct {
	Step                 string                 `json:"step"`
	SourceResultID       string                 `json:"source_result_id"`
	ImplementationSource RunnerAuditSource      `json:"implementation_source"`
	ResultID             string                 `json:"result_id"`
	CommandID            string                 `json:"command_id"`
	TurnID               string                 `json:"turn_id"`
	PreHead              string                 `json:"pre_head,omitempty"`
	PostHead             string                 `json:"post_head,omitempty"`
	FixedCount           *int                   `json:"fixed_count,omitempty"`
	FixedIssues          []RunnerFixedIssue     `json:"fixed_issues"`
	RemainingIssues      []RunnerRemainingIssue `json:"remaining_issues"`
	Failure              string                 `json:"failure,omitempty"`
}
type RunnerAuditSource struct {
	SessionID    string `json:"session_id,omitempty"`
	CommandID    string `json:"command_id"`
	TurnID       string `json:"turn_id"`
	AttemptID    string `json:"attempt_id"`
	InputVersion uint64 `json:"input_version"`
	FinalDigest  string `json:"final_digest"`
}
type RunnerFixedIssue struct {
	Severity string `json:"severity"`
	Issue    string `json:"issue"`
	Repair   string `json:"repair"`
	Evidence string `json:"evidence"`
}
type RunnerRemainingIssue struct {
	Severity string `json:"severity"`
	Issue    string `json:"issue"`
	Reason   string `json:"reason"`
}
