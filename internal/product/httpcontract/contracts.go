// Package httpcontract defines transport values shared by HTTP consumers and
// handlers. It has no backend dependencies and executes no domain rules.
package httpcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

const Protocol = "steerline-client/v1"

var requestIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ProjectID is an opaque, stable route key for the immutable repository
// identity. The identity itself remains in the authenticated identity DTO.
func ProjectID(identity string) string {
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

type Identity struct {
	Protocol           string          `json:"protocol"`
	ProjectID          string          `json:"project_id"`
	RepositoryIdentity string          `json:"repository_identity"`
	InstanceID         string          `json:"instance_id"`
	Process            ProcessIdentity `json:"process"`
	Address            string          `json:"address"`
	Ready              bool            `json:"ready"`
	Status             string          `json:"status"`
	Capabilities       []string        `json:"capabilities"`
	SampledAt          time.Time       `json:"sampled_at"`
	Diagnostic         string          `json:"diagnostic,omitempty"`
	LockPaths          []string        `json:"lock_paths,omitempty"`
}

type ProcessIdentity struct {
	Kind       string `json:"kind"`
	PID        int    `json:"pid"`
	Created    string `json:"created"`
	Executable string `json:"executable"`
}

type Discovery struct {
	Version            int             `json:"version"`
	Protocol           string          `json:"protocol"`
	ProjectID          string          `json:"project_id"`
	RepositoryIdentity string          `json:"repository_identity"`
	InstanceID         string          `json:"instance_id"`
	Process            ProcessIdentity `json:"process"`
	Address            string          `json:"address"`
	CredentialPath     string          `json:"credential_path"`
	Ready              bool            `json:"ready"`
	Status             string          `json:"status"`
	Diagnostic         string          `json:"diagnostic,omitempty"`
	LeaseID            string          `json:"lease_id"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type ErrorResponse struct {
	Code      string `json:"code"`
	Reason    string `json:"reason"`
	RequestID string `json:"request_id,omitempty"`
}

// CommandEnvelope is the immutable client request. Payload is action-specific
// JSON documented in docs/product-design/local-client-protocol.md.
type CommandEnvelope struct {
	RequestID               string          `json:"request_id"`
	ProjectID               string          `json:"project_id"`
	InstanceID              string          `json:"instance_id"`
	Action                  string          `json:"action"`
	TargetID                string          `json:"target_id"`
	ExpectedVersion         uint64          `json:"expected_version"`
	ExpectedControlRevision uint64          `json:"expected_control_revision,omitempty"`
	ExpectedGraphRevision   uint64          `json:"expected_graph_revision,omitempty"`
	Payload                 json.RawMessage `json:"payload"`
}

type CommandResponse struct {
	TestRecord       json.RawMessage             `json:"test_record,omitempty"`
	Maintenance      json.RawMessage             `json:"maintenance,omitempty"`
	EnvironmentCheck *api.EnvironmentCheckRecord `json:"environment_check,omitempty"`
	RequestID        string                      `json:"request_id"`
	ProjectID        string                      `json:"project_id"`
	InstanceID       string                      `json:"instance_id"`
	Action           string                      `json:"action"`
	TargetID         string                      `json:"target_id"`
	ExpectedVersion  uint64                      `json:"expected_version"`
	Original         *CommandEnvelope            `json:"original,omitempty"`
	Status           string                      `json:"status"`
	Code             string                      `json:"code,omitempty"`
	Command          *api.CommandReceipt         `json:"command,omitempty"`
	ControlRequest   *api.ControlRequest         `json:"control_request,omitempty"`
	ControlReceipt   *api.ControlReceipt         `json:"control_receipt,omitempty"`
	Discard          *api.TaskDiscardReceipt     `json:"discard,omitempty"`
	Diagnostic       string                      `json:"diagnostic,omitempty"`
	SampledAt        time.Time                   `json:"sampled_at"`
}

type ActivityPage struct {
	ProjectID string       `json:"project_id"`
	SampledAt time.Time    `json:"sampled_at"`
	Page      api.LogBatch `json:"page"`
}

type ConversationPage struct {
	ProjectID string               `json:"project_id"`
	SampledAt time.Time            `json:"sampled_at"`
	Page      api.ConversationPage `json:"page"`
}

// ProjectPresentation exposes the existing, non-secret profile summaries the
// native client needs to select a configured execution profile.
type ProjectPresentation struct {
	ProjectID    string                  `json:"project_id"`
	SampledAt    time.Time               `json:"sampled_at"`
	Presentation api.ProjectPresentation `json:"presentation"`
}

type FinalReply struct {
	ProjectID   string               `json:"project_id"`
	WorkflowID  string               `json:"workflow_id"`
	ExecutionID string               `json:"execution_id"`
	AttemptID   string               `json:"attempt_id"`
	SampledAt   time.Time            `json:"sampled_at"`
	Page        api.ConversationPage `json:"page"`
	Bodies      []FinalBody          `json:"bodies"`
}

type FinalBody struct {
	ItemKey      string               `json:"item_key"`
	Revision     int64                `json:"revision"`
	SHA256       string               `json:"sha256"`
	Completeness api.BodyCompleteness `json:"completeness"`
	Text         string               `json:"text"`
	Complete     bool                 `json:"complete"`
}

func (e CommandEnvelope) Validate(projectID, instanceID string) error {
	if !requestIDPattern.MatchString(e.RequestID) {
		return fmt.Errorf("request_id must be 32 lowercase hexadecimal characters")
	}
	if e.ProjectID == "" || e.ProjectID != projectID {
		return fmt.Errorf("project_id does not match the connected project")
	}
	if e.InstanceID == "" || e.InstanceID != instanceID {
		return fmt.Errorf("service instance changed; refresh identity before submitting")
	}
	if e.Action == "" || e.TargetID == "" {
		return fmt.Errorf("action and target_id are required")
	}
	if len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return fmt.Errorf("payload must contain one JSON value")
	}
	return nil
}
