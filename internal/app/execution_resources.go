package app

import (
	steerline "github.com/erranliu/SteerLine/internal/app/host"
	executionprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/hostcontract"
)

type executionResources struct{}

func (executionResources) WorkerTreeAbsent(identity hostcontract.ProcessIdentity) bool {
	return executionprocess.WorkerTreeAbsent(identity)
}

func (executionResources) AtomicWrite(path string, data []byte) error {
	return steerline.AtomicWrite(path, data)
}
