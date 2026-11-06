package runnercontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

const ExecutionConfigurationContract = "steerrunner-execution-configuration/v1"
const ExecutionIdentityContract = "steerrunner-execution-identity/v1"
const ExecutionConfigurationLimit = 1 << 20

type ExecutionSelection struct {
	Adapter         string          `json:"adapter"`
	Target          ExecutionTarget `json:"target"`
	Agent           string          `json:"agent"`
	Model           *string         `json:"model,omitempty"`
	ReasoningEffort *string         `json:"reasoning_effort,omitempty"`
	AccessMode      string          `json:"access_mode"`
}
type ExecutionIdentity struct {
	Contract              string         `json:"contract"`
	SourceExecutionDigest string         `json:"source_execution_digest"`
	Context               ExecutionInput `json:"context"`
}
type ExecutionConfigurationRequest struct {
	Operation     string              `json:"operation"`
	Execution     *ExecutionInput     `json:"execution,omitempty"`
	Workflow      Workflow            `json:"workflow"`
	AccessMode    string              `json:"access_mode,omitempty"`
	BaseExecution *ExecutionInput     `json:"base_execution,omitempty"`
	Selection     *ExecutionSelection `json:"selection,omitempty"`
	Unset         []string            `json:"unset,omitempty"`
}
type ExecutionConfigurationSource struct {
	Version          string `json:"version"`
	ExecutableSHA256 string `json:"executable_sha256"`
	ResourceDigest   string `json:"resource_digest"`
}
type ExecutionConfigurationResult struct {
	Contract       string                       `json:"contract"`
	Operation      string                       `json:"operation"`
	InputDigest    string                       `json:"input_digest"`
	Source         ExecutionConfigurationSource `json:"source"`
	Execution      *ExecutionInput              `json:"execution,omitempty"`
	View           ExecutionSelection           `json:"view"`
	Identity       *ExecutionIdentity           `json:"identity,omitempty"`
	SourceIdentity *ExecutionIdentity           `json:"source_identity,omitempty"`
}

// ExecutionConfigurationCapability is optional: a malformed value disables only
// new configuration operations, never an existing Run's observation or stop.
type ExecutionConfigurationCapability struct {
	Contract         string              `json:"contract"`
	Command          string              `json:"command"`
	QueryOnly        bool                `json:"query_only"`
	RequiresService  *bool               `json:"requires_service"`
	Operations       []string            `json:"operations"`
	RequestSchema    json.RawMessage     `json:"request_schema"`
	ResponseSchema   json.RawMessage     `json:"response_schema"`
	ReasoningEfforts map[string][]string `json:"reasoning_efforts"`
	EditableFields   []string            `json:"editable_fields"`
	Limits           struct {
		InputBytes  int `json:"input_bytes"`
		OutputBytes int `json:"output_bytes"`
		TimeoutMS   int `json:"timeout_ms"`
	} `json:"limits"`
}

func ConfigurationValueDigest(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	body, err = CanonicalJSON(body)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func ValidateExecutionIdentity(source ExecutionInput, identity ExecutionIdentity) error {
	var fields map[string]json.RawMessage
	var sourceFields map[string]json.RawMessage
	digest := ConfigurationValueDigest(source)
	if digest == "" || ConfigurationValueDigest(identity.Context) == "" || json.Unmarshal(source.Settings, &sourceFields) != nil || len(sourceFields) == 0 || identity.Contract != ExecutionIdentityContract || identity.SourceExecutionDigest != digest || identity.Context.Adapter != source.Adapter || identity.Context.Target != source.Target || json.Unmarshal(identity.Context.Settings, &fields) != nil || len(fields) == 0 {
		return errors.New("Runner configuration identity differs from its frozen source")
	}
	return nil
}
func ExecutionContextKey(source ExecutionInput, identity ExecutionIdentity) (string, error) {
	if err := ValidateExecutionIdentity(source, identity); err != nil {
		return "", err
	}
	return ConfigurationValueDigest(identity.Context), nil
}
