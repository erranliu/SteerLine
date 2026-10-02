//go:build !windows && !linux

package processhost

import (
	"errors"
	"os"
	"os/exec"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func startManaged(*exec.Cmd) (executionapi.StopHandle, error) {
	return nil, errors.New("managed process host is unsupported on this operating system")
}

func inspectProcess(*executionapi.ProcessIdentity) string { return "process identity unavailable" }

func replaceFile(from, to string) error { return os.Rename(from, to) }
