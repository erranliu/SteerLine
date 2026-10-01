package steerline

import (
	"context"
	"os"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type durableWorkerProcess struct {
	identity executionapi.ProcessIdentity
	process  *os.Process
}

func (p *durableWorkerProcess) Identity() executionapi.ProcessIdentity { return p.identity }
func (p *durableWorkerProcess) Kill() {
	if p.process != nil {
		_ = p.process.Kill()
	}
}
func (p *durableWorkerProcess) Close() {} // Closing observation never kills the worker.

func deliverDurableWorkerEvent(ctx context.Context, worker *Worker, event WorkerEvent) bool {
	event.Persisted = make(chan error, 1)
	select {
	case worker.Events <- event:
	case <-ctx.Done():
		return false
	}
	select {
	case err := <-event.Persisted:
		return err == nil
	case <-ctx.Done():
		return false
	}
}
