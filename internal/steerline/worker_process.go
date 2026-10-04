package steerline

import (
	"os"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type durableWorkerProcess struct {
	identity executionbackend.ProcessIdentity
	process  *os.Process
}

func (p *durableWorkerProcess) Identity() executionbackend.ProcessIdentity { return p.identity }

func (p *durableWorkerProcess) Kill() {
	if p.process != nil {
		_ = p.process.Kill()
	}
}

func (p *durableWorkerProcess) Close() {}
