package orca_remote_worker

import (
	"context"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func (l *ExecutionLifecycle) ShutdownLifecycle(ctx context.Context, snapshot executionbackend.LifecycleSnapshot, _ executionbackend.StopHandle) (executionbackend.LifecycleStateFacts, error) {
	if err := ctx.Err(); err != nil {
		return executionbackend.LifecycleStateFacts{}, err
	}
	return executionbackend.LifecycleStateFacts{}, l.validateSnapshot(snapshot)
}
