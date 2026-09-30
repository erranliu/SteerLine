package orca_remote_worker

import (
	"fmt"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// CheckPolicy preserves the remote route. Protocol v1 has no native permission
// settings receipt, so it cannot prove a requested policy for this target.
func CheckPolicy(policy executionapi.ExecutionPolicy, target Settings) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	version := target.ExpectedCodexVersion
	if version == "" {
		version = "unverified"
	}
	return &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: fmt.Sprintf("requested execution_policy (read=%s write=%s git=%t network=%s escalation=%s) lacks remote worker-start enforcement evidence: environment=%s connection=%s repository=%s Codex=%s", policy.Filesystem.ReadScope, policy.Filesystem.WriteScope, policy.Filesystem.GitMetadataWrite, policy.Network, policy.Escalation, target.EnvironmentID, target.ConnectionID, target.RepositorySelector, version)}
}
