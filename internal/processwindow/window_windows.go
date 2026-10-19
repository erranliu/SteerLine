//go:build windows

// Package processwindow configures short-lived background commands without
// changing their ownership, standard streams or cancellation policy.
package processwindow

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func Hide(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.HideWindow = true
	// SW_HIDE alone still allocates a console when the parent is detached.
	command.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}
