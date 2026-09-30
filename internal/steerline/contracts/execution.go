package contracts

import (
	"encoding/json"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type Command struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
}

type OrcaConfig struct {
	Executable   string                `json:"executable"`
	Args         []string              `json:"args"`
	Shell        string                `json:"shell"`
	WaitTimeout  string                `json:"wait_timeout"`
	KeepTerminal bool                  `json:"keep_terminal"`
	Activate     bool                  `json:"activate"`
	Supervised   *SupervisedOrcaConfig `json:"supervised,omitempty"`
}

type SupervisedOrcaConfig struct {
	ConnectionID            string `json:"connection_id"`
	EnvironmentID           string `json:"environment_id"`
	RepositorySelector      string `json:"repository_selector"`
	CoordinatorTerminal     string `json:"coordinator_terminal"`
	ExpectedCodexVersion    string `json:"expected_codex_version,omitempty"` // Optional exact pin; empty accepts and records the observed CLI version.
	PollInterval            string `json:"poll_interval"`
	CapabilityReportVersion int    `json:"capability_report_version"`
	RemoteCancelAvailable   bool   `json:"remote_cancel_available"`
}

type LegacyCompletionConfig struct {
	Command Command `json:"command"`
	Timeout string  `json:"timeout"`
}

type ControlReceipt struct {
	Version         int        `json:"version"`
	RequestID       string     `json:"request_id"`
	RequestDigest   string     `json:"request_digest"`
	InstanceID      string     `json:"instance_id"`
	Action          string     `json:"action"`
	Status          string     `json:"status"`
	Code            string     `json:"code"`
	Message         string     `json:"message"`
	RequestedBy     string     `json:"requested_by"`
	SubmittedAt     time.Time  `json:"submitted_at"`
	AcceptedAt      *time.Time `json:"accepted_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	RunID           string     `json:"run_id,omitempty"`
	AttemptID       string     `json:"attempt_id,omitempty"`
	DecisionID      string     `json:"decision_id,omitempty"`
	ControlRevision uint64     `json:"control_revision,omitempty"`
	RunRevision     uint64     `json:"run_revision,omitempty"`
}

type CodexAskCall struct {
	RequestID  json.RawMessage         `json:"request_id"`
	ThreadID   string                  `json:"thread_id"`
	TurnID     string                  `json:"turn_id"`
	CallID     string                  `json:"call_id"`
	Generation string                  `json:"generation,omitempty"`
	Questions  []executionapi.Question `json:"questions"`
}

type OrcaAskCall struct {
	RunID      string                  `json:"run_id"`
	TaskID     string                  `json:"task_id"`
	DispatchID string                  `json:"dispatch_id"`
	MessageID  string                  `json:"message_id"`
	Questions  []executionapi.Question `json:"questions"`
}

type FixedExecutionProfile struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Digest          string     `json:"digest"`
	Host            string     `json:"host"`
	Agent           string     `json:"agent"`
	Model           string     `json:"model,omitempty"`
	ReasoningEffort string     `json:"reasoning_effort,omitempty"`
	Sandbox         string     `json:"sandbox"`
	WindowsSandbox  string     `json:"windows_sandbox,omitempty"`
	ApprovalPolicy  string     `json:"approval_policy,omitempty"`
	CodexProfile    string     `json:"profile,omitempty"`
	Codex           Command    `json:"codex"`
	Orca            OrcaConfig `json:"orca,omitempty"`
}
