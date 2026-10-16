// Package-owned copies of the published Runner check/v1 wire contract.
package runnercontract

import (
	"encoding/json"
	"time"
)

const EnvironmentCheckContract = "steerrunner-environment-check/v1"

type CheckInput struct {
	Correlation      map[string]string `json:"correlation,omitempty"`
	Repository       Repository        `json:"repository"`
	Execution        ExecutionInput    `json:"execution"`
	Publication      *Publication      `json:"publication,omitempty"`
	ExecutionTimeout string            `json:"execution_timeout,omitempty"`
	CleanupTimeout   string            `json:"cleanup_timeout,omitempty"`
}

type CheckStop struct {
	CheckID          string `json:"check_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
}

type CheckReceipt struct {
	CommandID     string `json:"command_id"`
	CheckID       string `json:"check_id"`
	ReceiptStatus string `json:"receipt_status"`
	ActionStatus  string `json:"action_status"`
}

type CheckCommand struct {
	RequestID     string      `json:"request_id"`
	Command       string      `json:"command"`
	CommandID     string      `json:"command_id"`
	CheckID       string      `json:"check_id"`
	ReceiptStatus string      `json:"receipt_status"`
	ActionStatus  string      `json:"action_status"`
	Error         *CheckError `json:"error"`
}

type CheckInspection struct {
	Raw     json.RawMessage `json:"-"`
	Command *CheckCommand   `json:"command"`
	Check   *Check          `json:"check"`
}

type CheckEvidence struct {
	CheckID     string            `json:"check_id"`
	SessionID   string            `json:"session_id"`
	TurnID      string            `json:"turn_id"`
	Nonce       string            `json:"nonce"`
	OperationID string            `json:"operation_id"`
	ToolCallID  string            `json:"tool_call_id"`
	Source      string            `json:"source"`
	Verified    bool              `json:"verified"`
	Facts       map[string]string `json:"facts,omitempty"`
}

type CheckItem struct {
	ID       string          `json:"id"`
	Required bool            `json:"required"`
	Status   string          `json:"status"`
	Stage    string          `json:"stage"`
	Evidence []CheckEvidence `json:"evidence"`
}

type CheckContext struct {
	Nonce                string `json:"nonce,omitempty"`
	NativeGeneration     string `json:"native_generation,omitempty"`
	AttemptID            string `json:"attempt_id"`
	Generation           string `json:"generation"`
	SessionID            string `json:"session_id,omitempty"`
	TurnID               string `json:"turn_id,omitempty"`
	RuntimeVersion       string `json:"runtime_version,omitempty"`
	PID                  int    `json:"pid,omitempty"`
	ProcessCreated       string `json:"process_created,omitempty"`
	Account              string `json:"account,omitempty"`
	AuthenticationSource string `json:"authentication_source,omitempty"`
	AccessMode           string `json:"access_mode"`
	Sandbox              string `json:"sandbox,omitempty"`
	ApprovalPolicy       string `json:"approval_policy,omitempty"`
	Workspace            string `json:"workspace,omitempty"`
}

type CheckResources struct {
	BaseOwned     bool   `json:"base_owned"`
	HeadOwned     bool   `json:"head_owned"`
	BaseRef       string `json:"base_ref"`
	HeadRef       string `json:"head_ref"`
	BaseSHA       string `json:"base_sha"`
	HeadSHA       string `json:"head_sha,omitempty"`
	PRURL         string `json:"pr_url,omitempty"`
	PRNumber      int    `json:"pr_number,omitempty"`
	PRClosed      bool   `json:"pr_closed"`
	BaseDeleted   bool   `json:"base_deleted"`
	HeadDeleted   bool   `json:"head_deleted"`
	WriterStopped bool   `json:"writer_stopped"`
	LocalRemoved  bool   `json:"local_removed"`
}

type Check struct {
	CheckID           string         `json:"check_id"`
	Revision          uint64         `json:"revision"`
	Contract          string         `json:"contract"`
	InputDigest       string         `json:"input_digest"`
	ResourceDigest    string         `json:"resource_digest"`
	Input             CheckInput     `json:"input"`
	Actual            CheckContext   `json:"actual"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	ExecutionDeadline time.Time      `json:"execution_deadline"`
	CleanupDeadline   time.Time      `json:"cleanup_deadline"`
	Status            string         `json:"status"`
	Result            string         `json:"result"`
	CleanupStatus     string         `json:"cleanup_status"`
	Diagnostic        string         `json:"diagnostic,omitempty"`
	Items             []CheckItem    `json:"items"`
	Resources         CheckResources `json:"resources"`
	AllowedActions    []string       `json:"allowed_actions"`
	Excluded          []string       `json:"excluded"`
}

type CheckError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CheckSupport struct {
	Adapter               string   `json:"adapter"`
	Target                string   `json:"target"`
	Platform              string   `json:"platform,omitempty"`
	AccessModes           []string `json:"access_modes"`
	Mechanism             string   `json:"mechanism"`
	Reason                string   `json:"reason,omitempty"`
	AuthenticationSources []string `json:"authentication_sources,omitempty"`
	NativeVersion         string   `json:"native_version,omitempty"`
}

type CheckCapabilities struct {
	Contract     string         `json:"contract"`
	InputSchema  string         `json:"input_schema"`
	ReportSchema string         `json:"report_schema"`
	Targets      []CheckSupport `json:"targets"`
	// Schema is the same executable's environment_check_schema, frozen with the request.
	Schema json.RawMessage `json:"schema"`
}

// Definition selects a local schema without permitting filesystem/network loads.
func (c CheckCapabilities) Definition(name string) json.RawMessage {
	var root map[string]json.RawMessage
	if json.Unmarshal(c.Schema, &root) != nil || len(root["$defs"]) == 0 {
		return nil
	}
	delete(root, "$id")
	root["$ref"], _ = json.Marshal("#/$defs/" + name)
	body, _ := json.Marshal(root)
	return body
}

func (c Check) CanStop() bool {
	for _, action := range c.AllowedActions {
		if action == "stop" {
			return true
		}
	}
	return false
}

// Rejected operations have no Check, represented by null in the public contract.
func (c CheckCommand) MarshalJSON() ([]byte, error) {
	type wire CheckCommand
	var id *string
	if c.CheckID != "" {
		id = &c.CheckID
	}
	return json.Marshal(struct {
		wire
		CheckID *string `json:"check_id"`
	}{wire(c), id})
}
