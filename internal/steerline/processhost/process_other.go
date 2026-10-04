//go:build !windows && !linux

package processhost

import (
	"errors"
	"os"
	"os/exec"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func startManaged(*exec.Cmd) (executionbackend.StopHandle, error) {
	return nil, errors.New("managed process host is unsupported on this operating system")
}

func inspectProcess(*executionbackend.ProcessIdentity) string { return "process identity unavailable" }

func replaceFile(from, to string) error { return os.Rename(from, to) }
