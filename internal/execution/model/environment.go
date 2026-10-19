package model

import (
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/hostcontract"
)

type WorkerEnvironment struct {
	OriginalTerminal *hostcontract.ProcessIdentity `json:"original_terminal,omitempty"`
	ProfileDigest    string                        `json:"profile_digest"`
	LaunchCommand    string                        `json:"launch_command,omitempty"`
	WriteRoots       []string                      `json:"write_roots,omitempty"`
	Runtime          string                        `json:"runtime,omitempty"`
	Preparation      *WorkerPreparation            `json:"preparation"`
	History          []WorkerPreparation           `json:"history,omitempty"`
	Blocker          *WorkerHostBlocker            `json:"blocker,omitempty"`
	Ready            bool                          `json:"ready"`
}

type WorkerPreparation struct {
	TurnID       string                             `json:"turn_id"`
	Nonce        string                             `json:"nonce"`
	StartedAt    time.Time                          `json:"started_at"`
	Deadline     time.Time                          `json:"deadline"`
	Probe        executionbackend.ProbeIdentity     `json:"probe"`
	Command      string                             `json:"command,omitempty"`
	InputDigest  string                             `json:"input_digest,omitempty"`
	ScriptDigest string                             `json:"script_digest,omitempty"`
	Receipt      *executionbackend.ProbeReceipt     `json:"receipt,omitempty"`
	Evidence     executionbackend.ProbeToolEvidence `json:"evidence"`
	Verified     bool                               `json:"verified"`
	Invalidated  bool                               `json:"invalidated,omitempty"`
}

type WorkerHostBlocker struct {
	ID                 string                    `json:"id"`
	Fact               executionbackend.HostFact `json:"fact"`
	ObservedAt         time.Time                 `json:"observed_at"`
	TurnID             string                    `json:"turn_id"`
	Version            uint64                    `json:"version"`
	RecheckOperationID string                    `json:"recheck_operation_id,omitempty"`
}
