package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const StartEnvironmentCheck = "start_environment_check"
const StopEnvironmentCheck = "stop_environment_check"

var ErrEnvironmentCheckNotFound = errors.New("environment check not found")

type EnvironmentCheckError struct{ Code, Message string }

func (e *EnvironmentCheckError) Error() string { return e.Message }

type EnvironmentCheckStartPayload struct {
	ProfileID  string           `json:"profile_id,omitempty"`
	Execution  *json.RawMessage `json:"execution,omitempty"`
	AccessMode string           `json:"access_mode"`
}

// Public projections contain opaque Runner documents. Native contract values
// and process clients stay behind the injected application port.
type EnvironmentCheckRecord struct {
	ID             string                      `json:"id"`
	Project        string                      `json:"project"`
	Revision       uint64                      `json:"local_revision"`
	Runner         json.RawMessage             `json:"runner"`
	ResourceDigest string                      `json:"resource_digest"`
	Input          json.RawMessage             `json:"input"`
	Capabilities   json.RawMessage             `json:"capabilities"`
	CheckID        string                      `json:"check_id"`
	Report         json.RawMessage             `json:"report"`
	Operations     []EnvironmentCheckOperation `json:"operations"`
	CreatedAt      time.Time                   `json:"created_at"`
	Diagnostic     string                      `json:"diagnostic,omitempty"`
}
type EnvironmentCheckOperation struct {
	RequestID  string          `json:"request_id"`
	Action     string          `json:"action"`
	Original   json.RawMessage `json:"original"`
	Request    json.RawMessage `json:"runner_request"`
	Status     string          `json:"status"`
	CommandID  string          `json:"command_id,omitempty"`
	Diagnostic string          `json:"diagnostic,omitempty"`
}
type EnvironmentCheckCatalog = json.RawMessage
type EnvironmentCheckCommand struct {
	RequestID        string
	Action           string
	TargetID         string
	ExpectedRevision uint64
	Payload          json.RawMessage
	Original         json.RawMessage
}
type EnvironmentCheckPort interface {
	EnvironmentCheckCatalog(context.Context) (EnvironmentCheckCatalog, error)
	EnvironmentChecks(context.Context) ([]EnvironmentCheckRecord, error)
	EnvironmentCheck(context.Context, string) (EnvironmentCheckRecord, error)
	EnvironmentCheckRequest(context.Context, string) (EnvironmentCheckRecord, error)
	SubmitEnvironmentCheck(context.Context, EnvironmentCheckCommand) (EnvironmentCheckRecord, error)
}
