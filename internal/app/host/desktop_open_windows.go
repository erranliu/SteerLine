//go:build windows

package host

import (
	"os/exec"

	"github.com/erranliu/SteerLine/internal/processwindow"
)

func hideDesktopLauncher(cmd *exec.Cmd) { processwindow.Hide(cmd) }
