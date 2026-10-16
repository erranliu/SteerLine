package app

import (
	"time"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionprocess "github.com/erranliu/SteerLine/internal/execution/process"
)

func NewProcessHost(timeout time.Duration) executionbackend.ManagedProcessHost {
	return executionprocess.New(timeout)
}

func InstallLaunchers() ([]string, error) { return steerline.InstallLaunchers(HomeDir()) }
