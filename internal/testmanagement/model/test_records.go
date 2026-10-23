package model

import (
	"time"
)

type Result struct {
	TestID  string `json:"test_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Log     string `json:"log"`
}

// Report is produced by the test program. Run provenance must come from Runner,
// never from this document or an agent's final message.
type Report struct {
	StartedAt    time.Time         `json:"started_at"`
	FinishedAt   time.Time         `json:"finished_at"`
	Definition   Definition        `json:"definition"`
	Command      string            `json:"command"`
	Branch       string            `json:"branch"`
	Repository   string            `json:"repository"`
	Evidence     ReportEvidence    `json:"evidence"`
	Scope        *Scope            `json:"scope,omitempty"`
	ID           string            `json:"id"`
	Sequence     uint64            `json:"sequence"`
	Selection    []string          `json:"selection"`
	Environment  string            `json:"environment"`
	Definitions  map[string]string `json:"definitions"`
	BeforeCommit string            `json:"before_commit"`
	AfterCommit  string            `json:"after_commit"`
	BeforeClean  bool              `json:"before_clean"`
	AfterClean   bool              `json:"after_clean"`
	Results      []Result          `json:"results"`
}

const MaxReportBytes = 128 << 10

type ScopeResult struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}
type Scope struct {
	Complete bool          `json:"complete"`
	Items    []ScopeResult `json:"items"`
}
type ReportEvidence struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256,omitempty"`
	Bytes      int    `json:"bytes"`
	Diagnostic string `json:"diagnostic,omitempty"`
}
type RecordFilter struct {
	TestID      string `json:"test_id"`
	Search      string `json:"search"`
	Level       string `json:"level"`
	Outcome     string `json:"outcome"`
	Environment string `json:"environment"`
	Source      string `json:"source"`
	From        string `json:"from"`
	To          string `json:"to"`
	Cursor      string `json:"cursor"`
	Limit       int    `json:"limit"`
}
