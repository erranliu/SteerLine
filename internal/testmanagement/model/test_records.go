package model

import (
	"encoding/json"
	"errors"
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
	RerunOf      string            `json:"rerun_of,omitempty"`
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
const MaxSummaryBytes = 512 << 10

var ErrInvalidRecord = errors.New("invalid test record")
var ErrRecordConflict = errors.New("test record identity conflict")

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
type RecordSource struct {
	Kind        string `json:"kind"`
	TaskID      string `json:"task_id,omitempty"`
	CandidateID string `json:"candidate_id,omitempty"`
	RunID       string `json:"run_id,omitempty"`
}
type TestRecord struct {
	Project        string       `json:"-"`
	Seq            int64        `json:"seq"`
	Report         Report       `json:"report"`
	EvidenceStatus string       `json:"evidence_status"`
	Diagnostic     string       `json:"diagnostic,omitempty"`
	Source         RecordSource `json:"source"`
	Summary        []byte       `json:"-"`
	ObservedAt     time.Time    `json:"observed_at"`
}
type RegisterRecord struct {
	Report     json.RawMessage `json:"report"`
	Summary    []byte          `json:"summary,omitempty"`
	Diagnostic string          `json:"diagnostic,omitempty"`
}
type RecordFilter struct {
	TestID      string `json:"test_id"`
	Search      string `json:"search"`
	Level       string `json:"level"`
	Outcome     string `json:"outcome"`
	Retest      string `json:"retest"`
	Environment string `json:"environment"`
	Source      string `json:"source"`
	From        string `json:"from"`
	To          string `json:"to"`
	Cursor      string `json:"cursor"`
	Limit       int    `json:"limit"`
}
