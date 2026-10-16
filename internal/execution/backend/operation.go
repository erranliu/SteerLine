package backend

import (
	"time"
)

// LifecycleOperationIdentity fixes the input resource binding at intent time.
// Reconciliation may inspect later committed receipts, but never replaces this
// original identity with a current Attempt, target, or generation.
type LifecycleOperationIdentity struct {
	ProjectID           string       `json:"project_id"`
	ExecutionID         string       `json:"execution_id"`
	ConfigurationDigest string       `json:"configuration_digest"`
	ResourceGeneration  string       `json:"resource_generation"`
	InputBindingDigest  string       `json:"input_binding_digest"`
	Ref                 ExecutionRef `json:"ref"`
}

// LaunchCommand is a fixed executable and independent argv. The execution
// implementation owns shell quoting; EnvironmentRefs contain no secret values.
type LaunchCommand struct {
	Executable       string
	Args             []string
	WorkingDirectory string
	EnvironmentRefs  []string
	// BlockedEnvironment removes ambient routing from commands with a fixed host.
	BlockedEnvironment []string
	// Timeout overrides the host's ordinary command budget for this operation.
	// The caller's context can still cancel it earlier.
	Timeout time.Duration
}
