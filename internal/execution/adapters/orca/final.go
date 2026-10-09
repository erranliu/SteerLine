package orcaworker

import (
	"context"
	"errors"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// Consume only the original managed launcher binding and public native history.
// Orca worker_done summaries and transcript text projections are not final facts.
func (l *ExecutionLifecycle) readNativeFinal(ctx context.Context, snapshot executionbackend.LifecycleSnapshot, turn *NativeTurnState, state *NativeExecutionState) error {
	handled, err := l.readBoundHistoryFinal(ctx, snapshot, turn, state)
	if !handled {
		return errors.New("original managed native history binding is unavailable; no alternate source/session was selected")
	}
	return err
}
