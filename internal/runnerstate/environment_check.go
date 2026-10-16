package runnerstate

import (
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"time"
)

const StartEnvironmentCheck = "start_environment_check"
const StopEnvironmentCheck = "stop_environment_check"

var ErrEnvironmentCheckNotFound = errors.New("environment check not found")

type EnvironmentCheckError struct{ Code, Message string }

func (e *EnvironmentCheckError) Error() string { return e.Code + ": " + e.Message }

type EnvironmentRunnerPin struct {
	Executable string `json:"executable"`
	Version    string `json:"version"`
	SHA256     string `json:"sha256"`
}
type EnvironmentCheckStartPayload struct {
	ProfileID  string                         `json:"profile_id,omitempty"`
	Execution  *runnercontract.ExecutionInput `json:"execution,omitempty"`
	AccessMode string                         `json:"access_mode"`
}
type EnvironmentCheckCommand struct {
	RequestID        string
	Action           string
	TargetID         string
	ExpectedRevision uint64
	Payload          json.RawMessage
	Original         json.RawMessage
}
type EnvironmentCheckOperation struct {
	RequestID  string                 `json:"request_id"`
	Action     string                 `json:"action"`
	Original   json.RawMessage        `json:"original"`
	Request    runnercontract.Request `json:"runner_request"`
	Status     string                 `json:"status"`
	CommandID  string                 `json:"command_id,omitempty"`
	Diagnostic string                 `json:"diagnostic,omitempty"`
}
type EnvironmentCheckRecord struct {
	ID             string                           `json:"id"`
	Project        string                           `json:"project"`
	Revision       uint64                           `json:"local_revision"`
	Selection      EnvironmentRunnerPin             `json:"runner"`
	ResourceDigest string                           `json:"resource_digest"`
	Input          runnercontract.CheckInput        `json:"input"`
	Capabilities   runnercontract.CheckCapabilities `json:"capabilities"`
	CheckID        string                           `json:"check_id"`
	Report         *runnercontract.Check            `json:"report"`
	Operations     []EnvironmentCheckOperation      `json:"operations"`
	CreatedAt      time.Time                        `json:"created_at"`
	Diagnostic     string                           `json:"diagnostic,omitempty"`
}
type EnvironmentCheckProfile struct {
	ID        string                        `json:"id"`
	Name      string                        `json:"name"`
	Execution runnercontract.ExecutionInput `json:"execution"`
}
type EnvironmentCheckCatalog struct {
	Profiles          []EnvironmentCheckProfile         `json:"profiles"`
	Capabilities      *runnercontract.CheckCapabilities `json:"capabilities"`
	UnavailableReason string                            `json:"unavailable_reason"`
	Repository        string                            `json:"repository"`
}
