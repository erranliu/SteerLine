//go:build !windows && !linux

package process

import (
	"errors"
	"os/exec"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/hostcontract"
)

func StartManaged(*exec.Cmd) (executionbackend.StopHandle, error) {
	return nil, errors.New("managed process host is unsupported on this operating system")
}

func InspectProcess(*hostcontract.ProcessIdentity) string { return "process identity unavailable" }

func Probe(*hostcontract.ProcessIdentity) ProbeResult {
	return ProbeResult{State: "unknown", Reason: "managed process host is unsupported on this operating system"}
}

func WorkerTreeAbsent(hostcontract.ProcessIdentity) bool { return false }
