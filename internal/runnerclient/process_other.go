//go:build !windows

package runnerclient

import "os/exec"

func hide(*exec.Cmd) {}
