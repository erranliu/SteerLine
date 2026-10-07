package orca_remote_worker

import (
	"fmt"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// CheckPolicy preserves the existing remote missing-evidence admission gate.
// Local Windows launch settings cannot prove this target's native enforcement.
func CheckPolicy(target Settings) error {
	version := target.ExpectedCodexVersion
	if version == "" {
		version = "unverified"
	}
	return &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: fmt.Sprintf("remote worker-start enforcement evidence is missing: environment=%s connection=%s repository=%s Codex=%s", target.EnvironmentID, target.ConnectionID, target.RepositorySelector, version)}
}
