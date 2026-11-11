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
	RejectionDiagnostic string                 `json:"rejection_diagnostic,omitempty"`
	ActionStatus        string                 `json:"action_status,omitempty"`
}

// EnvironmentAdmission is the caller's immutable reservation evidence. The
// environment owner qualifies it; storage checks all fields in its transaction.
type EnvironmentAdmission struct {
	Identity   runnercontract.ExecutionIdentity `json:"execution_identity"`
	Key        string                           `json:"environment_key"`
	Revision   uint64                           `json:"current_revision"`
	ActivityID string                           `json:"activity_id"`
	Basis      string                           `json:"basis"`
}

type Candidate struct {
	EnvironmentReadinessSkipped bool `json:"environment_readiness_skipped,omitempty"`

	Environment       *EnvironmentAdmission     `json:"environment,omitempty"`
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

// Only the latest resume can use the current Run's failure diagnostic.
// A direct rejection carries its own diagnostic independently of the Run.
func (c Candidate) ResumeFailureDiagnostic(action Action) string {
	if action.Command != "resume" {
		return ""
	}
	if action.State == "rejected" {
		return action.RejectionDiagnostic
	}
	r := c.Projection
	if action.State != "accepted" || action.ActionStatus != "failed" ||
		c.StopRequested || r == nil || r.Closed || r.Status != "blocked" ||
		c.RunID == "" || r.RunID != c.RunID || r.Revision == 0 ||
		r.InputDigest == "" || c.InputDigest != "" && c.InputDigest != r.InputDigest || !r.ResumeRequired {
		return ""
	}
	for i := len(c.Actions) - 1; i >= 0; i-- {
		if c.Actions[i].Command == "resume" {
			if c.Actions[i].Request.RequestID == action.Request.RequestID {
				return r.Diagnostic
			}
			break
		}
	}
	return ""
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
