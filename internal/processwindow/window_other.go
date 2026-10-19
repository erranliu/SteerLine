//go:build !windows

package processwindow

import "os/exec"

func Hide(*exec.Cmd) {}
