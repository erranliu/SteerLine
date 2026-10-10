//go:build !windows

package host

import "os/exec"

func hideDesktopLauncher(cmd *exec.Cmd) {}
