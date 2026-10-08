//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

func Start(cmd *exec.Cmd) (*Process, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &Process{Cmd: cmd, Done: make(chan struct{})}
	p.stop = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	go func() {
		err := cmd.Wait()
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		p.mu.Lock()
		p.err = err
		p.mu.Unlock()
		close(p.Done)
	}()
	return p, nil
}

func StartVisibleConsole(cmd *exec.Cmd) (*Process, error) { return Start(cmd) }
