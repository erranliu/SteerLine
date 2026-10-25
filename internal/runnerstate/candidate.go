// Package runnerstate owns caller participation and CLI outbox values.
// Execution state is only a copy of the public Runner contract.
package runnerstate

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/taskcontract"
	"time"
)

type Action struct {
	CallerRequestID     string                 `json:"caller_request_id,omitempty"`
	CallerRequestDigest string                 `json:"caller_request_digest,omitempty"`
	ObserveOnly         bool                   `json:"observe_only,omitempty"`
	Command             string                 `json:"command"`
	Request             runnercontract.Request `json:"request"`
	State               string                 `json:"state"`
	CommandID           string                 `json:"command_id,omitempty"`
	ErrorCode           string                 `json:"error_code,omitempty"`
	ActionStatus        string                 `json:"action_status,omitempty"`
}

type Candidate struct {
	Contract          taskcontract.TaskContract `json:"contract"`
	MaterialDigest    string                    `json:"material_digest"`
	TestPlan          json.RawMessage           `json:"test_plan,omitempty"`
	Withdrawal        *Withdrawal               `json:"withdrawal,omitempty"`
	StartDigest       string                    `json:"start_digest"`
	ProfileID         string                    `json:"profile_id"`
	ProfileName       string                    `json:"profile_name"`
	ProfileDigest     string                    `json:"profile_digest"`
	RunnerBuild       runnercontract.BuildPin   `json:"runner_build"`
	CreatedAt         time.Time                 `json:"created_at"`
	Explicit          bool                      `json:"explicit"`
	Number            int                       `json:"number"`
	Project           string                    `json:"project"`
	TaskID            string                    `json:"task_id"`
	Claimant          string                    `json:"claimant"`
	ContractDigest    string                    `json:"contract_digest"`
	BaseRevision      string                    `json:"base_revision"`
	Participation     string                    `json:"participation"`
	ID                string                    `json:"id"`
	Revision          uint64                    `json:"revision"`
	StateDir          string                    `json:"state_dir"`
	Submit            runnercontract.Request    `json:"submit"`
	SubmitObserveOnly bool                      `json:"submit_observe_only,omitempty"`
	SubmitState       string                    `json:"submit_state"`
	SubmitCommandID   string                    `json:"submit_command_id,omitempty"`
	RunID             string                    `json:"run_id,omitempty"`
	InputDigest       string                    `json:"input_digest,omitempty"`
	Branch            string                    `json:"branch,omitempty"`
	PRURL             string                    `json:"pr_url,omitempty"`
	Cursor            string                    `json:"cursor,omitempty"`
	Projection        *runnercontract.Run       `json:"projection,omitempty"`
	StopRequested     bool                      `json:"stop_requested"`
	InputsRevoked     bool                      `json:"inputs_revoked"`
	Stop              *Action                   `json:"stop,omitempty"`
	Actions           []Action                  `json:"actions,omitempty"`
	Diagnostic        string                    `json:"diagnostic,omitempty"`
}

// Withdrawal is a caller participation fact, independent of native stop intent.
type Withdrawal struct {
	RequestID        string    `json:"request_id"`
	ExpectedRevision uint64    `json:"expected_revision"`
	RequestedBy      string    `json:"requested_by"`
	At               time.Time `json:"at"`
}

func (w *Withdrawal) Matches(requestID string, revision uint64, requestedBy string) bool {
	return w != nil && w.RequestID == requestID && w.ExpectedRevision == revision && w.RequestedBy == requestedBy
}
