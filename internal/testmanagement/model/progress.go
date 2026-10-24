package model

import (
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"time"
)

// Progress retains the latest Runner snapshot and the last valid business map.
// It is independent of candidate/business revisions and standalone test records.
type Progress struct {
	ParsedAt      *time.Time                  `json:"parsed_at,omitempty"`
	Project       string                      `json:"project"`
	CandidateID   string                      `json:"candidate_id"`
	RunID         string                      `json:"run_id"`
	InputDigest   string                      `json:"input_digest"`
	Revision      uint64                      `json:"revision"`
	Snapshot      runnercontract.ProgressFile `json:"snapshot"`
	Completed     map[string]bool             `json:"completed"`
	ParsedVersion uint64                      `json:"parsed_version"`
	Diagnostic    string                      `json:"diagnostic,omitempty"`
}
