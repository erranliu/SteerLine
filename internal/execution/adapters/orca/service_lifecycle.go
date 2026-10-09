package orcaworker

import (
	"context"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// The native execution belongs to Orca. Releasing central observation never
// closes its terminal, interrupts its Dispatch, or starts another worker.
func (l *ExecutionLifecycle) ShutdownLifecycle(ctx context.Context, snapshot executionbackend.LifecycleSnapshot, _ executionbackend.StopHandle) (executionbackend.LifecycleStateFacts, error) {
	if err := ctx.Err(); err != nil {
		return executionbackend.LifecycleStateFacts{}, err
	}
	return executionbackend.LifecycleStateFacts{}, l.validateSnapshot(snapshot)
}
