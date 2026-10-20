package model

import (
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type Collection struct {
	Path       string                 `json:"path"`
	Request    runnercontract.Request `json:"request"`
	State      string                 `json:"state"`
	Diagnostic string                 `json:"diagnostic,omitempty"`
}
type EvidenceArtifact struct {
	Metadata runnercontract.Artifact `json:"metadata"`
	Content  []byte                  `json:"content"`
}
type Evidence struct {
	Project     string                      `json:"project"`
	CandidateID string                      `json:"candidate_id"`
	RunID       string                      `json:"run_id"`
	InputDigest string                      `json:"input_digest"`
	Revision    uint64                      `json:"revision"`
	Collections []Collection                `json:"collections"`
	Artifacts   []EvidenceArtifact          `json:"artifacts"`
	Git         runnercontract.WorkspaceGit `json:"git"`
	Diagnostic  string                      `json:"diagnostic,omitempty"`
}
