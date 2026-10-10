//go:build windows

package host

import (
	"os/exec"
	"syscall"
)

func hideDesktopLauncher(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
