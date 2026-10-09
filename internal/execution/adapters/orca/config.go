package orcaworker

import (
	"context"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// Output is the host process runner. The adapter owns CLI arguments and decoding.
type Config struct {
	Repository, CommandTimeout string
	Target                     executionbackend.ExecutionTarget
	Output                     func(context.Context, time.Duration, ...string) ([]byte, error)
	InspectWorkspace           func(context.Context, executionbackend.ExecutionRef) (executionbackend.WorkspaceInspection, error)
}
