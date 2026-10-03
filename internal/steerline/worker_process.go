package steerline

import (
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
