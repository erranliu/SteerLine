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
	RepairCount     uint64              `json:"repair_count"`
	Results         []RunnerAuditResult `json:"results"`
}

type RunnerAuditResult struct {
	Step                  string          `json:"step"`
	SourceReportCommandID string          `json:"source_report_command_id"`
	CommandID             string          `json:"command_id"`
	TurnID                string          `json:"turn_id"`
	Verdict               string          `json:"verdict,omitempty"`
	Findings              []RunnerFinding `json:"findings,omitempty"`
	Failure               string          `json:"failure,omitempty"`
}

type RunnerFinding struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
}
