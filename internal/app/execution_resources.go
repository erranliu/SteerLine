package app

import (
	steerline "github.com/erranliu/SteerLine/internal/app/host"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionprocess "github.com/erranliu/SteerLine/internal/execution/process"
)

type executionResources struct{}

func (executionResources) WorkerTreeAbsent(identity executionbackend.ProcessIdentity) bool {
	return executionprocess.WorkerTreeAbsent(identity)
}

func (executionResources) AtomicWrite(path string, data []byte) error {
	return steerline.AtomicWrite(path, data)
}
