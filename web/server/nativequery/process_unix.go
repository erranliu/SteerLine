//go:build !windows

package nativequery

import (
	"os/exec"
	"syscall"
)

func prepare(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func own(cmd *exec.Cmd) (func() error, error) {
	return func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }, nil
}
