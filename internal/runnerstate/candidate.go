// Package runnerstate owns caller participation and CLI outbox values.
// Execution state is only a copy of the public Runner contract.
package runnerstate

import (
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"time"
)

type Action struct {
	ObserveOnly  bool                   `json:"observe_only,omitempty"`
	Command      string                 `json:"command"`
	Request      runnercontract.Request `json:"request"`
	State        string                 `json:"state"`
	CommandID    string                 `json:"command_id,omitempty"`
	ErrorCode    string                 `json:"error_code,omitempty"`
	ActionStatus string                 `json:"action_status,omitempty"`
}

type Candidate struct {
	StartDigest     string                 `json:"start_digest"`
	ProfileID       string                 `json:"profile_id"`
	CreatedAt       time.Time              `json:"created_at"`
	Explicit        bool                   `json:"explicit"`
	Number          int                    `json:"number"`
	Project         string                 `json:"project"`
	TaskID          string                 `json:"task_id"`
	Claimant        string                 `json:"claimant"`
	ContractDigest  string                 `json:"contract_digest"`
	BaseRevision    string                 `json:"base_revision"`
	Participation   string                 `json:"participation"`
	ID              string                 `json:"id"`
	Revision        uint64                 `json:"revision"`
	StateDir        string                 `json:"state_dir"`
	Submit          runnercontract.Request `json:"submit"`
	SubmitState     string                 `json:"submit_state"`
	SubmitCommandID string                 `json:"submit_command_id,omitempty"`
	RunID           string                 `json:"run_id,omitempty"`
	InputDigest     string                 `json:"input_digest,omitempty"`
	Branch          string                 `json:"branch,omitempty"`
	PRURL           string                 `json:"pr_url,omitempty"`
	Cursor          string                 `json:"cursor,omitempty"`
	Projection      *runnercontract.Run    `json:"projection,omitempty"`
	StopRequested   bool                   `json:"stop_requested"`
	InputsRevoked   bool                   `json:"inputs_revoked"`
	Stop            *Action                `json:"stop,omitempty"`
	Actions         []Action               `json:"actions,omitempty"`
	Diagnostic      string                 `json:"diagnostic,omitempty"`
}
