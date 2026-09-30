package orcaworker

import (
	"context"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// Output is the host process runner. The adapter owns CLI arguments and decoding.
type Config struct {
	Repository, CommandTimeout string
	Target                     executionapi.ExecutionTarget
	Output                     func(context.Context, time.Duration, ...string) ([]byte, error)
	AgentVersion               func(context.Context) (string, error)
	InspectWorkspace           func(context.Context, executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error)
}
