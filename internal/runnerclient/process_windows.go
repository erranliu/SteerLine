//go:build windows

package runnerclient

import (
	"os/exec"

	"github.com/erranliu/SteerLine/internal/processwindow"
)

func hide(command *exec.Cmd) { processwindow.Hide(command) }
