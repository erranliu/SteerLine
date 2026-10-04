package orca_remote_worker

import (
	"fmt"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// CheckPolicy preserves the remote route. Protocol v1 has no native permission
// settings receipt, so it cannot prove a requested policy for this target.
func CheckPolicy(policy executionbackend.ExecutionPolicy, target Settings) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	version := target.ExpectedCodexVersion
	if version == "" {
		version = "unverified"
	}
	return &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: fmt.Sprintf("requested execution_policy (read=%s write=%s git=%t network=%s escalation=%s) lacks remote worker-start enforcement evidence: environment=%s connection=%s repository=%s Codex=%s", policy.Filesystem.ReadScope, policy.Filesystem.WriteScope, policy.Filesystem.GitMetadataWrite, policy.Network, policy.Escalation, target.EnvironmentID, target.ConnectionID, target.RepositorySelector, version)}
}
