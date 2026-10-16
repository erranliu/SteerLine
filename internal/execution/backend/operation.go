package backend

import (
	"time"
)

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
